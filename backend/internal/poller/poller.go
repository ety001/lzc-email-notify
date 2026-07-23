// Package poller 为每个启用中的账号维护一个轮询协程，
// 账号增删改时同步启停/重启对应协程；同一账号巡检单 flight。
package poller

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/events"
	"github.com/ety001/lzc-email-notify/backend/internal/mailcheck"
	"github.com/ety001/lzc-email-notify/backend/internal/notify"
)

// maxIndividualNotify 单次巡检逐封通知的最大封数，超出合并为一条汇总。
const maxIndividualNotify = 3

// Deps 是轮询管理器的依赖。
type Deps struct {
	Store  *account.Store
	Events *events.Buffer
	Sender notify.Sender
}

type runner struct {
	cancel context.CancelFunc
	key    string
}

// Manager 管理全部账号的轮询协程。
type Manager struct {
	deps    Deps
	mu      sync.Mutex
	runners map[string]*runner // accountID -> runner
	wg      sync.WaitGroup
	stopped bool
}

// New 创建轮询管理器。
func New(deps Deps) *Manager {
	return &Manager{deps: deps, runners: make(map[string]*runner)}
}

// configKey 用于判断账号配置是否变更（变更需重启协程）。
func configKey(a *account.Account) string {
	return fmt.Sprintf("%s|%s|%d|%t|%s|%s|%d|%t",
		a.Protocol, a.Host, a.Port, a.SSL, a.Username, a.Password, a.IntervalSec, a.Enabled)
}

// Sync 根据存储中的账号集合对齐轮询协程：
// enabled 账号无协程则启动、配置变更则重启；disabled 或已删除则停止。
func (m *Manager) Sync() {
	accounts := m.deps.Store.ListAll()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	alive := make(map[string]struct{}, len(accounts))
	for _, a := range accounts {
		if !a.Enabled {
			continue
		}
		alive[a.ID] = struct{}{}
		key := configKey(a)
		if r, ok := m.runners[a.ID]; ok && r.key == key {
			continue
		}
		m.stopLocked(a.ID)
		m.startLocked(a)
	}
	for id := range m.runners {
		if _, ok := alive[id]; !ok {
			m.stopLocked(id)
		}
	}
}

func (m *Manager) startLocked(a *account.Account) {
	ctx, cancel := context.WithCancel(context.Background())
	m.runners[a.ID] = &runner{cancel: cancel, key: configKey(a)}
	m.wg.Add(1)
	go m.loop(ctx, a.ID, time.Duration(a.IntervalSec)*time.Second)
	log.Printf("[poller] 账号 %s(%s) 轮询已启动，间隔 %ds", a.Name, a.ID, a.IntervalSec)
}

func (m *Manager) stopLocked(id string) {
	if r, ok := m.runners[id]; ok {
		r.cancel()
		delete(m.runners, id)
		log.Printf("[poller] 账号 %s 轮询已停止", id)
	}
}

// StopAll 停止全部轮询协程并等待退出。
func (m *Manager) StopAll() {
	m.mu.Lock()
	m.stopped = true
	for id := range m.runners {
		m.stopLocked(id)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) loop(ctx context.Context, accountID string, interval time.Duration) {
	defer m.wg.Done()
	// 启动后立即执行一次巡检
	m.runCheck(ctx, accountID)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runCheck(ctx, accountID)
		}
	}
}

// Trigger 手动触发一次巡检（异步，单 flight），用于 /check 端点。
func (m *Manager) Trigger(accountID string) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		m.runCheck(ctx, accountID)
	}()
}

// tryBeginCheck 原子地将账号 checking 置 true；已在巡检中返回 false（单 flight）。
func (m *Manager) tryBeginCheck(accountID string) (*account.Account, bool) {
	acc, err := m.deps.Store.GetByID(accountID)
	if err != nil {
		return nil, false
	}
	if acc.Status.Checking {
		return nil, false
	}
	updated, err := m.deps.Store.UpdateByID(accountID, func(a *account.Account) bool {
		if a.Status.Checking {
			return false
		}
		a.Status.Checking = true
		return true
	})
	if err != nil || !updated.Status.Checking {
		return nil, false
	}
	return updated, true
}

func (m *Manager) endCheck(accountID string, mutate func(a *account.Account)) {
	if _, err := m.deps.Store.UpdateByID(accountID, func(a *account.Account) bool {
		a.Status.Checking = false
		mutate(a)
		return true
	}); err != nil {
		log.Printf("[poller] 账号 %s 巡检结束状态写回失败: %v", accountID, err)
	}
}

// runCheck 执行一次完整巡检：状态更新、事件记录、新邮件通知。
func (m *Manager) runCheck(ctx context.Context, accountID string) {
	acc, ok := m.tryBeginCheck(accountID)
	if !ok {
		return // 账号不存在或已有巡检在进行
	}

	res, err := mailcheck.Check(ctx, acc)
	now := time.Now()
	if err != nil {
		log.Printf("[poller] 账号 %s(%s) 巡检失败: %v", acc.Name, acc.ID, err)
		m.endCheck(accountID, func(a *account.Account) {
			a.Status.LastCheckAt = &now
			a.Status.LastError = err.Error()
		})
		m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindCheckFailed, err.Error())
		return
	}

	m.endCheck(accountID, func(a *account.Account) {
		a.Status.LastCheckAt = &now
		a.Status.LastSuccessAt = &now
		a.Status.LastError = ""
		a.Status.BaselineDone = true
		a.State = res.NewState
		if n := len(res.NewMails); n > 0 {
			latest := res.NewMails[n-1]
			a.Status.LastMail = &account.MailInfo{
				From:    latest.From,
				Subject: latest.Subject,
				Date:    latest.Date.Format(time.RFC3339),
			}
		}
	})

	switch {
	case res.UIDValidityChanged:
		m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindInfo, "邮箱 UIDVALIDITY 发生变化，已重建基线")
	case res.BaselineEstablished:
		m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindInfo, "已建立基线，后续新邮件将发送通知")
	}

	if len(res.NewMails) > 0 {
		m.notifyNewMails(ctx, acc, res.NewMails)
	}
}

// notifyNewMails 逐封通知（最多 maxIndividualNotify 封），超出合并一条汇总。
func (m *Manager) notifyNewMails(ctx context.Context, acc *account.Account, mails []mailcheck.Mail) {
	individual := mails
	var rest []mailcheck.Mail
	if len(mails) > maxIndividualNotify {
		individual = mails[:maxIndividualNotify]
		rest = mails[maxIndividualNotify:]
	}
	title := fmt.Sprintf("【%s】新邮件", acc.Name)
	for _, mail := range individual {
		body := mail.From + "\n" + mail.Subject
		if err := m.send(ctx, acc, notify.Payload{Title: title, Body: body, DeeplinkURL: acc.WebURL}); err != nil {
			m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindNotifyFailed,
				fmt.Sprintf("通知发送失败: %v", err))
			continue
		}
		m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindNewMail,
			fmt.Sprintf("%s：%s", mail.From, mail.Subject))
	}
	if len(rest) > 0 {
		latest := rest[len(rest)-1]
		body := fmt.Sprintf("共 %d 封新邮件，最新：%s - %s", len(rest), latest.From, latest.Subject)
		if err := m.send(ctx, acc, notify.Payload{Title: title, Body: body, DeeplinkURL: acc.WebURL}); err != nil {
			m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindNotifyFailed,
				fmt.Sprintf("汇总通知发送失败: %v", err))
			return
		}
		m.deps.Events.Add(acc.UID, acc.ID, acc.Name, events.KindNewMail, body)
	}
}

func (m *Manager) send(ctx context.Context, acc *account.Account, p notify.Payload) error {
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// 用户在设置页勾选了设备过滤时，只向选中设备定向发送
	if st := m.deps.Store.GetSettings(acc.UID); st.DeviceFilterEnabled {
		if dm, ok := m.deps.Sender.(notify.DeviceManager); ok {
			return dm.SendToDevices(sctx, acc.UID, st.NotifyDevices, p)
		}
	}
	return m.deps.Sender.Send(sctx, acc.UID, p)
}
