// Package notify 提供懒猫系统通知发送能力。
// Sender 语义：向指定 uid 的所有在线客户端设备广播一条通知。
package notify

import (
	"context"
	"log"
	"sync"
	"time"
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
	// NotifyUnsupported 为 true 表示发送时发现该设备未实现系统通知能力
	//（如无界面的 hclient-cli），发送时会被跳过。
	NotifyUnsupported bool `json:"notify_unsupported,omitempty"`
}

// UserInfo 是懒猫账号概要。
type UserInfo struct {
	UID      string `json:"uid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// DeviceManager 是懒猫环境特有的能力：设备/用户查询与定向通知。
// 非懒猫环境的回落实现（LogSender）不实现该接口，
// 调用方通过 DeviceManagerOf 判断能力是否可用。
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

// retryInterval 是懒猫环境探测失败后的后台重试间隔。
const retryInterval = 30 * time.Second

// AutoSender 是带后台重试的 Sender：懒猫环境暂不可用时回落 LogSender，
// 后台每 retryInterval 重试接入，成功后自动切换为 LZCSender。
// 用于容忍懒猫平台侧 API 网关注入晚于应用启动的竞态。
type AutoSender struct {
	mu  sync.RWMutex
	cur Sender
}

// New 返回 AutoSender：立即开始探测懒猫环境，失败则后台重试直到成功。
// ctx 取消后重试停止。
func New(ctx context.Context) Sender {
	a := &AutoSender{cur: LogSender{}}
	go a.probeLoop(ctx)
	return a
}

func (a *AutoSender) current() Sender {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cur
}

// Send 实现 Sender。
func (a *AutoSender) Send(ctx context.Context, uid string, p Payload) error {
	return a.current().Send(ctx, uid, p)
}

func (a *AutoSender) probeLoop(ctx context.Context) {
	first := true
	for {
		s, err := newLZCSender(ctx)
		if err == nil {
			a.mu.Lock()
			a.cur = s
			a.mu.Unlock()
			log.Printf("[notify] 已接入懒猫系统通知")
			return
		}
		if first {
			log.Printf("[notify] 懒猫环境暂不可用（%v），通知暂时仅输出到日志，将每 %v 后台重试", err, retryInterval)
			first = false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// CapabilityReporter 报告发送过程中发现的不支持系统通知的设备。
type CapabilityReporter interface {
	// UnsupportedNotifyDevices 返回已知不支持系统通知的设备 ID 列表。
	UnsupportedNotifyDevices() []string
}

// UnsupportedDevicesOf 提取 Sender 已发现的不支持系统通知的设备集合；
// Sender 不具备该能力或无记录时返回 nil。AutoSender 自动解包。
func UnsupportedDevicesOf(s Sender) map[string]bool {
	if a, ok := s.(*AutoSender); ok {
		s = a.current()
	}
	r, ok := s.(CapabilityReporter)
	if !ok {
		return nil
	}
	ids := r.UnsupportedNotifyDevices()
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// DeviceManagerOf 提取 Sender 的设备管理能力。
// AutoSender 在尚未接入懒猫环境时返回 false，接入成功后自动可用。
func DeviceManagerOf(s Sender) (DeviceManager, bool) {
	if a, ok := s.(*AutoSender); ok {
		s = a.current()
	}
	dm, ok := s.(DeviceManager)
	return dm, ok
}
