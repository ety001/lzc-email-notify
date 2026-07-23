// Package mailcheck 实现 IMAP/POP3 邮箱巡检与新邮件判定。
//
// 新邮件判定语义：
//   - IMAP：记录 UIDVALIDITY + 最大 UID；UIDVALIDITY 变化重建基线；
//     新邮件 = UID 大于已记录最大 UID。
//   - POP3：用 UIDL 维护已知邮件集合（上限 5000，超出裁最旧）；
//     新邮件 = UIDL 不在集合中。
//
// 首次成功巡检只建立基线，不返回新邮件。
package mailcheck

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/pop3"
)

// opTimeout 是单个网络操作阶段的超时。
const opTimeout = 25 * time.Second

// TestTimeout 是测试连接端点的整体超时。
const TestTimeout = 15 * time.Second

// Mail 是一封新邮件的概要。
type Mail struct {
	UID     uint32 // IMAP UID（POP3 恒为 0）
	From    string
	Subject string
	Date    time.Time
}

// Result 是一次成功巡检的结果。
type Result struct {
	// NewMails 按时间先后（UID 升序 / UIDL 顺序）排列的新邮件。
	NewMails []Mail
	// BaselineEstablished 本次巡检建立了基线（首次成功或 UIDVALIDITY 变化重建）。
	BaselineEstablished bool
	// UIDValidityChanged 本次因 UIDVALIDITY 变化而重建基线。
	UIDValidityChanged bool
	// NewState 巡检后应持久化的内部状态。
	NewState account.State
}

// Check 按账号协议执行一次巡检。acc 为账号快照（含密码与既有 State）。
func Check(ctx context.Context, acc *account.Account) (*Result, error) {
	switch acc.Protocol {
	case account.ProtocolIMAP:
		return checkIMAP(ctx, acc)
	case account.ProtocolPOP3:
		return checkPOP3(ctx, acc)
	default:
		return nil, fmt.Errorf("不支持的协议: %s", acc.Protocol)
	}
}

// Test 测试连接：拨号 + 登录 + 轻量探测（IMAP: STATUS INBOX；
// POP3: CAPA + UIDL），不改动任何巡检状态。返回的 error 为中文描述。
func Test(ctx context.Context, acc *account.Account) error {
	ctx, cancel := context.WithTimeout(ctx, TestTimeout)
	defer cancel()
	switch acc.Protocol {
	case account.ProtocolIMAP:
		return testIMAP(ctx, acc)
	case account.ProtocolPOP3:
		return testPOP3(ctx, acc)
	default:
		return fmt.Errorf("不支持的协议: %s", acc.Protocol)
	}
}

func addr(acc *account.Account) string {
	return net.JoinHostPort(acc.Host, fmt.Sprint(acc.Port))
}

// ---------------------------------------------------------------------------
// IMAP
// ---------------------------------------------------------------------------

func imapOptions() *imapclient.Options {
	o := &imapclient.Options{Dialer: &net.Dialer{Timeout: opTimeout}}
	// IMAP_DEBUG=1 时输出协议级交互日志，用于排查服务器兼容性问题
	if os.Getenv("IMAP_DEBUG") == "1" {
		o.DebugWriter = os.Stderr
	}
	return o
}

// dialIMAP 建立并返回 IMAP 连接。ssl=true 隐式 TLS；ssl=false 时先明文，
// 若服务器声明 STARTTLS 则用 DialStartTLS 升级，升级失败回落明文。
func dialIMAP(acc *account.Account) (*imapclient.Client, error) {
	a := addr(acc)
	if acc.SSL {
		return imapclient.DialTLS(a, imapOptions())
	}
	probe, err := imapclient.DialInsecure(a, imapOptions())
	if err != nil {
		return nil, err
	}
	if probe.Caps().Has(imap.CapStartTLS) {
		probe.Close()
		if c, err := imapclient.DialStartTLS(a, imapOptions()); err == nil {
			return c, nil
		}
		return imapclient.DialInsecure(a, imapOptions())
	}
	return probe, nil
}

// imapCtx 返回带 25 秒超时的子 context，并启动看门狗：
// 超时/取消时关闭连接使阻塞中的命令返回错误。stop 必须在阶段结束后调用。
func imapCtx(ctx context.Context, c *imapclient.Client) (context.Context, func() (timedOut bool)) {
	cctx, cancel := context.WithTimeout(ctx, opTimeout)
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-cctx.Done():
			// stop() 先 close(done) 再 cancel()，看门狗可能在两者都就绪后
			// 才被调度（select 随机选择），需二次确认：done 已关闭说明是
			// 正常结束，不能误杀健康连接（否则下一阶段读写报 unexpected EOF）
			select {
			case <-done:
			default:
				c.Close()
			}
		}
	}()
	return cctx, func() bool {
		close(done)
		timedOut := cctx.Err() != nil
		cancel()
		return timedOut
	}
}

func checkIMAP(ctx context.Context, acc *account.Account) (*Result, error) {
	// 拨号超时由 Options.Dialer 控制（25s）
	c, err := dialIMAP(acc)
	if err != nil {
		return nil, fmt.Errorf("连接服务器失败: %w", err)
	}
	defer c.Close()

	// 阶段一：登录 + STATUS INBOX
	_, stop1 := imapCtx(ctx, c)
	if err := c.Login(acc.Username, acc.Password).Wait(); err != nil {
		timedOut := stop1()
		c.Logout().Wait()
		if timedOut {
			return nil, fmt.Errorf("登录超时: %w", err)
		}
		return nil, fmt.Errorf("登录失败，请检查用户名和授权码: %w", err)
	}
	status, err := c.Status("INBOX", &imap.StatusOptions{UIDNext: true, UIDValidity: true}).Wait()
	timedOut := stop1()
	if err != nil {
		c.Logout().Wait()
		if timedOut {
			return nil, fmt.Errorf("查询邮箱状态超时: %w", err)
		}
		return nil, fmt.Errorf("查询邮箱状态失败: %w", err)
	}
	// 注意：不能写 defer c.Logout().Wait()——defer 会立即对 c.Logout() 求值，
	// 导致 LOGOUT 提前发出，后续 SELECT/FETCH 直接 unexpected EOF
	defer func() { c.Logout().Wait() }()

	var maxUID uint32
	if status.UIDNext > 0 {
		maxUID = uint32(status.UIDNext) - 1
	}
	uidValidity := status.UIDValidity

	res := &Result{}
	st := acc.State
	st.UIDValidity = uidValidity

	// 首次成功或 UIDVALIDITY 变化：只建基线，不通知
	if !acc.Status.BaselineDone || acc.State.UIDValidity != uidValidity {
		res.BaselineEstablished = true
		res.UIDValidityChanged = acc.Status.BaselineDone && acc.State.UIDValidity != uidValidity
		st.LastUID = maxUID
		res.NewState = st
		return res, nil
	}

	if maxUID <= acc.State.LastUID {
		// 无新邮件（空邮箱 UIDNext==1 时 maxUID=0 也走这里）
		res.NewState = st
		return res, nil
	}

	// 阶段二：SELECT + FETCH (lastUID+1):*
	_, stop2 := imapCtx(ctx, c)
	if _, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		stop2()
		return nil, fmt.Errorf("打开收件箱失败: %w", err)
	}
	var uidSet imap.UIDSet
	uidSet.AddRange(imap.UID(acc.State.LastUID+1), 0) // 0 表示 "*"
	bufs, err := c.Fetch(uidSet, &imap.FetchOptions{Envelope: true, UID: true}).Collect()
	timedOut2 := stop2()
	if err != nil {
		if timedOut2 {
			return nil, fmt.Errorf("拉取新邮件超时: %w", err)
		}
		return nil, fmt.Errorf("拉取新邮件失败: %w", err)
	}

	mails := make([]Mail, 0, len(bufs))
	for _, b := range bufs {
		m := Mail{UID: uint32(b.UID)}
		if b.Envelope != nil {
			m.From = formatAddresses(b.Envelope.From)
			m.Subject = b.Envelope.Subject
			m.Date = b.Envelope.Date
		}
		mails = append(mails, m)
	}
	// 按 UID 升序排序保证稳定
	sortMailsByUID(mails)

	st.LastUID = maxUID
	res.NewState = st
	res.NewMails = mails
	return res, nil
}

func testIMAP(ctx context.Context, acc *account.Account) error {
	c, err := dialIMAP(acc)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer c.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()
	if err := c.Login(acc.Username, acc.Password).Wait(); err != nil {
		c.Logout().Wait()
		return fmt.Errorf("登录失败，请检查用户名和授权码: %w", err)
	}
	if _, err := c.Status("INBOX", &imap.StatusOptions{UIDNext: true, UIDValidity: true}).Wait(); err != nil {
		c.Logout().Wait()
		return fmt.Errorf("查询邮箱状态失败: %w", err)
	}
	c.Logout().Wait()
	return nil
}

func formatAddresses(addrs []imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	a := addrs[0]
	if a.Name != "" {
		return a.Name + " <" + a.Addr() + ">"
	}
	return a.Addr()
}

func sortMailsByUID(mails []Mail) {
	for i := 1; i < len(mails); i++ {
		for j := i; j > 0 && mails[j].UID < mails[j-1].UID; j-- {
			mails[j], mails[j-1] = mails[j-1], mails[j]
		}
	}
}

// ---------------------------------------------------------------------------
// POP3
// ---------------------------------------------------------------------------

func checkPOP3(_ context.Context, acc *account.Account) (*Result, error) {
	c, err := pop3.Dial(addr(acc), acc.SSL, opTimeout)
	if err != nil {
		return nil, fmt.Errorf("连接服务器失败: %w", err)
	}
	defer c.Close()
	if err := c.Login(acc.Username, acc.Password); err != nil {
		c.Quit()
		return nil, fmt.Errorf("登录失败，请检查用户名和授权码: %w", err)
	}
	uidls, err := c.UIDL()
	if err != nil {
		c.Quit()
		return nil, fmt.Errorf("获取邮件列表失败: %w", err)
	}

	res := &Result{}
	known := append([]string(nil), acc.State.KnownUIDLs...)
	knownSet := make(map[string]struct{}, len(known))
	for _, u := range known {
		knownSet[u] = struct{}{}
	}

	// 首次成功：只建基线，不通知
	if !acc.Status.BaselineDone {
		res.BaselineEstablished = true
		res.NewState = account.State{KnownUIDLs: trimUIDLs(append(known, uidlStrings(uidls)...))}
		c.Quit()
		return res, nil
	}

	var newEntries []pop3.UIDLEntry
	for _, e := range uidls {
		if _, ok := knownSet[e.UID]; !ok {
			newEntries = append(newEntries, e)
		}
	}

	mails := make([]Mail, 0, len(newEntries))
	for _, e := range newEntries {
		// 无论取头部成功与否都登记 UIDL，避免每次巡检反复重试同一封
		raw, err := c.Top(e.Num, 0)
		if err != nil {
			known = append(known, e.UID)
			continue
		}
		h, err := pop3.ParseHeaders(raw)
		if err != nil {
			known = append(known, e.UID)
			continue
		}
		mails = append(mails, Mail{From: h.From, Subject: h.Subject, Date: h.Date})
		known = append(known, e.UID)
	}
	c.Quit()

	res.NewState = account.State{KnownUIDLs: trimUIDLs(known)}
	res.NewMails = mails
	return res, nil
}

func testPOP3(ctx context.Context, acc *account.Account) error {
	timeout := opTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d < timeout {
			timeout = d
		}
	}
	c, err := pop3.Dial(addr(acc), acc.SSL, timeout)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	defer c.Close()
	if err := c.Login(acc.Username, acc.Password); err != nil {
		c.Quit()
		return fmt.Errorf("登录失败，请检查用户名和授权码: %w", err)
	}
	// CAPA 不支持不算失败，继续 UIDL 探测
	_, _ = c.CAPA()
	if _, err := c.UIDL(); err != nil {
		c.Quit()
		return fmt.Errorf("获取邮件列表失败: %w", err)
	}
	c.Quit()
	return nil
}

func uidlStrings(entries []pop3.UIDLEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.UID)
	}
	return out
}

// trimUIDLs 将已知 UIDL 集合裁剪到上限，按插入顺序裁最旧。
func trimUIDLs(known []string) []string {
	if len(known) > account.MaxKnownUIDLs {
		known = known[len(known)-account.MaxKnownUIDLs:]
	}
	return known
}
