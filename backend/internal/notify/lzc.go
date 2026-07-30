package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"gitee.com/linakesi/lzc-sdk/lang/go/common"
	"gitee.com/linakesi/lzc-sdk/lang/go/localdevice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// LZCSender 通过懒猫微服 API 网关向用户所有在线设备广播通知。
type LZCSender struct {
	gateway *gohelper.APIGateway

	mu          sync.RWMutex
	unsupported map[string]struct{} // 发送时发现未实现 NotificationService 的设备
}

func newLZCSender(ctx context.Context) (*LZCSender, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gateway, err := gohelper.NewAPIGateway(cctx)
	if err != nil {
		return nil, err
	}
	return &LZCSender{gateway: gateway}, nil
}

// Close 释放网关连接。
func (s *LZCSender) Close() error { return s.gateway.Close() }

// Send 向 uid 的所有在线设备广播。全部设备离线时返回 nil；
// 部分设备送达即视为成功（失败明细记日志）；一台都未送达时才返回错误。
func (s *LZCSender) Send(ctx context.Context, uid string, p Payload) error {
	return s.sendFiltered(ctx, uid, nil, p)
}

// SendToDevices 仅向 deviceIDs 列出的在线设备发送（实现 DeviceManager）。
// 错误语义与 Send 相同。
func (s *LZCSender) SendToDevices(ctx context.Context, uid string, deviceIDs []string, p Payload) error {
	if len(deviceIDs) == 0 {
		log.Printf("[notify] uid=%s 设备过滤为空，通知已丢弃: %q", uid, p.Title)
		return nil
	}
	allow := make(map[string]struct{}, len(deviceIDs))
	for _, id := range deviceIDs {
		allow[id] = struct{}{}
	}
	return s.sendFiltered(ctx, uid, allow, p)
}

// sendFiltered 是 Send/SendToDevices 的公共实现；allow 为 nil 表示不过滤。
// 各目标设备并行发送；不支持系统通知的设备（Unimplemented）跳过并记录，
// 不计入失败。
func (s *LZCSender) sendFiltered(ctx context.Context, uid string, allow map[string]struct{}, p Payload) error {
	reply, err := s.gateway.Devices.ListEndDevices(ctx, &common.ListEndDeviceRequest{Uid: uid})
	if err != nil {
		return fmt.Errorf("查询设备列表失败: %w", err)
	}
	type target struct{ id, apiURL string }
	var targets []target
	for _, dev := range reply.GetDevices() {
		if !dev.GetIsOnline() || dev.GetDeviceApiUrl() == "" {
			continue
		}
		if allow != nil {
			if _, ok := allow[dev.GetUniqueDeivceId()]; !ok {
				continue
			}
		}
		targets = append(targets, target{dev.GetUniqueDeivceId(), dev.GetDeviceApiUrl()})
	}
	if len(targets) == 0 {
		log.Printf("[notify] uid=%s 无在线设备，通知已丢弃: %q", uid, p.Title)
		return nil
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	sent, skipped := 0, 0
	var errs []string
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			err := s.sendToDevice(ctx, t.apiURL, p)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				sent++
			case isNotifyUnimplemented(err):
				// 设备本地 API 未实现 NotificationService（如无界面的
				// hclient-cli）：不是故障，跳过并记录，设置页据此标记
				skipped++
				s.markUnsupported(t.id)
				log.Printf("[notify] 设备 %s 不支持系统通知，已跳过", t.apiURL)
			default:
				errs = append(errs, fmt.Sprintf("设备 %s: %v", t.apiURL, err))
			}
		}(t)
	}
	wg.Wait()

	if sent > 0 {
		if len(errs) > 0 || skipped > 0 {
			log.Printf("[notify] uid=%s 通知部分送达（成功 %d，不支持 %d，失败 %d: %s）: %q",
				uid, sent, skipped, len(errs), strings.Join(errs, "; "), p.Title)
		}
		return nil
	}
	if len(errs) > 0 {
		if skipped > 0 {
			errs = append(errs, fmt.Sprintf("另有 %d 台设备不支持系统通知", skipped))
		}
		return errors.New(strings.Join(errs, "; "))
	}
	return fmt.Errorf("没有可通知的设备：%d 台目标在线设备均不支持系统通知", skipped)
}

// UnsupportedNotifyDevices 实现 CapabilityReporter。
func (s *LZCSender) UnsupportedNotifyDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.unsupported))
	for id := range s.unsupported {
		out = append(out, id)
	}
	return out
}

// markUnsupported 记录不支持系统通知的设备。
func (s *LZCSender) markUnsupported(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsupported == nil {
		s.unsupported = make(map[string]struct{})
	}
	s.unsupported[id] = struct{}{}
}

// isNotifyUnimplemented 判断错误是否为设备端未实现 NotificationService。
func isNotifyUnimplemented(err error) bool {
	var gs interface{ GRPCStatus() *status.Status }
	if errors.As(err, &gs) {
		return gs.GRPCStatus().Code() == codes.Unimplemented
	}
	return false
}

// ListDevices 返回 uid 名下的全部设备（实现在线状态，实现 DeviceManager）。
func (s *LZCSender) ListDevices(ctx context.Context, uid string) ([]DeviceInfo, error) {
	reply, err := s.gateway.Devices.ListEndDevices(ctx, &common.ListEndDeviceRequest{Uid: uid})
	if err != nil {
		return nil, fmt.Errorf("查询设备列表失败: %w", err)
	}
	out := make([]DeviceInfo, 0, len(reply.GetDevices()))
	for _, dev := range reply.GetDevices() {
		out = append(out, DeviceInfo{
			ID:         dev.GetUniqueDeivceId(),
			Name:       dev.GetName(),
			RemarkName: dev.GetRemarkName(),
			Model:      dev.GetModel(),
			Online:     dev.GetIsOnline(),
			IsMobile:   dev.GetIsMobile(),
			IsTV:       dev.GetIsTv(),
		})
	}
	return out, nil
}

// QueryUser 查询懒猫账号信息（实现 DeviceManager）。
func (s *LZCSender) QueryUser(ctx context.Context, uid string) (*UserInfo, error) {
	info, err := s.gateway.Users.QueryUserInfo(ctx, &common.UserID{Uid: uid})
	if err != nil {
		return nil, fmt.Errorf("查询用户信息失败: %w", err)
	}
	return &UserInfo{
		UID:      info.GetUid(),
		Nickname: info.GetNickname(),
		Avatar:   info.GetAvatar(),
	}, nil
}

func (s *LZCSender) sendToDevice(ctx context.Context, deviceAPIURL string, p Payload) error {
	parsedURL, err := url.Parse(deviceAPIURL)
	if err != nil {
		return fmt.Errorf("设备地址无效: %w", err)
	}
	cred, err := gohelper.BuildClientCredOption(gohelper.CAPath, gohelper.APPKeyPath, gohelper.APPCertPath)
	if err != nil {
		return fmt.Errorf("加载证书失败: %w", err)
	}

	dial := func() (*grpc.ClientConn, error) {
		dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return grpc.DialContext(dctx, parsedURL.Host, grpc.WithBlock(), cred)
	}

	authConn, err := dial()
	if err != nil {
		return fmt.Errorf("连接设备失败: %w", err)
	}
	token, err := gohelper.RequestAuthToken(ctx, authConn)
	authConn.Close()
	if err != nil {
		return fmt.Errorf("获取设备授权失败: %w", err)
	}

	conn, err := dial()
	if err != nil {
		return fmt.Errorf("连接设备失败: %w", err)
	}
	defer conn.Close()

	req := &localdevice.NotifyRequest{Title: p.Title, Body: p.Body}
	if p.DeeplinkURL != "" {
		req.DeeplinkUrl = &p.DeeplinkURL
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "lzc_dapi_auth_token", token.Token)
	if _, err := localdevice.NewNotificationServiceClient(conn).Notify(ctx, req); err != nil {
		return fmt.Errorf("发送通知失败: %w", err)
	}
	return nil
}
