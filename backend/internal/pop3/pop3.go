// Package pop3 实现一个最小 POP3 客户端（RFC 1939 + STLS/CAPA 扩展），
// 仅支持巡检所需的命令：CAPA、STLS、USER/PASS、UIDL、TOP、QUIT。
package pop3

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"mime"
)

// UIDLEntry 是 UIDL 列表中的一项。
type UIDLEntry struct {
	Num int    // 邮件序号（1 起）
	UID string // 服务器唯一标识
}

// Client 是一个 POP3 连接。
type Client struct {
	conn    net.Conn
	r       *bufio.Reader
	timeout time.Duration
}

// Dial 建立 POP3 连接。ssl=true 时直接隐式 TLS；ssl=false 时先明文，
// 若服务器 CAPA 声明支持 STLS 则升级为 TLS。
func Dial(addr string, ssl bool, timeout time.Duration) (*Client, error) {
	d := &net.Dialer{Timeout: timeout}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("服务器地址无效: %w", err)
	}

	if ssl {
		conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return nil, err
		}
		c := &Client{conn: conn, r: bufio.NewReader(conn), timeout: timeout}
		if _, err := c.readOK(); err != nil {
			conn.Close()
			return nil, err
		}
		return c, nil
	}

	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, r: bufio.NewReader(conn), timeout: timeout}
	if _, err := c.readOK(); err != nil {
		conn.Close()
		return nil, err
	}
	// 明文连接：尝试 STLS 升级（服务器不支持则保持明文）
	if caps, err := c.CAPA(); err == nil {
		if _, ok := caps["STLS"]; ok {
			if _, err := c.cmd("STLS"); err == nil {
				tconn := tls.Client(conn, &tls.Config{ServerName: host})
				_ = tconn.SetDeadline(time.Now().Add(timeout))
				if err := tconn.Handshake(); err != nil {
					conn.Close()
					return nil, fmt.Errorf("TLS 握手失败: %w", err)
				}
				_ = tconn.SetDeadline(time.Time{})
				c.conn = tconn
				c.r = bufio.NewReader(tconn)
			}
			// STLS 命令失败则继续使用明文连接
		}
	}
	return c, nil
}

// Close 关闭连接（未 QUIT 时的兜底）。
func (c *Client) Close() error { return c.conn.Close() }

// Quit 发送 QUIT 并关闭连接。
func (c *Client) Quit() error {
	_, err := c.cmd("QUIT")
	c.conn.Close()
	return err
}

// CAPA 返回服务器能力集合（key 大写）。服务器不支持 CAPA 时返回错误。
func (c *Client) CAPA() (map[string]string, error) {
	lines, err := c.cmdMulti("CAPA")
	if err != nil {
		return nil, err
	}
	caps := make(map[string]string, len(lines))
	for _, ln := range lines {
		k, v, _ := strings.Cut(ln, " ")
		caps[strings.ToUpper(k)] = v
	}
	return caps, nil
}

// Login 使用 USER/PASS 认证。
func (c *Client) Login(user, pass string) error {
	if _, err := c.cmd("USER %s", user); err != nil {
		return err
	}
	if _, err := c.cmd("PASS %s", pass); err != nil {
		return err
	}
	return nil
}

// UIDL 返回全量 UIDL 列表（空邮箱返回空切片）。
func (c *Client) UIDL() ([]UIDLEntry, error) {
	lines, err := c.cmdMulti("UIDL")
	if err != nil {
		return nil, err
	}
	out := make([]UIDLEntry, 0, len(lines))
	for _, ln := range lines {
		numStr, uid, ok := strings.Cut(ln, " ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			continue
		}
		out = append(out, UIDLEntry{Num: n, UID: strings.TrimSpace(uid)})
	}
	return out, nil
}

// Top 返回第 n 封邮件的头 lines 行正文（lines=0 时仅头部）。
// 返回的是 dot-unstuffed 的原始报文文本。
func (c *Client) Top(n int, lines int) (string, error) {
	body, err := c.cmdMulti("TOP %d %d", n, lines)
	if err != nil {
		return "", err
	}
	return strings.Join(body, "\r\n"), nil
}

func (c *Client) cmd(format string, args ...any) (string, error) {
	if err := c.writeLine(fmt.Sprintf(format, args...)); err != nil {
		return "", err
	}
	return c.readOK()
}

// cmdMulti 发送命令并读取多行响应（以 "." 结束），返回数据行。
func (c *Client) cmdMulti(format string, args ...any) ([]string, error) {
	if err := c.writeLine(fmt.Sprintf(format, args...)); err != nil {
		return nil, err
	}
	if _, err := c.readOK(); err != nil {
		return nil, err
	}
	var lines []string
	for {
		ln, err := c.readLine()
		if err != nil {
			return nil, err
		}
		if ln == "." {
			return lines, nil
		}
		// dot-unstuffing
		ln = strings.TrimPrefix(ln, ".")
		lines = append(lines, ln)
	}
}

func (c *Client) writeLine(s string) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write([]byte(s + "\r\n")); err != nil {
		return fmt.Errorf("发送命令失败: %w", err)
	}
	return nil
}

func (c *Client) readLine() (string, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	ln, err := c.r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("读取服务器响应失败: %w", err)
	}
	return strings.TrimRight(ln, "\r\n"), nil
}

// readOK 读取一行状态响应，要求以 +OK 开头。
func (c *Client) readOK() (string, error) {
	ln, err := c.readLine()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(ln, "+OK") {
		if strings.HasPrefix(ln, "-ERR") {
			return "", errors.New(strings.TrimSpace(strings.TrimPrefix(ln, "-ERR")))
		}
		return "", fmt.Errorf("服务器响应异常: %s", ln)
	}
	return ln, nil
}

// MailHeaders 是从 TOP 响应中解析出的邮件头概要。
type MailHeaders struct {
	From    string
	Subject string
	Date    time.Time
}

var wordDecoder = new(mime.WordDecoder)

// ParseHeaders 用 net/mail 解析 TOP 0 响应文本中的 From/Subject/Date，
// 并解码 RFC 2047 编码词。
func ParseHeaders(raw string) (*MailHeaders, error) {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析邮件头失败: %w", err)
	}
	h := &MailHeaders{
		Subject: decodeWord(msg.Header.Get("Subject")),
	}
	if addr, err := msg.Header.AddressList("From"); err == nil && len(addr) > 0 {
		name := decodeWord(addr[0].Name)
		if name != "" {
			h.From = name + " <" + addr[0].Address + ">"
		} else {
			h.From = addr[0].Address
		}
	} else {
		h.From = decodeWord(msg.Header.Get("From"))
	}
	if d, err := mail.ParseDate(msg.Header.Get("Date")); err == nil {
		h.Date = d
	}
	return h, nil
}

func decodeWord(s string) string {
	if s == "" {
		return ""
	}
	if d, err := wordDecoder.DecodeHeader(s); err == nil {
		return d
	}
	return s
}
