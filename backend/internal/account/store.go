package account

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrNotFound 表示账号不存在（或不属于当前用户）。
var ErrNotFound = errors.New("账号不存在")

type fileFormat struct {
	Accounts []*Account           `json:"accounts"`
	Settings map[string]*Settings `json:"settings,omitempty"` // uid -> 设置
}

// Store 是基于单个 JSON 文件的账号存储，原子写、0600 权限，
// 所有读写均持锁，返回给调用方的都是深拷贝。
type Store struct {
	mu       sync.Mutex
	path     string
	accounts []*Account
	settings map[string]*Settings
}

// Open 加载 dir 下的 config.json；目录不存在则创建，文件不存在视为空。
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "config.json")}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	s.accounts = f.Accounts
	s.settings = f.Settings
	return s, nil
}

// Flush 将当前内容落盘（常规写操作已即时落盘，用于退出前兜底）。
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(fileFormat{Accounts: s.accounts, Settings: s.settings}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入临时配置文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	// rename 后确保权限（部分文件系统 rename 保留目标权限）
	_ = os.Chmod(s.path, 0o600)
	return nil
}

// NewID 生成 16 位十六进制随机 ID。
func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ListAll 返回全部账号的深拷贝（内部使用，含所有 uid）。
func (s *Store) ListAll() []*Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, a.Clone())
	}
	return out
}

// List 返回指定 uid 的全部账号（深拷贝）。
func (s *Store) List(uid string) []*Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Account, 0)
	for _, a := range s.accounts {
		if a.UID == uid {
			out = append(out, a.Clone())
		}
	}
	return out
}

// Get 按 uid+id 获取账号深拷贝。
func (s *Store) Get(uid, id string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.accounts {
		if a.ID == id && a.UID == uid {
			return a.Clone(), nil
		}
	}
	return nil, ErrNotFound
}

// GetByID 不按 uid 过滤获取账号深拷贝（仅供后端内部轮询使用）。
func (s *Store) GetByID(id string) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.accounts {
		if a.ID == id {
			return a.Clone(), nil
		}
	}
	return nil, ErrNotFound
}

// Create 新建账号并落盘，返回带 ID/时间戳的深拷贝。
func (s *Store) Create(a *Account) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	a.ID = NewID()
	a.CreatedAt = now
	a.UpdatedAt = now
	s.accounts = append(s.accounts, a.Clone())
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return a.Clone(), nil
}

// Update 对 uid+id 命中的账号执行 mutator（锁内），成功后落盘。
// mutator 返回 false 表示放弃本次修改（不落盘）。
func (s *Store) Update(uid, id string, mutate func(a *Account) bool) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.accounts {
		if a.ID == id && a.UID == uid {
			if !mutate(a) {
				return a.Clone(), nil
			}
			a.UpdatedAt = time.Now()
			if err := s.saveLocked(); err != nil {
				return nil, err
			}
			return a.Clone(), nil
		}
	}
	return nil, ErrNotFound
}

// UpdateByID 同 Update，但不按 uid 过滤（仅供后端内部轮询使用）。
func (s *Store) UpdateByID(id string, mutate func(a *Account) bool) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.accounts {
		if a.ID == id {
			if !mutate(a) {
				return a.Clone(), nil
			}
			a.UpdatedAt = time.Now()
			if err := s.saveLocked(); err != nil {
				return nil, err
			}
			return a.Clone(), nil
		}
	}
	return nil, ErrNotFound
}

// Delete 删除 uid+id 命中的账号并落盘。
func (s *Store) Delete(uid, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.accounts {
		if a.ID == id && a.UID == uid {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}
