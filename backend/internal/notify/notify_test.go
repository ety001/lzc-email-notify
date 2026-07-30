package notify

import (
	"context"
	"testing"
)

// fakeDM 是测试用的 DeviceManager 实现。
type fakeDM struct{ LogSender }

func (fakeDM) ListDevices(_ context.Context, _ string) ([]DeviceInfo, error) { return nil, nil }
func (fakeDM) QueryUser(_ context.Context, uid string) (*UserInfo, error) {
	return &UserInfo{UID: uid}, nil
}
func (fakeDM) SendToDevices(_ context.Context, _ string, _ []string, _ Payload) error {
	return nil
}

func TestDeviceManagerOf(t *testing.T) {
	// 普通回落 sender：无设备管理能力
	if _, ok := DeviceManagerOf(LogSender{}); ok {
		t.Fatal("LogSender 不应具备设备管理能力")
	}
	// 直接实现了 DeviceManager 的 sender
	if _, ok := DeviceManagerOf(fakeDM{}); !ok {
		t.Fatal("fakeDM 应具备设备管理能力")
	}
}

func TestDeviceManagerOfAutoSender(t *testing.T) {
	// AutoSender 未接入懒猫环境（cur 为 LogSender）时返回 false
	a := &AutoSender{cur: LogSender{}}
	if _, ok := DeviceManagerOf(a); ok {
		t.Fatal("未接入懒猫环境的 AutoSender 不应具备设备管理能力")
	}
	// 接入成功（cur 切换为具备 DeviceManager 的实现）后返回 true
	a.cur = fakeDM{}
	if _, ok := DeviceManagerOf(a); !ok {
		t.Fatal("已接入的 AutoSender 应具备设备管理能力")
	}
}
