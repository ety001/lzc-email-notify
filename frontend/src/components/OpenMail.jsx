import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'

import { api } from '@/lib/api'
import { clearAccountUnread } from '@/lib/unread'

/**
 * 系统通知 deeplink 的落地路由（/#/open/:accountId）：
 * 找到对应账号 → 新标签页打开其网页版邮箱 → 清除该账号未读提醒 → 回到首页。
 */
export default function OpenMail() {
  const { accountId } = useParams()
  const navigate = useNavigate()
  const [message, setMessage] = useState('正在打开网页版邮箱…')

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const accounts = await api.listAccounts()
        const account = (Array.isArray(accounts) ? accounts : []).find((a) => a.id === accountId)
        // 无论是否找到账号都清除未读（账号可能已被删除）
        clearAccountUnread(accountId)
        if (account?.web_url) {
          window.open(account.web_url, '_blank', 'noopener')
          if (!cancelled) setMessage(`已在新标签页打开「${account.name}」的网页版邮箱，正在返回…`)
        } else if (!cancelled) {
          setMessage(account ? '该邮箱未配置网页版地址，正在返回…' : '邮箱账号不存在，正在返回…')
        }
      } catch {
        if (!cancelled) setMessage('打开失败，正在返回首页…')
      }
      setTimeout(() => {
        if (!cancelled) navigate('/', { replace: true })
      }, 1500)
    })()
    return () => {
      cancelled = true
    }
  }, [accountId, navigate])

  return (
    <div className="flex items-center justify-center gap-2 py-24 text-sm text-muted-foreground">
      <Loader2 className="h-4 w-4 animate-spin" />
      {message}
    </div>
  )
}
