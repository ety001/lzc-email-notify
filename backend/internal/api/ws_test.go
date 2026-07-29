package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// readUpdate 从 WS 读取一条 update 消息并解析。
func readUpdate(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal ws message: %v", err)
	}
	if msg["type"] != "update" {
		t.Fatalf("unexpected message type: %v", msg["type"])
	}
	return msg
}

func TestWebSocketPush(t *testing.T) {
	_, h, _ := newTestServer(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// 连接后应立即收到全量快照（空数据）
	msg := readUpdate(t, ctx, c)
	if accounts, _ := msg["accounts"].([]any); len(accounts) != 0 {
		t.Fatalf("expected empty accounts, got %v", msg["accounts"])
	}
	if evs, _ := msg["events"].([]any); len(evs) != 0 {
		t.Fatalf("expected empty events, got %v", msg["events"])
	}

	// REST 创建账号 → Store 变更钩子应触发推送
	do(t, h, "POST", "/api/accounts", validAccount(), nil)
	msg = readUpdate(t, ctx, c)
	accounts, _ := msg["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account after create, got %v", msg["accounts"])
	}
	acc, _ := accounts[0].(map[string]any)
	if acc["name"] != "QQ 邮箱" {
		t.Fatalf("unexpected account: %v", acc)
	}

	// 删除账号 → 再次推送空列表
	do(t, h, "DELETE", "/api/accounts/"+acc["id"].(string), nil, nil)
	msg = readUpdate(t, ctx, c)
	if accounts, _ := msg["accounts"].([]any); len(accounts) != 0 {
		t.Fatalf("expected empty accounts after delete, got %v", msg["accounts"])
	}
}

func TestWebSocketUIDIsolation(t *testing.T) {
	_, h, _ := newTestServer(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 两个不同 uid 的连接（DEV_NOAUTH 下默认 dev-user；另一个显式 u-other）
	c1, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial c1: %v", err)
	}
	defer c1.Close(websocket.StatusNormalClosure, "")
	c2, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"X-HC-User-ID": {"u-other"}},
	})
	if err != nil {
		t.Fatalf("dial c2: %v", err)
	}
	defer c2.Close(websocket.StatusNormalClosure, "")

	readUpdate(t, ctx, c1) // 初始快照
	readUpdate(t, ctx, c2)

	// dev-user 创建账号：c1 收到推送；c2 在等待窗口内不应收到任何消息
	do(t, h, "POST", "/api/accounts", validAccount(), nil)
	msg := readUpdate(t, ctx, c1)
	if accounts, _ := msg["accounts"].([]any); len(accounts) != 1 {
		t.Fatalf("c1 expected 1 account, got %v", msg["accounts"])
	}

	shortCtx, shortCancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer shortCancel()
	if _, _, err := c2.Read(shortCtx); err == nil {
		t.Fatal("c2 should not receive updates for other uid")
	}
}
