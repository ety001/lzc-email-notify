import { useCallback, useEffect, useRef, useState } from 'react'
import { Routes, Route, Navigate, useNavigate } from 'react-router-dom'
import { MailWarning, RefreshCw, Plus, Inbox, AlertTriangle, Settings } from 'lucide-react'
import { Toaster } from 'sonner'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import AccountCard from '@/components/AccountCard'
import AccountFormDialog from '@/components/AccountFormDialog'
import EventList from '@/components/EventList'
import SettingsPage from '@/components/SettingsPage'

const POLL_INTERVAL = 10_000

export default function App() {
  const [accounts, setAccounts] = useState(null)
  const [events, setEvents] = useState(null)
  const [refreshing, setRefreshing] = useState(false)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(null) // null=新增；Account=编辑
  const [backendError, setBackendError] = useState('') // 后端断连时的错误信息
  const [backendVersion, setBackendVersion] = useState('') // 后端版本号（来自 /api/health）
  const inFlight = useRef(false)
  const navigate = useNavigate()

  // 获取后端版本（仅首次），用于核对前后端是否为同一发布包
  useEffect(() => {
    api.health()
      .then((h) => setBackendVersion(h && h.version ? h.version : '未知'))
      .catch(() => setBackendVersion(''))
  }, [])

  const refresh = useCallback(async ({ silent = false } = {}) => {
    if (inFlight.current) return
    inFlight.current = true
    if (!silent) setRefreshing(true)
    try {
      const [accs, evs] = await Promise.all([
        api.listAccounts(),
        api.listEvents(50).catch(() => []),
      ])
      setAccounts(Array.isArray(accs) ? accs : [])
      setEvents(Array.isArray(evs) ? evs : [])
      setBackendError('')
    } catch (err) {
      if (!silent) toast.error(err.message || '加载失败')
      setBackendError(err.message || '加载失败')
      // 首次加载失败也置为空数组，避免永久骨架屏
      setAccounts((prev) => (prev === null ? [] : prev))
      setEvents((prev) => (prev === null ? [] : prev))
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const timer = setInterval(() => refresh({ silent: true }), POLL_INTERVAL)
    return () => clearInterval(timer)
  }, [refresh])

  const openCreate = () => {
    setEditing(null)
    setFormOpen(true)
  }
  const openEdit = (account) => {
    setEditing(account)
    setFormOpen(true)
  }

  const loading = accounts === null

  return (
    <div className="min-h-screen bg-background">
      {/* 顶部标题栏 */}
      <header className="sticky top-0 z-40 border-b bg-background/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3 sm:px-6">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <MailWarning className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-semibold leading-tight">邮件提醒器</h1>
            <p className="truncate text-xs text-muted-foreground">
              定期检查邮箱，新邮件即时推送通知
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => navigate('/settings')}
            className="shrink-0"
            title="通知与设备设置"
          >
            <Settings />
            <span className="hidden sm:inline">设置</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refresh()}
            disabled={refreshing}
            className="shrink-0"
          >
            <RefreshCw className={refreshing ? 'animate-spin' : ''} />
            <span className="hidden sm:inline">刷新</span>
          </Button>
        </div>
      </header>

      {/* 后端版本标识：核对前后端是否同一发布包；为空表示版本接口不可达（老版本后端） */}
      <div className="mx-auto max-w-6xl px-4 pt-3 sm:px-6">
        <p className="text-right text-xs text-muted-foreground/70">
          {backendVersion ? `后端 v${backendVersion}` : '后端版本未知（可能为旧版）'}
        </p>
      </div>

      <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6">
        {/* 后端断连提示横幅：所有保存/测试操作依赖后端服务，异常时明确告知 */}
        {backendError && (
          <div className="mb-6 flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <div>
              <p className="font-medium">无法连接后端服务：{backendError}</p>
              <p className="mt-0.5 text-red-700/80">
                账号的保存、测试与巡检都依赖后端服务，请稍等片刻后点击右上角「刷新」重试；若持续出现请重启应用。
              </p>
            </div>
          </div>
        )}
        <Routes>
          <Route
            path="/"
            element={
              <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
                {/* 账号列表 */}
                <section className="min-w-0">
                  <div className="mb-4 flex items-center justify-between">
                    <h2 className="text-base font-semibold">
                      监控账号
                      {accounts && accounts.length > 0 && (
                        <span className="ml-2 text-sm font-normal text-muted-foreground">
                          {accounts.length} 个
                        </span>
                      )}
                    </h2>
                    {accounts && accounts.length > 0 && (
                      <Button size="sm" onClick={openCreate}>
                        <Plus /> 添加邮箱
                      </Button>
                    )}
                  </div>

                  {loading ? (
                    <div className="grid gap-4 sm:grid-cols-2">
                      <Skeleton className="h-44" />
                      <Skeleton className="h-44" />
                    </div>
                  ) : accounts.length === 0 ? (
                    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed py-20 text-center">
                      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-accent">
                        <Inbox className="h-7 w-7 text-accent-foreground" />
                      </div>
                      <p className="mb-1 font-medium">还没有监控的邮箱</p>
                      <p className="mb-6 text-sm text-muted-foreground">
                        添加一个邮箱账号，收到新邮件时立即通知你
                      </p>
                      <Button size="lg" onClick={openCreate}>
                        <Plus /> 添加邮箱
                      </Button>
                    </div>
                  ) : (
                    <div className="grid gap-4 sm:grid-cols-2">
                      {accounts.map((acc) => (
                        <AccountCard
                          key={acc.id}
                          account={acc}
                          onEdit={() => openEdit(acc)}
                          onChanged={() => refresh({ silent: true })}
                        />
                      ))}
                    </div>
                  )}
                </section>

                {/* 最近事件 */}
                <aside className="min-w-0">
                  <h2 className="mb-4 text-base font-semibold">最近事件</h2>
                  <EventList events={events} />
                </aside>
              </div>
            }
          />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>

      <Separator className="opacity-0" />

      <AccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        account={editing}
        onSaved={() => {
          setFormOpen(false)
          refresh({ silent: true })
        }}
      />

      <Toaster position="top-center" richColors closeButton />
    </div>
  )
}
