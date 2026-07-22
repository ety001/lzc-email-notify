package account

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, dir
}

func sample(uid string) *Account {
	return &Account{
		UID:         uid,
		Name:        "QQ 邮箱",
		Protocol:    ProtocolIMAP,
		Host:        "imap.qq.com",
		Port:        993,
		SSL:         true,
		Username:    "someone@qq.com",
		Password:    "secret",
		IntervalSec: 60,
		WebURL:      "https://mail.qq.com",
		Enabled:     true,
	}
}

func TestCreateListGetUpdateDelete(t *testing.T) {
	s, dir := newTestStore(t)

	created, err := s.Create(sample("u1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" || len(created.ID) != 16 {
		t.Fatalf("bad id: %q", created.ID)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("timestamps not set")
	}

	// uid 隔离
	if _, err := s.Get("u2", created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for other uid, got %v", err)
	}
	if list := s.List("u2"); len(list) != 0 {
		t.Fatalf("expected empty list for u2, got %d", len(list))
	}

	// 更新并持久化
	if _, err := s.Update("u1", created.ID, func(a *Account) bool {
		a.IntervalSec = 120
		a.Status.BaselineDone = true
		return true
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// 重新加载验证落盘
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := s2.Get("u1", created.ID)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.IntervalSec != 120 || !got.Status.BaselineDone || got.Password != "secret" {
		t.Fatalf("unexpected reloaded account: %+v", got)
	}

	// 文件权限 0600
	fi, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("config.json perm = %o, want 600", fi.Mode().Perm())
	}

	// 顶层结构为 {"accounts":[...]}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["accounts"]; !ok {
		t.Fatalf("config.json missing accounts key: %s", raw)
	}

	// 删除 + uid 隔离
	if err := s.Delete("u2", created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting as u2, got %v", err)
	}
	if err := s.Delete("u1", created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("u1", created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestViewNeverContainsPassword(t *testing.T) {
	a := sample("u1")
	v := a.ToView()
	if !v.HasPassword {
		t.Fatal("HasPassword should be true")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["password"]; ok {
		t.Fatalf("view must not contain password: %s", raw)
	}
	if _, ok := m["has_password"]; !ok {
		t.Fatalf("view must contain has_password: %s", raw)
	}
}

func TestCloneDeepCopiesState(t *testing.T) {
	a := sample("u1")
	a.State.KnownUIDLs = []string{"x", "y"}
	lm := &MailInfo{From: "f", Subject: "s", Date: "d"}
	a.Status.LastMail = lm
	c := a.Clone()
	c.State.KnownUIDLs[0] = "changed"
	c.Status.LastMail.From = "changed"
	if a.State.KnownUIDLs[0] != "x" || a.Status.LastMail.From != "f" {
		t.Fatal("Clone is not a deep copy")
	}
}

func TestClampInterval(t *testing.T) {
	if ClampInterval(0) != 60 || ClampInterval(-5) != 60 || ClampInterval(30) != 60 || ClampInterval(120) != 120 {
		t.Fatal("ClampInterval wrong")
	}
}
