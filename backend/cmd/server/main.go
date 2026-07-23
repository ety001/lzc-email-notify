// 懒猫微服「邮件提醒器」后端入口。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/api"
	"github.com/ety001/lzc-email-notify/backend/internal/events"
	"github.com/ety001/lzc-email-notify/backend/internal/notify"
	"github.com/ety001/lzc-email-notify/backend/internal/poller"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8000"
	}
	log.Printf("邮件提醒器后端版本 %s", api.Version)
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "./data"
	}

	store, err := account.Open(configDir)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("config dir: %s", configDir)

	ev := events.New()

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 15*time.Second)
	sender := notify.New(notifyCtx)
	notifyCancel()

	pm := poller.New(poller.Deps{Store: store, Events: ev, Sender: sender})
	pm.Sync()

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           api.New(store, ev, pm, sender).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		log.Printf("收到信号 %v，开始优雅退出", sig)
	case err := <-errCh:
		log.Fatalf("HTTP 服务启动失败: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 服务关闭异常: %v", err)
	}
	pm.StopAll()
	if err := store.Flush(); err != nil {
		log.Printf("配置落盘失败: %v", err)
	}
	log.Printf("已退出")
}
