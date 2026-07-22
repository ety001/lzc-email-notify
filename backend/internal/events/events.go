// Package events 提供按 uid 隔离的内存环形事件缓冲（容量 200，不持久化）。
package events

import (
	"sync"
	"time"
)

// 事件类型
const (
	KindNewMail      = "new_mail"
	KindNotifyFailed = "notify_failed"
	KindCheckFailed  = "check_failed"
	KindInfo         = "info"
)

// Capacity 是环形缓冲容量。
const Capacity = 200

// DefaultLimit 与 MaxLimit 是查询条数限制。
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Event 是一条事件记录。
type Event struct {
	ID          int64     `json:"id"`
	Time        time.Time `json:"time"`
	AccountID   string    `json:"account_id"`
	AccountName string    `json:"account_name"`
	Kind        string    `json:"kind"`
	Detail      string    `json:"detail"`

	uid string
}

// Buffer 是固定容量的环形事件缓冲，并发安全。
type Buffer struct {
	mu    sync.Mutex
	items [Capacity]Event
	head  int // 下一个写入位置
	len   int
	seq   int64
}

// New 创建事件缓冲。
func New() *Buffer { return &Buffer{} }

// Add 追加一条事件。
func (b *Buffer) Add(uid, accountID, accountName, kind, detail string) Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	e := Event{
		ID:          b.seq,
		Time:        time.Now(),
		AccountID:   accountID,
		AccountName: accountName,
		Kind:        kind,
		Detail:      detail,
		uid:         uid,
	}
	b.items[b.head] = e
	b.head = (b.head + 1) % Capacity
	if b.len < Capacity {
		b.len++
	}
	return e
}

// List 按时间倒序返回指定 uid 的最近 limit 条事件。
// limit<=0 使用 DefaultLimit，超过 MaxLimit 钳制到 MaxLimit。
func (b *Buffer) List(uid string, limit int) []Event {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Event, 0, limit)
	// 从最新一条开始向前遍历
	for i := 0; i < b.len && len(out) < limit; i++ {
		idx := (b.head - 1 - i + Capacity) % Capacity
		e := b.items[idx]
		if e.uid == uid {
			out = append(out, e)
		}
	}
	return out
}
