package mailcheck

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCheckProxyURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"http://127.0.0.1:7890", false},
		{"http://user:pass@proxy.local:8080", false},
		{"https://proxy.local", false},
		{"socks5://127.0.0.1:1080", true},
		{"127.0.0.1:7890", true}, // 缺 scheme，按绝对地址解析后 Host 为空
		{"", true},
		{"http://", true},
	}
	for _, c := range cases {
		err := CheckProxyURL(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("CheckProxyURL(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

// fakeConnectProxy 是一个最小 CONNECT 代理：校验请求后回 200 并回显标记。
func fakeConnectProxy(t *testing.T, ln net.Listener, wantAuth bool) {
	t.Helper()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			br := bufio.NewReader(conn)
			req, err := http.ReadRequest(br)
			if err != nil || req.Method != http.MethodConnect {
				conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
				return
			}
			if wantAuth && req.Header.Get("Proxy-Authorization") == "" {
				conn.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
				return
			}
			conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			// 隧道建立后回显一行标记，证明隧道可用
			conn.Write([]byte("TUNNEL-OK " + req.Host + "\r\n"))
			io.Copy(io.Discard, conn)
		}()
	}
}

func TestDialViaProxy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go fakeConnectProxy(t, ln, true)

	conn, err := dialViaProxy("http://user:pass@"+ln.Addr().String(), "imap.example.com:993", 3*time.Second)
	if err != nil {
		t.Fatalf("dialViaProxy: %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 64)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read tunnel: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "TUNNEL-OK imap.example.com:993") {
		t.Fatalf("unexpected tunnel data: %q", buf[:n])
	}
}

func TestDialViaProxyAuthRequired(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go fakeConnectProxy(t, ln, true)

	// 不带凭据：代理应返回 407
	_, err = dialViaProxy("http://"+ln.Addr().String(), "imap.example.com:993", 3*time.Second)
	if err == nil || !strings.Contains(err.Error(), "407") {
		t.Fatalf("expected 407 error, got %v", err)
	}
}
