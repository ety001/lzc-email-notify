// ws.go 实现 WebSocket 推送：前端订阅后由后端在数据变更时主动推送
// accounts+events 全量快照（与 REST 响应同构，不做 diff），替代 10 秒轮询。
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/ety001/lzc-email-notify/backend/internal/account"
	"github.com/ety001/lzc-email-notify/backend/internal/events"
)

// wsDebounce 是变更信号到快照推送的去抖间隔（合并一次巡检的多次写入）。
const wsDebounce = 200 * time.Millisecond

// wsPingInterval 是服务端心跳间隔，防止网关空闲断连。
const wsPingInterval = 30 * time.Second

// Hub 按 uid 分组管理 WebSocket 连接。
type Hub struct {
	srv *Server

	mu      sync.Mutex
	conns   map[string]map[*wsConn]struct{}
	pending map[string]*time.Timer
}

type wsConn struct {
	c  *websocket.Conn
	mu sync.Mutex // 写串行化
}

func newHub(srv *Server) *Hub {
	return &Hub{
		srv:     srv,
		conns:   make(map[string]map[*wsConn]struct{}),
		pending: make(map[string]*time.Timer),
	}
}

// NotifyChange 数据变更信号（Store/Events 钩子的回调），去抖后推送。
func (h *Hub) NotifyChange(uid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.pending[uid]; ok {
		t.Stop()
	}
	h.pending[uid] = time.AfterFunc(wsDebounce, func() {
		h.mu.Lock()
		delete(h.pending, uid)
		h.mu.Unlock()
		h.broadcast(uid)
	})
}

// ServeHTTP 处理 GET /api/ws（挂在 authMiddleware 之后，uid 取自请求上下文）。
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	uid := uidOf(r)
	// Origin 校验：懒猫网关转发后 Host 变为内部地址（server:8000），
	// 与浏览器 Origin（*.heiyu.space）必然不一致，因此按域名白名单校验，
	// 防止跨站 WS 盗用网关会话；本地开发放行 localhost/127.0.0.1
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*.heiyu.space", "localhost", "localhost:*", "127.0.0.1", "127.0.0.1:*"},
	})
	if err != nil {
		log.Printf("[ws] uid=%s 握手失败: %v", uid, err)
		return
	}
	conn := &wsConn{c: c}
	h.add(uid, conn)
	defer func() {
		h.remove(uid, conn)
		c.Close(websocket.StatusNormalClosure, "")
		log.Printf("[ws] uid=%s 连接关闭", uid)
	}()
	log.Printf("[ws] uid=%s 连接建立", uid)

	// 连接建立立即推一次全量快照
	h.push(uid, conn)

	// 服务端心跳，防止网关空闲断连
	pingDone := make(chan struct{})
	defer close(pingDone)
	go h.pingLoop(conn, pingDone)

	// 读循环：仅用于检测关闭（coder/websocket 自动处理 ping/pong）
	ctx := c.CloseRead(r.Context())
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return
		}
	}
}

func (h *Hub) pingLoop(conn *wsConn, done <-chan struct{}) {
	t := time.NewTicker(wsPingInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := conn.c.Ping(ctx)
			cancel()
			if err != nil {
				conn.c.Close(websocket.StatusGoingAway, "ping failed")
				return
			}
		}
	}
}

func (h *Hub) add(uid string, conn *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[uid] == nil {
		h.conns[uid] = make(map[*wsConn]struct{})
	}
	h.conns[uid][conn] = struct{}{}
}

func (h *Hub) remove(uid string, conn *wsConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[uid], conn)
	if len(h.conns[uid]) == 0 {
		delete(h.conns, uid)
	}
}

// snapshot 构造指定 uid 的全量快照消息（与 REST 响应同构）。
func (h *Hub) snapshot(uid string) []byte {
	accounts := h.srv.store.List(uid)
	views := make([]account.View, 0, len(accounts))
	for _, a := range accounts {
		views = append(views, a.ToView())
	}
	msg := map[string]any{
		"type":     "update",
		"accounts": views,
		"events":   h.srv.events.List(uid, events.DefaultLimit),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[ws] uid=%s 快照序列化失败: %v", uid, err)
		return nil
	}
	return data
}

// push 向单个连接发送快照。
func (h *Hub) push(uid string, conn *wsConn) {
	data := h.snapshot(uid)
	if data == nil {
		return
	}
	h.write(conn, data)
}

// broadcast 向某 uid 的全部连接推送快照；写失败的连接移除。
func (h *Hub) broadcast(uid string) {
	h.mu.Lock()
	conns := make([]*wsConn, 0, len(h.conns[uid]))
	for conn := range h.conns[uid] {
		conns = append(conns, conn)
	}
	h.mu.Unlock()
	if len(conns) == 0 {
		return
	}
	data := h.snapshot(uid)
	if data == nil {
		return
	}
	for _, conn := range conns {
		if err := h.write(conn, data); err != nil {
			h.remove(uid, conn)
		}
	}
}

func (h *Hub) write(conn *wsConn, data []byte) error {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.c.Write(ctx, websocket.MessageText, data); err != nil {
		conn.c.Close(websocket.StatusInternalError, "write failed")
		return err
	}
	return nil
}
