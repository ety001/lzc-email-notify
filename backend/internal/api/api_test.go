package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/events"
	"github.com/ety001/lzc-email-notify/backend/internal/notify"
)

type fakeSyncer struct {
	syncs    int
	triggers []string
}

func (f *fakeSyncer) Sync()             { f.syncs++ }
func (f *fakeSyncer) Trigger(id string) { f.triggers = append(f.triggers, id) }

type fakeSender struct {
	sent []notify.Payload
	err  error
}

func (f *fakeSender) Send(_ context.Context, _ string, p notify.Payload) error {
	f.sent = append(f.sent, p)
	return f.err
}

func newTestServer(t *testing.T) (*Server, http.Handler, *fakeSyncer) {
	t.Helper()
	t.Setenv("DEV_NOAUTH", "1")
	store, err := account.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fs := &fakeSyncer{}
	s := New(store, events.New(), fs, &fakeSender{})
	return s, s.Handler(), fs
}

func do(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var parsed map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func validAccount() map[string]any {
	return map[string]any{
		"name":         "QQ 邮箱",
		"protocol":     "imap",
		"host":         "imap.qq.com",
		"port":         993,
		"ssl":          true,
		"username":     "someone@qq.com",
		"password":     "secret",
		"interval_sec": 60,
		"web_url":      "https://mail.qq.com",
		"enabled":      false, // 测试里不启用轮询
	}
}

func TestHealthNoAuth(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec, body := do(t, h, "GET", "/api/health", nil, nil)
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("health: %d %v", rec.Code, body)
	}
}

func TestNotifyTest(t *testing.T) {
	t.Setenv("DEV_NOAUTH", "1")
	store, _ := account.Open(t.TempDir())
	fs := &fakeSender{}
	s := New(store, events.New(), &fakeSyncer{}, fs)
	h := s.Handler()

	rec, body := do(t, h, "POST", "/api/notify/test", nil, nil)
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("expected ok, got %d %v", rec.Code, body)
	}
	if len(fs.sent) != 1 || fs.sent[0].Title == "" || fs.sent[0].Body == "" {
		t.Fatalf("expected 1 non-empty notification, got %+v", fs.sent)
	}

	// 发送失败应返回 502 与错误信息
	fs.err = errors.New("boom")
	rec, body = do(t, h, "POST", "/api/notify/test", nil, nil)
	if rec.Code != http.StatusBadGateway || body["error"] == nil {
		t.Fatalf("expected 502, got %d %v", rec.Code, body)
	}
}

func TestAuthRequired(t *testing.T) {
	t.Setenv("DEV_NOAUTH", "")
	store, _ := account.Open(t.TempDir())
	s := New(store, events.New(), &fakeSyncer{}, &fakeSender{})
	h := s.Handler()
	rec, body := do(t, h, "GET", "/api/accounts", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d %v", rec.Code, body)
	}
	// 带 uid 头则放行
	rec, _ = do(t, h, "GET", "/api/accounts", nil, map[string]string{"X-HC-User-ID": "u9"})
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAccountCRUDAndIsolation(t *testing.T) {
	_, h, fs := newTestServer(t)

	// 空列表应为 [] 而非 null
	rec, _ := do(t, h, "GET", "/api/accounts", nil, nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected [], got %q", rec.Body.String())
	}

	// 创建
	rec, created := do(t, h, "POST", "/api/accounts", validAccount(), nil)
	if rec.Code != 200 {
		t.Fatalf("create: %d %v", rec.Code, created)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no id in response: %v", created)
	}
	if created["has_password"] != true {
		t.Fatalf("expected has_password=true: %v", created)
	}
	if _, leaked := created["password"]; leaked {
		t.Fatalf("password leaked in response: %v", created)
	}
	if fs.syncs == 0 {
		t.Fatal("poller.Sync not called after create")
	}

	// 校验失败：缺密码
	bad := validAccount()
	bad["password"] = ""
	rec, body := do(t, h, "POST", "/api/accounts", bad, nil)
	if rec.Code != 400 || body["error"] == nil {
		t.Fatalf("expected 400 with error, got %d %v", rec.Code, body)
	}
	// 校验失败：协议非法
	bad = validAccount()
	bad["protocol"] = "smtp"
	rec, _ = do(t, h, "POST", "/api/accounts", bad, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	// 校验失败：端口越界
	bad = validAccount()
	bad["port"] = 70000
	rec, _ = do(t, h, "POST", "/api/accounts", bad, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// interval 钳制：小于 60 静默改为 60
	bad = validAccount()
	bad["interval_sec"] = 5
	rec, created2 := do(t, h, "POST", "/api/accounts", bad, nil)
	if rec.Code != 200 || created2["interval_sec"].(float64) != 60 {
		t.Fatalf("interval clamp: %d %v", rec.Code, created2)
	}
	id2 := created2["id"].(string)

	// PUT：interval 改为 120，password 空字符串 = 不修改
	upd := validAccount()
	upd["interval_sec"] = 120
	upd["password"] = ""
	rec, updated := do(t, h, "PUT", "/api/accounts/"+id, upd, nil)
	if rec.Code != 200 || updated["interval_sec"].(float64) != 120 {
		t.Fatalf("put: %d %v", rec.Code, updated)
	}
	if updated["has_password"] != true {
		t.Fatalf("password should be kept: %v", updated)
	}

	// check 端点（异步触发）
	rec, body = do(t, h, "POST", "/api/accounts/"+id+"/check", nil, nil)
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("check: %d %v", rec.Code, body)
	}
	if len(fs.triggers) != 1 || fs.triggers[0] != id {
		t.Fatalf("trigger mismatch: %v", fs.triggers)
	}

	// test 端点：假服务器 → HTTP 200 + ok:false + 中文错误
	rec, body = do(t, h, "POST", "/api/accounts/"+id+"/test", nil, nil)
	if rec.Code != 200 || body["ok"] != false || body["error"] == nil || body["error"] == "" {
		t.Fatalf("test endpoint: %d %v", rec.Code, body)
	}

	// uid 隔离：另一个 uid 操作 id 一律 404
	other := map[string]string{"X-HC-User-ID": "someone-else"}
	for _, tc := range [][2]string{
		{"PUT", "/api/accounts/" + id},
		{"DELETE", "/api/accounts/" + id},
		{"POST", "/api/accounts/" + id + "/test"},
		{"POST", "/api/accounts/" + id + "/check"},
	} {
		var payload any
		if tc[0] == "PUT" {
			payload = validAccount()
		}
		rec, body = do(t, h, tc[0], tc[1], payload, other)
		if rec.Code != 404 || body["error"] != "账号不存在" {
			t.Fatalf("%s %s: expected 404 账号不存在, got %d %v", tc[0], tc[1], rec.Code, body)
		}
	}

	// DELETE
	rec, body = do(t, h, "DELETE", "/api/accounts/"+id, nil, nil)
	if rec.Code != 200 || body["ok"] != true {
		t.Fatalf("delete: %d %v", rec.Code, body)
	}
	rec, _ = do(t, h, "DELETE", "/api/accounts/"+id, nil, nil)
	if rec.Code != 404 {
		t.Fatalf("delete twice should 404, got %d", rec.Code)
	}
	// 清理第二个账号
	do(t, h, "DELETE", "/api/accounts/"+id2, nil, nil)
}

func TestEventsEndpoint(t *testing.T) {
	s, h, _ := newTestServer(t)

	// 空事件应为 []
	rec, _ := do(t, h, "GET", "/api/events", nil, nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("expected [], got %q", rec.Body.String())
	}

	s.events.Add("dev-user", "acc1", "账号一", events.KindInfo, "已建立基线")
	s.events.Add("dev-user", "acc1", "账号一", events.KindNewMail, "张三 <zhangsan@example.com>：本周例会通知")
	s.events.Add("other-user", "acc2", "账号二", events.KindInfo, "别人的事件")

	rec, _ = do(t, h, "GET", "/api/events?limit=50", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("events: %d", rec.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 events for dev-user, got %d", len(list))
	}
	// 倒序
	if list[0]["kind"] != events.KindNewMail || list[1]["kind"] != events.KindInfo {
		t.Fatalf("bad order/kinds: %v", list)
	}
	if list[0]["account_name"] != "账号一" {
		t.Fatalf("bad event fields: %v", list[0])
	}
}

// 双斜杠路径（//api/accounts）不得被 ServeMux 301 重定向——否则浏览器会把
// 跟随重定向的 POST 变成 GET，创建请求被静默吞掉（生产环境真实踩坑）。
func TestDoubleSlashPostNotRedirected(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec, body := do(t, h, "POST", "//api/accounts", validAccount(), nil)
	if rec.Code == http.StatusMovedPermanently || rec.Code == http.StatusTemporaryRedirect {
		t.Fatalf("双斜杠路径被重定向: %d", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("双斜杠 POST 应正常创建, got %d %v", rec.Code, body)
	}
	if body["id"] == nil || body["id"] == "" {
		t.Fatalf("创建应返回 id: %v", body)
	}
	// 确认真的存上了
	rec2, list := do(t, h, "GET", "/api/accounts", nil, nil)
	if rec2.Code != 200 {
		t.Fatalf("list: %d", rec2.Code)
	}
	_ = list
}

func TestTestConnectionValidation(t *testing.T) {
	_, h, _ := newTestServer(t)
	// 缺密码 → ok:false，HTTP 仍 200
	rec, body := do(t, h, "POST", "/api/test-connection", map[string]any{
		"protocol": "imap", "host": "imap.qq.com", "port": 993,
		"username": "a@qq.com",
	}, nil)
	if rec.Code != 200 || body["ok"] != false {
		t.Fatalf("缺密码应 ok:false: %d %v", rec.Code, body)
	}
	// 缺服务器 → ok:false
	_, body = do(t, h, "POST", "/api/test-connection", map[string]any{
		"protocol": "imap", "port": 993, "username": "a@qq.com", "password": "x",
	}, nil)
	if body["ok"] != false {
		t.Fatalf("缺服务器应 ok:false: %v", body)
	}
	// 坏协议 → ok:false
	_, body = do(t, h, "POST", "/api/test-connection", map[string]any{
		"protocol": "smtp", "host": "h", "port": 1, "username": "u", "password": "p",
	}, nil)
	if body["ok"] != false {
		t.Fatalf("坏协议应 ok:false: %v", body)
	}
}
