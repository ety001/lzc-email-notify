package notify

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// fakeCapable 是测试用的 CapabilityReporter 实现。
type fakeCapable struct {
	LogSender
	ids []string
}

func (f fakeCapable) UnsupportedNotifyDevices() []string { return f.ids }

func TestUnsupportedDevicesOf(t *testing.T) {
	if got := UnsupportedDevicesOf(LogSender{}); got != nil {
		t.Fatalf("LogSender 应返回 nil，got %v", got)
	}
	if got := UnsupportedDevicesOf(fakeCapable{}); got != nil {
		t.Fatalf("空记录应返回 nil，got %v", got)
	}
	got := UnsupportedDevicesOf(fakeCapable{ids: []string{"d1", "d2"}})
	if !got["d1"] || !got["d2"] || len(got) != 2 {
		t.Fatalf("应包含 d1/d2，got %v", got)
	}
	// AutoSender 解包
	a := &AutoSender{cur: fakeCapable{ids: []string{"d1"}}}
	if got := UnsupportedDevicesOf(a); !got["d1"] {
		t.Fatalf("AutoSender 应解包，got %v", got)
	}
}

func TestIsNotifyUnimplemented(t *testing.T) {
	unimpl := status.Error(codes.Unimplemented, "Notify is not registered")
	if !isNotifyUnimplemented(unimpl) {
		t.Fatal("原始 Unimplemented 应识别为不支持")
	}
	// sendToDevice 链路会用 fmt.Errorf %w 逐层包装
	wrapped := fmt.Errorf("发送通知失败: %w", unimpl)
	if !isNotifyUnimplemented(wrapped) {
		t.Fatal("包装后的 Unimplemented 应识别为不支持")
	}
	if isNotifyUnimplemented(status.Error(codes.DeadlineExceeded, "timeout")) {
		t.Fatal("DeadlineExceeded 不应识别为不支持")
	}
	if isNotifyUnimplemented(fmt.Errorf("普通错误")) {
		t.Fatal("非 gRPC 错误不应识别为不支持")
	}
}
