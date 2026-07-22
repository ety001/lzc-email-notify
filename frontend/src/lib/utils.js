import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs) {
  return twMerge(clsx(inputs))
}

/** 把 ISO 时间格式化为中文相对时间，如「3 分钟前」；空值返回 fallback */
export function relativeTime(iso, fallback = '尚未检查') {
  if (!iso) return fallback
  const t = new Date(iso)
  if (Number.isNaN(t.getTime())) return fallback
  const diff = Date.now() - t.getTime()
  if (diff < 0) return '刚刚'
  const sec = Math.floor(diff / 1000)
  if (sec < 10) return '刚刚'
  if (sec < 60) return `${sec} 秒前`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hour = Math.floor(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.floor(hour / 24)
  if (day < 30) return `${day} 天前`
  return t.toLocaleDateString('zh-CN')
}
