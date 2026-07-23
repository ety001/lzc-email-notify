package account

// Settings 是按 uid 存储的应用级设置。
type Settings struct {
	// DeviceFilterEnabled 为 true 时仅向 NotifyDevices 列出的设备发送通知；
	// false（默认）表示向全部在线设备广播。
	DeviceFilterEnabled bool `json:"device_filter_enabled,omitempty"`
	// NotifyDevices 选中的通知设备 ID（懒猫 unique_deivce_id）。
	NotifyDevices []string `json:"notify_devices,omitempty"`
}

// GetSettings 返回指定 uid 的设置；切片已复制，调用方可安全修改。
func (s *Store) GetSettings(uid string) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.settings[uid]
	if st == nil {
		return Settings{}
	}
	out := Settings{DeviceFilterEnabled: st.DeviceFilterEnabled}
	out.NotifyDevices = append([]string(nil), st.NotifyDevices...)
	return out
}

// SetNotifyDevices 更新指定 uid 的通知设备过滤设置并落盘。
func (s *Store) SetNotifyDevices(uid string, enabled bool, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings == nil {
		s.settings = make(map[string]*Settings)
	}
	s.settings[uid] = &Settings{
		DeviceFilterEnabled: enabled,
		NotifyDevices:       append([]string(nil), ids...),
	}
	return s.saveLocked()
}
