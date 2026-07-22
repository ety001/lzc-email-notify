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
