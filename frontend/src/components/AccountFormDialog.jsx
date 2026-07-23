import { useEffect, useState } from 'react'
import { CheckCircle2, Loader2, PlugZap, XCircle } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { MAIL_PRESETS, findPreset } from '@/lib/presets'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const EMPTY_FORM = {
  preset: 'custom',
  name: '',
  protocol: 'imap',
  host: '',
  port: '993',
  ssl: true,
  username: '',
  password: '',
  interval_min: '1',
  web_url: '',
  enabled: true,
}

function accountToForm(account) {
  return {
    preset: 'custom',
    name: account.name || '',
    protocol: account.protocol || 'imap',
    host: account.host || '',
    port: String(account.port ?? 993),
    ssl: !!account.ssl,
    username: account.username || '',
    password: '',
    interval_min: String(Math.max(1, Math.round((account.interval_sec || 60) / 60))),
    web_url: account.web_url || '',
    enabled: !!account.enabled,
  }
}

export default function AccountFormDialog({ open, onOpenChange, account, onSaved }) {
  const isEdit = !!account
  const [form, setForm] = useState(EMPTY_FORM)
  const [errors, setErrors] = useState({})
  const [submitting, setSubmitting] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null) // {ok:true} | {ok:false,error:string} | null

  useEffect(() => {
    if (open) {
      setForm(account ? accountToForm(account) : EMPTY_FORM)
      setErrors({})
      setSubmitting(false)
      setTesting(false)
      setTestResult(null)
    }
  }, [open, account])

  const set = (key, value) => {
    setForm((f) => ({ ...f, [key]: value }))
    setErrors((e) => ({ ...e, [key]: undefined }))
    // 连接参数变了，上次的测试结果即失效
    if (['protocol', 'host', 'port', 'ssl', 'username', 'password'].includes(key)) {
      setTestResult(null)
    }
  }

  const applyPreset = (presetKey, protocol, base = null) => {
    const preset = findPreset(presetKey)
    const conf = preset[protocol]
    setForm((f) => {
      const next = { ...(base || f), preset: presetKey, protocol }
      if (conf) {
        next.host = conf.host
        next.port = String(conf.port)
        next.ssl = conf.ssl
        next.web_url = conf.web_url
      }
      return next
    })
    setErrors((e) => ({ ...e, host: undefined, port: undefined }))
    setTestResult(null)
  }

  const handlePresetChange = (presetKey) => {
    if (presetKey === 'custom') {
      set('preset', 'custom')
      return
    }
    applyPreset(presetKey, form.protocol)
  }

  const handleProtocolChange = (protocol) => {
    if (form.preset !== 'custom') {
      // 保留当前预设，切换协议时同步切换 host/port
      applyPreset(form.preset, protocol)
    } else {
      setForm((f) => ({
        ...f,
        protocol,
        // 自定义模式下按协议给个常见默认端口（仅在端口为空或是另一协议的默认值时）
        port:
          f.port === '' || f.port === '993' || f.port === '995'
            ? protocol === 'imap'
              ? '993'
              : '995'
            : f.port,
      }))
    }
  }

  const validate = () => {
    const errs = {}
    if (!form.name.trim()) errs.name = '请输入名称'
    if (!form.host.trim()) errs.host = '请输入服务器地址'
    const port = Number(form.port)
    if (!form.port || !Number.isInteger(port) || port < 1 || port > 65535) {
      errs.port = '端口需为 1-65535 的整数'
    }
    if (!form.username.trim()) errs.username = '请输入用户名'
    if (!isEdit && !form.password) errs.password = '请输入密码（或授权码）'
    const interval = Number(form.interval_min)
    if (!form.interval_min || !Number.isFinite(interval) || interval < 1) {
      errs.interval_min = '检查间隔最小为 1 分钟'
    }
    if (form.web_url.trim() && !/^https?:\/\/.+/.test(form.web_url.trim())) {
      errs.web_url = '请输入以 http:// 或 https:// 开头的地址'
    }
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  // 测试连接：独立异步进行，不阻塞保存；编辑模式且未填新密码时测已保存的配置
  const handleTest = async () => {
    if (testing) return
    const useSaved = isEdit && !form.password
    if (!useSaved && !form.password) {
      setTestResult({ ok: false, error: '请先填写密码/授权码' })
      return
    }
    if (!form.host.trim() || !form.username.trim()) {
      setTestResult({ ok: false, error: '请先填写服务器与用户名' })
      return
    }
    setTesting(true)
    setTestResult(null)
    try {
      let res
      if (useSaved) {
        res = await api.testAccount(account.id)
      } else {
        res = await api.testConnection({
          protocol: form.protocol,
          host: form.host.trim(),
          port: Number(form.port),
          ssl: !!form.ssl,
          username: form.username.trim(),
          password: form.password,
        })
      }
      setTestResult(res && res.ok ? { ok: true } : { ok: false, error: (res && res.error) || '连接失败' })
    } catch (err) {
      setTestResult({ ok: false, error: err.message || '连接测试失败' })
    } finally {
      setTesting(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    if (!validate()) return
    setSubmitting(true)
    const payload = {
      name: form.name.trim(),
      protocol: form.protocol,
      host: form.host.trim(),
      port: Number(form.port),
      ssl: !!form.ssl,
      username: form.username.trim(),
      password: form.password, // 编辑时为空字符串表示不修改
      interval_sec: Math.max(1, Math.round(Number(form.interval_min))) * 60,
      web_url: form.web_url.trim(),
      enabled: !!form.enabled,
    }
    try {
      if (isEdit) {
        await api.updateAccount(account.id, payload)
        toast.success(`已保存「${payload.name}」`)
      } else {
        await api.createAccount(payload)
        toast.success(`已添加「${payload.name}」`)
      }
      onSaved()
    } catch (err) {
      toast.error(err.message || '保存失败')
    } finally {
      setSubmitting(false)
    }
  }

  const fieldError = (key) =>
    errors[key] ? <p className="text-xs text-destructive">{errors[key]}</p> : null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? '编辑邮箱' : '添加邮箱'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? '修改邮箱监控配置，密码留空则不修改'
              : '配置一个需要监控的邮箱账号，首次巡检只建立基线，不会轰炸历史邮件'}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="f-name">名称 *</Label>
              <Input
                id="f-name"
                value={form.name}
                onChange={(e) => set('name', e.target.value)}
                placeholder="如：我的 QQ 邮箱"
              />
              {fieldError('name')}
            </div>
            <div className="grid gap-1.5">
              <Label>常用邮箱预设</Label>
              <Select value={form.preset} onValueChange={handlePresetChange}>
                <SelectTrigger>
                  <SelectValue placeholder="选择预设" />
                </SelectTrigger>
                <SelectContent>
                  {MAIL_PRESETS.map((p) => (
                    <SelectItem key={p.key} value={p.key}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label>协议 *</Label>
              <Select value={form.protocol} onValueChange={handleProtocolChange}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="imap">IMAP</SelectItem>
                  <SelectItem value="pop3">POP3</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-[1fr_110px] gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="f-host">服务器 *</Label>
                <Input
                  id="f-host"
                  value={form.host}
                  onChange={(e) => set('host', e.target.value)}
                  placeholder="imap.qq.com"
                />
                {fieldError('host')}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="f-port">端口 *</Label>
                <Input
                  id="f-port"
                  type="number"
                  min={1}
                  max={65535}
                  value={form.port}
                  onChange={(e) => set('port', e.target.value)}
                />
                {fieldError('port')}
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="f-username">用户名 *</Label>
              <Input
                id="f-username"
                value={form.username}
                onChange={(e) => set('username', e.target.value)}
                placeholder="someone@qq.com"
                autoComplete="off"
              />
              {fieldError('username')}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="f-password">{isEdit ? '密码' : '密码 / 授权码 *'}</Label>
              <Input
                id="f-password"
                type="password"
                value={form.password}
                onChange={(e) => set('password', e.target.value)}
                placeholder={isEdit ? '已保存，留空则不修改' : 'QQ/163 请使用授权码'}
                autoComplete="new-password"
              />
              {fieldError('password')}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="f-interval">检查间隔（分钟）*</Label>
              <Input
                id="f-interval"
                type="number"
                min={1}
                step={1}
                value={form.interval_min}
                onChange={(e) => set('interval_min', e.target.value)}
              />
              {fieldError('interval_min')}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="f-weburl">网页版跳转地址</Label>
              <Input
                id="f-weburl"
                value={form.web_url}
                onChange={(e) => set('web_url', e.target.value)}
                placeholder="https://mail.qq.com"
              />
              {fieldError('web_url')}
            </div>
          </div>

          <div className="flex items-center justify-between rounded-md bg-muted px-3 py-2.5">
            <div className="flex items-center gap-2">
              <Switch
                id="f-ssl"
                checked={form.ssl}
                onCheckedChange={(v) => set('ssl', v)}
              />
              <Label htmlFor="f-ssl" className="cursor-pointer font-normal">
                使用 SSL/TLS 加密连接
              </Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="f-enabled"
                checked={form.enabled}
                onCheckedChange={(v) => set('enabled', v)}
              />
              <Label htmlFor="f-enabled" className="cursor-pointer font-normal">
                启用监控
              </Label>
            </div>
          </div>

          {/* 测试连接结果（内联展示，与保存互不影响） */}
          {testResult && (
            <div
              className={`flex items-start gap-2 rounded-md px-3 py-2 text-sm ${
                testResult.ok
                  ? 'bg-emerald-50 text-emerald-700'
                  : 'bg-red-50 text-red-700'
              }`}
            >
              {testResult.ok ? (
                <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
              ) : (
                <XCircle className="mt-0.5 h-4 w-4 shrink-0" />
              )}
              <span className="min-w-0 break-all">
                {testResult.ok
                  ? `连接成功${isEdit && !form.password ? '（使用已保存的密码）' : ''}，服务器与账号验证通过`
                  : testResult.error}
              </span>
            </div>
          )}

          <DialogFooter className="gap-2 pt-2 sm:justify-between">
            <Button
              type="button"
              variant="outline"
              onClick={handleTest}
              disabled={testing}
              title="使用当前填写的信息测试连接，不影响保存"
            >
              {testing ? <Loader2 className="animate-spin" /> : <PlugZap />}
              {testing ? '测试中…' : '测试连接'}
            </Button>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={submitting}
              >
                取消
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="animate-spin" />}
                {isEdit ? '保存修改' : '添加邮箱'}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
