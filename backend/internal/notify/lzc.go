package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"gitee.com/linakesi/lzc-sdk/lang/go/common"
	"gitee.com/linakesi/lzc-sdk/lang/go/localdevice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// LZCSender 通过懒猫微服 API 网关向用户所有在线设备广播通知。
type LZCSender struct {
	gateway *gohelper.APIGateway
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
// 部分设备失败时合并错误返回。
func (s *LZCSender) Send(ctx context.Context, uid string, p Payload) error {
	reply, err := s.gateway.Devices.ListEndDevices(ctx, &common.ListEndDeviceRequest{Uid: uid})
	if err != nil {
		return fmt.Errorf("查询设备列表失败: %w", err)
	}
	var errs []string
	sent := 0
	for _, dev := range reply.GetDevices() {
		if !dev.GetIsOnline() || dev.GetDeviceApiUrl() == "" {
			continue
		}
		if err := s.sendToDevice(ctx, dev.GetDeviceApiUrl(), p); err != nil {
			errs = append(errs, fmt.Sprintf("设备 %s: %v", dev.GetDeviceApiUrl(), err))
			continue
		}
		sent++
	}
	if sent == 0 && len(errs) == 0 {
		log.Printf("[notify] uid=%s 无在线设备，通知已丢弃: %q", uid, p.Title)
		return nil
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
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
