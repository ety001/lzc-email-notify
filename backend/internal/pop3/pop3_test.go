package pop3

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// scriptedServer 启动一个本地假 POP3 服务器，按脚本应答。
// script 的 key 是收到的命令行（前缀匹配），返回多行响应（每元素一行）。
func scriptedServer(t *testing.T, script map[string][]string) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "+OK fake POP3 server ready\r\n")
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			cmd := line
			if i := strings.IndexByte(cmd, ' '); i >= 0 {
				cmd = cmd[:i]
			}
			resp, ok := script[strings.ToUpper(cmd)]
			if !ok {
				fmt.Fprintf(conn, "-ERR unknown command %s\r\n", cmd)
				continue
			}
			for _, l := range resp {
				fmt.Fprintf(conn, "%s\r\n", l)
			}
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestClientFlow(t *testing.T) {
	addr, cleanup := scriptedServer(t, map[string][]string{
		"CAPA": {"+OK Capability list follows", "TOP", "UIDL", "USER", "."},
		"USER": {"+OK user accepted"},
		"PASS": {"+OK pass accepted"},
		"UIDL": {"+OK 2 messages", "1 uid-aaa-111", "2 uid-bbb-222", "."},
		"TOP": {"+OK top of message follows",
			"From: =?UTF-8?B?5byg5LiJ?= <zhangsan@example.com>",
			"Subject: =?UTF-8?B?5pys5ZGo5Lya6K6u6YCa55+l?=",
			"Date: Wed, 22 Jul 2026 13:58:11 +0800",
			"",
			"."},
		"QUIT": {"+OK bye"},
	})
	defer cleanup()

	c, err := Dial(addr, false, 5*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if err := c.Login("someone@qq.com", "secret"); err != nil {
		t.Fatalf("Login: %v", err)
	}

	uidls, err := c.UIDL()
	if err != nil {
		t.Fatalf("UIDL: %v", err)
	}
	if len(uidls) != 2 || uidls[0].UID != "uid-aaa-111" || uidls[1].Num != 2 || uidls[1].UID != "uid-bbb-222" {
		t.Fatalf("unexpected UIDL result: %+v", uidls)
	}

	raw, err := c.Top(2, 0)
	if err != nil {
		t.Fatalf("TOP: %v", err)
	}
	h, err := ParseHeaders(raw)
	if err != nil {
		t.Fatalf("ParseHeaders: %v", err)
	}
	if h.From != "张三 <zhangsan@example.com>" {
		t.Fatalf("unexpected From: %q", h.From)
	}
	if h.Subject != "本周会议通知" {
		t.Fatalf("unexpected Subject: %q", h.Subject)
	}
	if h.Date.Format("2006-01-02") != "2026-07-22" {
		t.Fatalf("unexpected Date: %v", h.Date)
	}

	if err := c.Quit(); err != nil {
		t.Fatalf("Quit: %v", err)
	}
}

func TestCAPAUnsupported(t *testing.T) {
	// 服务器不支持 CAPA：客户端应继续明文工作
	addr, cleanup := scriptedServer(t, map[string][]string{
		"USER": {"+OK"},
		"PASS": {"+OK"},
		"UIDL": {"+OK", "."},
	})
	defer cleanup()

	c, err := Dial(addr, false, 5*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Login("u", "p"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	uidls, err := c.UIDL()
	if err != nil {
		t.Fatalf("UIDL: %v", err)
	}
	if len(uidls) != 0 {
		t.Fatalf("expected empty UIDL list, got %+v", uidls)
	}
}

func TestLoginRejected(t *testing.T) {
	addr, cleanup := scriptedServer(t, map[string][]string{
		"USER": {"+OK"},
		"PASS": {"-ERR authentication failed"},
	})
	defer cleanup()

	c, err := Dial(addr, false, 5*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Login("u", "bad"); err == nil {
		t.Fatal("expected login error")
	} else if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseHeadersPlain(t *testing.T) {
	h, err := ParseHeaders("From: alice@example.com\r\nSubject: hello world\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if h.From != "alice@example.com" || h.Subject != "hello world" {
		t.Fatalf("unexpected headers: %+v", h)
	}
}
