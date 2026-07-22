import { useState } from 'react'
import {
  Mail,
  Pencil,
  Trash2,
  PlugZap,
  Radar,
  Loader2,
  CircleAlert,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { cn, relativeTime } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

function StatusDot({ account }) {
  const [showError, setShowError] = useState(false)
  const enabled = account.enabled
  const error = account.status?.last_error

  if (!enabled) {
    return (
      <span className="flex items-center gap-1.5 text-xs text-muted-foreground" title="未启用">
        <span className="h-2.5 w-2.5 rounded-full bg-stone-300" />
        未启用
      </span>
    )
  }
  if (error) {
    return (
      <button
        type="button"
        onClick={() => setShowError(true)}
        className="flex items-center gap-1.5 text-xs text-red-700"
        title={error}
      >
        <span className="h-2.5 w-2.5 rounded-full bg-red-600" />
        异常
        <Dialog open={showError} onOpenChange={setShowError}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CircleAlert className="h-5 w-5 text-red-700" />
                最近错误
              </DialogTitle>
              <DialogDescription>{account.name} 的最近一次巡检错误</DialogDescription>
            </DialogHeader>
            <div className="max-h-60 overflow-y-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 text-sm">
              {error}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowError(false)}>
                关闭
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </button>
    )
  }
  return (
    <span className="flex items-center gap-1.5 text-xs text-emerald-700" title="运行正常">
      <span className="h-2.5 w-2.5 rounded-full bg-emerald-600" />
      正常
    </span>
  )
}

export default function AccountCard({ account, onEdit, onChanged }) {
  const [busy, setBusy] = useState('') // 'toggle' | 'check' | 'test' | 'delete'
  const [confirmDelete, setConfirmDelete] = useState(false)

  const run = async (kind, fn) => {
    if (busy) return
    setBusy(kind)
    try {
      await fn()
    } catch (err) {
      toast.error(err.message || '操作失败')
    } finally {
      setBusy('')
    }
  }

  const handleToggle = (checked) =>
    run('toggle', async () => {
      await api.updateAccount(account.id, {
        name: account.name,
        protocol: account.protocol,
        host: account.host,
        port: account.port,
        ssl: account.ssl,
        username: account.username,
        password: '',
        interval_sec: account.interval_sec,
        web_url: account.web_url || '',
        enabled: checked,
      })
      toast.success(checked ? `已启用「${account.name}」` : `已禁用「${account.name}」`)
      onChanged()
    })

  const handleCheck = () =>
    run('check', async () => {
      await api.checkAccount(account.id)
      toast.success(`已触发「${account.name}」巡检，稍候自动刷新结果`)
      setTimeout(onChanged, 1500)
    })

  const handleTest = () =>
    run('test', async () => {
      const res = await api.testAccount(account.id)
      if (res?.ok) {
        toast.success(`「${account.name}」连接成功`)
      } else {
        toast.error(res?.error || '连接失败')
      }
    })

  const handleDelete = () =>
    run('delete', async () => {
      await api.deleteAccount(account.id)
      setConfirmDelete(false)
      toast.success(`已删除「${account.name}」`)
      onChanged()
    })

  const lastMail = account.status?.last_mail
  const intervalMin = Math.max(1, Math.round((account.interval_sec || 60) / 60))

  return (
    <Card className={cn('flex flex-col', !account.enabled && 'opacity-80')}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <CardTitle className="truncate text-base">{account.name}</CardTitle>
            <Badge variant="secondary" className="shrink-0 uppercase">
              {account.protocol}
            </Badge>
          </div>
          <StatusDot account={account} />
        </div>
        <p className="truncate text-xs text-muted-foreground">
          {account.host}:{account.port}
          {account.ssl ? ' · SSL' : ''} · 每 {intervalMin} 分钟检查
        </p>
      </CardHeader>

      <CardContent className="flex-1 space-y-2 pb-3 text-sm">
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>最近检查</span>
          <span className="flex items-center gap-1">
            {account.status?.checking && (
              <Loader2 className="h-3 w-3 animate-spin" aria-label="检查中" />
            )}
            {relativeTime(account.status?.last_check_at)}
          </span>
        </div>
        {lastMail ? (
          <div className="rounded-md bg-muted px-3 py-2">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Mail className="h-3 w-3 shrink-0" />
              最近邮件
            </div>
            <p className="mt-0.5 truncate text-xs" title={lastMail.from}>
              {lastMail.from}
            </p>
            <p className="truncate text-xs font-medium" title={lastMail.subject}>
              {lastMail.subject}
            </p>
          </div>
        ) : (
          <div className="rounded-md bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
            暂无邮件记录
          </div>
        )}
      </CardContent>

      <CardFooter className="flex-wrap items-center gap-2 border-t pt-3">
        <div className="flex items-center gap-2">
          <Switch
            checked={!!account.enabled}
            disabled={!!busy}
            onCheckedChange={handleToggle}
            aria-label="启用或禁用"
          />
          <span className="text-xs text-muted-foreground">
            {account.enabled ? '已启用' : '已禁用'}
          </span>
        </div>
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleCheck}
            disabled={!!busy || !account.enabled}
            title="立即检查"
          >
            {busy === 'check' ? <Loader2 className="animate-spin" /> : <Radar />}
            <span className="hidden xl:inline">检查</span>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleTest}
            disabled={!!busy}
            title="测试连接"
          >
            {busy === 'test' ? <Loader2 className="animate-spin" /> : <PlugZap />}
            <span className="hidden xl:inline">测试</span>
          </Button>
          <Button variant="ghost" size="sm" onClick={onEdit} disabled={!!busy} title="编辑">
            <Pencil />
            <span className="hidden xl:inline">编辑</span>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => setConfirmDelete(true)}
            disabled={!!busy}
            title="删除"
          >
            <Trash2 />
          </Button>
        </div>
      </CardFooter>

      {/* 删除二次确认 */}
      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>删除账号</DialogTitle>
            <DialogDescription>
              确定要删除「{account.name}」（{account.username}）吗？删除后将停止监控该邮箱，此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setConfirmDelete(false)} disabled={!!busy}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={!!busy}>
              {busy === 'delete' && <Loader2 className="animate-spin" />}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
