// Package account 定义邮箱监控账号模型与基于 JSON 文件的持久化存储。
package account

import (
	"time"
)

// 协议类型
const (
	ProtocolIMAP = "imap"
	ProtocolPOP3 = "pop3"
)

// MinIntervalSec 是最小轮询间隔（秒），小于该值会被静默钳制。
const MinIntervalSec = 60

// MaxKnownUIDLs 是 POP3 已知 UIDL 集合上限，超出按插入顺序裁最旧。
const MaxKnownUIDLs = 5000

// MailInfo 描述一封邮件的概要信息。
type MailInfo struct {
	From    string `json:"from"`
	Subject string `json:"subject"`
	Date    string `json:"date"` // RFC3339
}

// Status 是账号的只读运行时状态。
type Status struct {
	Checking      bool       `json:"checking"`
	LastCheckAt   *time.Time `json:"last_check_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastError     string     `json:"last_error"`
	BaselineDone  bool       `json:"baseline_done"`
	LastMail      *MailInfo  `json:"last_mail"`
}

// State 是巡检器需要持久化的内部状态（不属于 API 契约输出）。
type State struct {
	// IMAP
	UIDValidity uint32 `json:"uid_validity,omitempty"`
	LastUID     uint32 `json:"last_uid,omitempty"`
	// POP3（按插入顺序，超出 MaxKnownUIDLs 裁最旧）
	KnownUIDLs []string `json:"known_uidls,omitempty"`
}

// Account 是邮箱监控账号。
type Account struct {
	ID          string    `json:"id"`
	UID         string    `json:"uid"` // 归属用户
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	SSL         bool      `json:"ssl"`
	Username    string    `json:"username"`
	Password    string    `json:"password,omitempty"`
	IntervalSec int       `json:"interval_sec"`
	WebURL      string    `json:"web_url"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      Status    `json:"status"`
	State       State     `json:"state,omitempty"`
}

// View 是 Account 的 API 输出形式：永不包含密码明文与内部状态，
// 以 has_password 布尔位替代。
type View struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	SSL         bool      `json:"ssl"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"`
	IntervalSec int       `json:"interval_sec"`
	WebURL      string    `json:"web_url"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Status      Status    `json:"status"`
}

// ToView 生成 API 输出视图。
func (a *Account) ToView() View {
	return View{
		ID:          a.ID,
		Name:        a.Name,
		Protocol:    a.Protocol,
		Host:        a.Host,
		Port:        a.Port,
		SSL:         a.SSL,
		Username:    a.Username,
		HasPassword: a.Password != "",
		IntervalSec: a.IntervalSec,
		WebURL:      a.WebURL,
		Enabled:     a.Enabled,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		Status:      a.Status,
	}
}

// Clone 返回账号的深拷贝（含内部状态切片），用于锁外安全使用。
func (a *Account) Clone() *Account {
	c := *a
	if a.State.KnownUIDLs != nil {
		c.State.KnownUIDLs = append([]string(nil), a.State.KnownUIDLs...)
	}
	if a.Status.LastMail != nil {
		lm := *a.Status.LastMail
		c.Status.LastMail = &lm
	}
	if a.Status.LastCheckAt != nil {
		t := *a.Status.LastCheckAt
		c.Status.LastCheckAt = &t
	}
	if a.Status.LastSuccessAt != nil {
		t := *a.Status.LastSuccessAt
		c.Status.LastSuccessAt = &t
	}
	return &c
}

// ClampInterval 将轮询间隔钳制到最小值。
func ClampInterval(sec int) int {
	if sec < MinIntervalSec {
		return MinIntervalSec
	}
	return sec
}
