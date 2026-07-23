// Package api 实现 HTTP 接口：路由、X-HC-User-ID 鉴权中间件与各端点 handler。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/events"
	"github.com/ety001/lzc-email-notify/backend/internal/mailcheck"
	"github.com/ety001/lzc-email-notify/backend/internal/notify"
)

// Syncer 抽象轮询管理器，便于测试替换。
type Syncer interface {
	Sync()
	Trigger(accountID string)
}

// Version 是后端版本号，随发布更新，通过 /api/health 暴露给前端做版本核对。
const Version = "0.2.1"

// Server 是 API 服务。
type Server struct {
	store  *account.Store
	events *events.Buffer
	poller Syncer
	sender notify.Sender
}

// New 创建 API 服务。
func New(store *account.Store, ev *events.Buffer, p Syncer, sender notify.Sender) *Server {
	return &Server{store: store, events: ev, poller: p, sender: sender}
}

type ctxKeyUID struct{}

// Handler 返回根路由。
func (s *Server) Handler() http.Handler {
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/accounts", s.listAccounts)
	authed.HandleFunc("POST /api/accounts", s.createAccount)
	authed.HandleFunc("PUT /api/accounts/{id}", s.updateAccount)
	authed.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	authed.HandleFunc("POST /api/accounts/{id}/test", s.testAccount)
	authed.HandleFunc("POST /api/test-connection", s.testConnection)
	authed.HandleFunc("POST /api/accounts/{id}/check", s.checkAccount)
	authed.HandleFunc("GET /api/events", s.listEvents)
	authed.HandleFunc("POST /api/notify/test", s.testNotify)

	root := http.NewServeMux()
	root.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": Version})
	})
	root.Handle("/api/", authMiddleware(authed))
	// Docker 镜像部署时由后端直接伺服前端产物（HashRouter，无需 SPA 回退）；
	// STATIC_DIR 为空时（如 contentdir 部署）不注册，/ 由平台 file:// 路由处理。
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		root.Handle("/", http.FileServer(http.Dir(dir)))
	}
	return accessLogMiddleware(cleanPathMiddleware(root))
}

// statusWriter 记录响应状态码，用于访问日志。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// accessLogMiddleware 输出每个请求的访问日志（方法、原始 URI、状态码、耗时）。
// 用于排查网关转发/重定向类问题：请求是否到达后端、以什么路径到达，一目了然。
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("[http] %s %s -> %d (%s)", r.Method, r.RequestURI, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// cleanPathMiddleware 把连续多个斜杠开头的路径（如 //api/accounts）归一化。
// 懒猫 ingress 在某些配置下可能转发出双斜杠路径；Go ServeMux 默认会对这种
// 路径返回 301 重定向，浏览器跟随重定向时会把 POST 变成 GET，导致创建类
// 请求被静默吞掉。在 mux 之前归一化可避免该问题。
func cleanPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; len(p) > 1 && p[0] == '/' && p[1] == '/' {
			r.URL.Path = "/" + strings.TrimLeft(p, "/")
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware 校验 X-HC-User-ID；缺失时 DEV_NOAUTH=1 回落 dev-user，否则 401。
func authMiddleware(next http.Handler) http.Handler {
	devNoAuth := os.Getenv("DEV_NOAUTH") == "1"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := r.Header.Get("X-HC-User-ID")
		if uid == "" {
			if !devNoAuth {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录或登录已过期"})
				return
			}
			uid = "dev-user"
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUID{}, uid)))
	})
}

func uidOf(r *http.Request) string {
	uid, _ := r.Context().Value(ctxKeyUID{}).(string)
	return uid
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// accountRequest 是 POST/PUT 的请求体。
type accountRequest struct {
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	SSL         bool   `json:"ssl"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	IntervalSec int    `json:"interval_sec"`
	WebURL      string `json:"web_url"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := s.store.List(uidOf(r))
	views := make([]account.View, 0, len(accounts))
	for _, a := range accounts {
		views = append(views, a.ToView())
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[api] 创建账号请求体解析失败: %v", err)
		writeError(w, http.StatusBadRequest, "请求体不是有效的 JSON")
		return
	}
	if err := validate(&req, true); err != nil {
		log.Printf("[api] 创建账号校验失败: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acc := &account.Account{
		UID:         uidOf(r),
		Name:        strings.TrimSpace(req.Name),
		Protocol:    req.Protocol,
		Host:        strings.TrimSpace(req.Host),
		Port:        req.Port,
		SSL:         req.SSL,
		Username:    strings.TrimSpace(req.Username),
		Password:    req.Password,
		IntervalSec: account.ClampInterval(req.IntervalSec),
		WebURL:      strings.TrimSpace(req.WebURL),
		Enabled:     req.Enabled,
	}
	created, err := s.store.Create(acc)
	if err != nil {
		log.Printf("[api] 创建账号失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存账号失败")
		return
	}
	s.poller.Sync()
	log.Printf("[api] 账号已创建: id=%s name=%q protocol=%s host=%s:%d", created.ID, created.Name, created.Protocol, created.Host, created.Port)
	writeJSON(w, http.StatusOK, created.ToView())
}

func validate(req *accountRequest, requirePassword bool) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("账号名称不能为空")
	}
	if req.Protocol != account.ProtocolIMAP && req.Protocol != account.ProtocolPOP3 {
		return errors.New("协议仅支持 imap 或 pop3")
	}
	if strings.TrimSpace(req.Host) == "" {
		return errors.New("服务器地址不能为空")
	}
	if req.Port < 1 || req.Port > 65535 {
		return errors.New("端口必须在 1-65535 之间")
	}
	if strings.TrimSpace(req.Username) == "" {
		return errors.New("用户名不能为空")
	}
	if requirePassword && req.Password == "" {
		return errors.New("密码/授权码不能为空")
	}
	return nil
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	uid, id := uidOf(r), r.PathValue("id")
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是有效的 JSON")
		return
	}
	// PUT 时 password 为空字符串表示不修改原密码
	if _, err := s.store.Get(uid, id); err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err := validate(&req, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.Update(uid, id, func(a *account.Account) bool {
		a.Name = strings.TrimSpace(req.Name)
		a.Protocol = req.Protocol
		a.Host = strings.TrimSpace(req.Host)
		a.Port = req.Port
		a.SSL = req.SSL
		a.Username = strings.TrimSpace(req.Username)
		if req.Password != "" {
			a.Password = req.Password
		}
		a.IntervalSec = account.ClampInterval(req.IntervalSec)
		a.WebURL = strings.TrimSpace(req.WebURL)
		a.Enabled = req.Enabled
		// 连接参数变化可能导致既有巡检状态失效，重置运行时状态与基线
		a.Status = account.Status{}
		a.State = account.State{}
		return true
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	s.poller.Sync()
	writeJSON(w, http.StatusOK, updated.ToView())
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(uidOf(r), r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	s.poller.Sync()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// testConnection 用请求体中（未保存的）账号信息做连接测试，不落库。
// 与 testAccount 的区别：面向「添加/编辑对话框里还没保存的配置」。
func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是有效的 JSON")
		return
	}
	// 连接测试只关心连通性字段，不要求名称
	if req.Protocol != account.ProtocolIMAP && req.Protocol != account.ProtocolPOP3 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "协议仅支持 imap 或 pop3"})
		return
	}
	if strings.TrimSpace(req.Host) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "服务器地址不能为空"})
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "端口必须在 1-65535 之间"})
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "用户名不能为空"})
		return
	}
	if req.Password == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "密码/授权码不能为空"})
		return
	}
	acc := &account.Account{
		Protocol: req.Protocol,
		Host:     strings.TrimSpace(req.Host),
		Port:     req.Port,
		SSL:      req.SSL,
		Username: strings.TrimSpace(req.Username),
		Password: req.Password,
	}
	if err := mailcheck.Test(r.Context(), acc); err != nil {
		log.Printf("[api] 测试连接失败: %s://%s@%s:%d ssl=%v: %v", acc.Protocol, acc.Username, acc.Host, acc.Port, acc.SSL, err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	log.Printf("[api] 测试连接成功: %s://%s@%s:%d ssl=%v", acc.Protocol, acc.Username, acc.Host, acc.Port, acc.SSL)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.store.Get(uidOf(r), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if acc.Password == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "账号未设置密码/授权码"})
		return
	}
	if err := mailcheck.Test(r.Context(), acc); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) checkAccount(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.Get(uidOf(r), r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	s.poller.Trigger(r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit := events.DefaultLimit
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > events.MaxLimit {
		limit = events.MaxLimit
	}
	writeJSON(w, http.StatusOK, s.events.List(uidOf(r), limit))
}

// testNotify 向当前登录用户的全部在线设备发送一条测试通知，
// 用于在 UI 上验证懒猫系统通知通道是否正常。
func (s *Server) testNotify(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err := s.sender.Send(ctx, uidOf(r), notify.Payload{
		Title: "邮件提醒器",
		Body:  "这是一条测试通知，说明懒猫系统通知通道工作正常",
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "测试通知发送失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
