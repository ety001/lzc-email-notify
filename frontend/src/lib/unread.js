/**
 * 未读新邮件状态管理（localStorage 持久化，跨标签页同步）。
 *
 * 语义：每个账号记录一个"已清除到"的事件 ID（cleared map），
 * id 大于该值的 new_mail 事件即为未读。清除动作来自：
 *   - 点击懒猫系统通知（deeplink → /open/:accountId 路由）
 *   - 点击卡片内的新邮件提醒条 / 最近邮件区块
 */

const KEY = 'email-notify:unread-cleared-v1'
const INIT_KEY = 'email-notify:unread-init-v1'

export function getClearedMap() {
  try {
    return JSON.parse(localStorage.getItem(KEY)) || {}
  } catch {
    return {}
  }
}

function saveClearedMap(map) {
  localStorage.setItem(KEY, JSON.stringify(map))
  // 同一标签页内通知状态变更（storage 事件只跨标签页触发）
  window.dispatchEvent(new Event('unread-changed'))
}

/** 清除指定账号的未读（默认清除到当前已知的最大事件 ID） */
export function clearAccountUnread(accountId, eventId = Number.MAX_SAFE_INTEGER) {
  const map = getClearedMap()
  map[accountId] = Math.max(map[accountId] || 0, eventId)
  saveClearedMap(map)
}

export function isUnreadInitialized() {
  return localStorage.getItem(INIT_KEY) === '1'
}

/** 首次打开应用：把已有事件全部标记为已读，避免历史事件造成未读轰炸 */
export function initUnreadBaseline(events) {
  const map = getClearedMap()
  for (const ev of events || []) {
    if (ev.kind === 'new_mail' && ev.account_id) {
      map[ev.account_id] = Math.max(map[ev.account_id] || 0, ev.id)
    }
  }
  localStorage.setItem(KEY, JSON.stringify(map))
  localStorage.setItem(INIT_KEY, '1')
}
