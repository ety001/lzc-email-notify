import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  ArrowLeft,
  Bell,
  Loader2,
  Monitor,
  Smartphone,
  Tv,
  User,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

function DeviceIcon({ device }) {
  const cls = 'h-4 w-4 shrink-0 text-muted-foreground'
  if (device.is_tv) return <Tv className={cls} />
  if (device.is_mobile) return <Smartphone className={cls} />
  return <Monitor className={cls} />
}

function deviceLabel(device) {
  return device.remark_name || device.name || device.model || device.id.slice(0, 8)
}

export default function SettingsPage() {
  const navigate = useNavigate()
  const [data, setData] = useState(null) // GET /api/settings 的响应
  const [selected, setSelected] = useState(null) // Set<deviceId>，开关状态
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('') // 'save' | 'test'

  useEffect(() => {
    api
      .getSettings()
      .then((res) => {
        setData(res)
        const all = new Set((res.devices || []).map((d) => d.id))
        if (res.device_filter_enabled) {
          // 已启用过滤：以保存的选择为准（不在列表里的旧设备 ID 丢弃）
          const saved = new Set(res.selected_notify_devices || [])
          setSelected(new Set([...all].filter((id) => saved.has(id))))
        } else {
          // 未启用过滤：等于全部选中
          setSelected(all)
        }
      })
      .catch((err) => setError(err.message || '加载设置失败'))
  }, [])

  const devices = useMemo(() => data?.devices || [], [data])
  const dirty = useMemo(() => {
    if (!data || !selected) return false
    const allOn = selected.size === devices.length
    const savedEnabled = !!data.device_filter_enabled
    const savedSet = new Set(data.selected_notify_devices || [])
    if (allOn && !savedEnabled) return false
    if (!allOn) {
      if (!savedEnabled) return true
      if (savedSet.size !== selected.size) return true
      for (const id of selected) if (!savedSet.has(id)) return true
      return false
    }
    return savedEnabled
  }, [data, selected, devices])

  const toggle = (id, checked) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  // 全部开关打开 = 不过滤（enabled:false）；否则按选中集合定向发送
  const buildPayload = () => {
    const allOn = selected.size === devices.length
    if (allOn) return { enabled: false, devices: [] }
    return { enabled: true, devices: [...selected] }
  }

  const handleSave = async () => {
    if (busy) return
    setBusy('save')
    try {
      await api.saveNotifyDevices(buildPayload())
      const res = await api.getSettings()
      setData(res)
      toast.success('通知设备设置已保存')
    } catch (err) {
      toast.error(err.message || '保存失败')
    } finally {
      setBusy('')
    }
  }

  // 先保存当前选择再发送测试通知，保证测试的就是界面看到的选择
  const handleTest = async () => {
    if (busy) return
    setBusy('test')
    try {
      await api.saveNotifyDevices(buildPayload())
      const res = await api.getSettings()
      setData(res)
      await api.testNotify()
      toast.success('测试通知已发送，请留意选中设备上的系统通知')
    } catch (err) {
      toast.error(err.message || '测试通知发送失败')
    } finally {
      setBusy('')
    }
  }

  const user = data?.user

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-40 border-b bg-background/90 backdrop-blur">
        <div className="mx-auto flex max-w-2xl items-center gap-3 px-4 py-3 sm:px-6">
          <Button variant="ghost" size="sm" onClick={() => navigate('/')}>
            <ArrowLeft />
            返回
          </Button>
          <h1 className="flex-1 text-lg font-semibold">设置</h1>
        </div>
      </header>

      <main className="mx-auto max-w-2xl space-y-6 px-4 py-6 sm:px-6">
        {error ? (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
            {error}
          </div>
        ) : !data ? (
          <div className="space-y-4">
            <Skeleton className="h-28" />
            <Skeleton className="h-56" />
          </div>
        ) : (
          <>
            {/* 当前懒猫账号 */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-base">当前账号</CardTitle>
                <CardDescription>通过懒猫微服 OIDC 登录的账号信息</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex items-center gap-3">
                  {user?.avatar ? (
                    <img
                      src={user.avatar}
                      alt="头像"
                      className="h-11 w-11 rounded-full object-cover"
                    />
                  ) : (
                    <div className="flex h-11 w-11 items-center justify-center rounded-full bg-accent">
                      <User className="h-5 w-5 text-accent-foreground" />
                    </div>
                  )}
                  <div className="min-w-0">
                    <p className="truncate font-medium">{user?.nickname || user?.uid}</p>
                    <p className="truncate text-xs text-muted-foreground">UID：{user?.uid}</p>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* 通知设备选择 */}
            <Card>
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0 pb-3">
                <div>
                  <CardTitle className="text-base">通知设备</CardTitle>
                  <CardDescription>
                    选择接收新邮件通知的客户端设备；全部打开表示向所有在线设备广播
                  </CardDescription>
                </div>
                {devices.length > 0 && selected && (
                  // 与下方列表的 Switch 垂直对齐（列表行 px-2 → pr-2 补偿）
                  <div className="shrink-0 pr-2 pt-1">
                    <Switch
                      checked={selected.size === devices.length}
                      onCheckedChange={(checked) => {
                        if (checked) setSelected(new Set(devices.map((d) => d.id)))
                        else setSelected(new Set())
                      }}
                      disabled={!!busy}
                      aria-label="全选或全不选"
                      title="全选 / 全不选"
                    />
                  </div>
                )}
              </CardHeader>
              <CardContent className="space-y-1">
                {devices.length === 0 ? (
                  <p className="py-4 text-center text-sm text-muted-foreground">
                    当前账号下没有已绑定的客户端设备
                  </p>
                ) : (
                  devices.map((d) => (
                    <div
                      key={d.id}
                      className="flex items-center gap-3 rounded-md px-2 py-2.5 hover:bg-muted/60"
                    >
                      <DeviceIcon device={d} />
                      <div className="min-w-0 flex-1">
                        <p className="flex items-center gap-2 truncate text-sm font-medium">
                          {deviceLabel(d)}
                          <Badge
                            variant={d.online ? 'default' : 'secondary'}
                            className="shrink-0 text-[10px]"
                          >
                            {d.online ? '在线' : '离线'}
                          </Badge>
                        </p>
                        {d.model && (
                          <p className="truncate text-xs text-muted-foreground">{d.model}</p>
                        )}
                      </div>
                      <Switch
                        checked={selected?.has(d.id) ?? false}
                        onCheckedChange={(checked) => toggle(d.id, checked)}
                        disabled={!!busy}
                        aria-label={`接收通知：${deviceLabel(d)}`}
                      />
                    </div>
                  ))
                )}
                <div className="flex flex-wrap items-center gap-2 pt-4">
                  <Button onClick={handleSave} disabled={!!busy || !dirty}>
                    {busy === 'save' && <Loader2 className="animate-spin" />}
                    保存设置
                  </Button>
                  <Button variant="outline" onClick={handleTest} disabled={!!busy}>
                    {busy === 'test' ? <Loader2 className="animate-spin" /> : <Bell />}
                    测试客户端通知
                  </Button>
                  <p className="w-full text-xs text-muted-foreground">
                    「测试客户端通知」会先保存当前选择，再向选中的在线设备发送一条测试通知
                  </p>
                </div>
              </CardContent>
            </Card>
          </>
        )}
      </main>
    </div>
  )
}
