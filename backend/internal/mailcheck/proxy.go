// proxy.go 实现 HTTP CONNECT 代理隧道：IMAP/POP3 这类裸 TCP 协议
// 通过 HTTP 代理访问时，先与代理建立 CONNECT 隧道，再在隧道上
// 运行（可选的）TLS 与邮件协议。
package mailcheck

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CheckProxyURL 校验代理地址格式：必须是 http/https 绝对地址。
// 供 API 层在保存账号前校验。
func CheckProxyURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("代理地址格式无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("代理协议仅支持 http 或 https")
	}
	return nil
}

// dialViaProxy 通过 HTTP CONNECT 代理建立到 target（host:port）的隧道连接。
// proxyURL 形如 http://[user:pass@]host:port；https 代理会先与代理做 TLS。
func dialViaProxy(proxyURL, target string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return nil, errors.New("代理地址解析失败")
	}
	proxyAddr := u.Host
	if !strings.Contains(proxyAddr, ":") {
		if u.Scheme == "https" {
			proxyAddr += ":443"
		} else {
			proxyAddr += ":80"
		}
	}

	d := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	if u.Scheme == "https" {
		conn, err = tls.DialWithDialer(d, "tcp", proxyAddr, &tls.Config{ServerName: u.Hostname()})
	} else {
		conn, err = d.Dial("tcp", proxyAddr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接代理服务器失败: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if u.User != nil {
		pass, _ := u.User.Password()
		cred := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + pass))
		sb.WriteString("Proxy-Authorization: Basic " + cred + "\r\n")
	}
	sb.WriteString("\r\n")

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte(sb.String())); err != nil {
		conn.Close()
		return nil, fmt.Errorf("代理 CONNECT 请求发送失败: %w", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("代理 CONNECT 响应读取失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("代理 CONNECT 失败: %s", resp.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	// ReadResponse 只消费响应头，缓冲里可能已有隧道数据，需保留
	return &bufferedConn{Conn: conn, r: br}, nil
}

// bufferedConn 让后续的协议读取先走 bufio.Reader（保留 CONNECT 响应
// 解析时预读的隧道字节）。
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
