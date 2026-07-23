// Package notify 提供懒猫系统通知发送能力。
// Sender 语义：向指定 uid 的所有在线客户端设备广播一条通知。
package notify

import (
	"context"
	"log"
)

// Payload 是一条通知的内容。
type Payload struct {
	Title       string
	Body        string
	DeeplinkURL string // 为空则通知不带跳转链接
}

// Sender 向 uid 的全部在线设备发送通知。
type Sender interface {
	Send(ctx context.Context, uid string, p Payload) error
}

// DeviceInfo 是一台可通知设备的概要。
type DeviceInfo struct {
	ID         string `json:"id"` // 懒猫 unique_deivce_id
	Name       string `json:"name"`
	RemarkName string `json:"remark_name"`
	Model      string `json:"model"`
	Online     bool   `json:"online"`
	IsMobile   bool   `json:"is_mobile"`
	IsTV       bool   `json:"is_tv"`
}

// UserInfo 是懒猫账号概要。
type UserInfo struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// DeviceManager 是懒猫环境特有的能力：设备/用户查询与定向通知。
// 非懒猫环境的回落实现（LogSender）不实现该接口，
// 调用方通过类型断言判断能力是否可用。
type DeviceManager interface {
	ListDevices(ctx context.Context, uid string) ([]DeviceInfo, error)
	QueryUser(ctx context.Context, uid string) (*UserInfo, error)
	// SendToDevices 仅向 deviceIDs 列出的在线设备发送；空列表表示不发送。
	SendToDevices(ctx context.Context, uid string, deviceIDs []string, p Payload) error
}

// LogSender 是回落实现：懒猫环境不可用时仅打日志。
type LogSender struct{}

// Send 实现 Sender。
func (LogSender) Send(_ context.Context, uid string, p Payload) error {
	log.Printf("[notify:log] uid=%s title=%q body=%q deeplink=%q", uid, p.Title, p.Body, p.DeeplinkURL)
	return nil
}

// New 探测懒猫运行环境：可用则返回 LZCSender，否则回落 LogSender。
func New(ctx context.Context) Sender {
	s, err := newLZCSender(ctx)
	if err != nil {
		log.Printf("[notify] 懒猫环境不可用（%v），通知将仅输出到日志", err)
		return LogSender{}
	}
	log.Printf("[notify] 已接入懒猫系统通知")
	return s
}
