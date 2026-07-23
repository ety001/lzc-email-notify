import { Mail, BellOff, AlertTriangle, Info, ScrollText, ExternalLink } from 'lucide-react'
import { useMemo } from 'react'

import { cn, relativeTime } from '@/lib/utils'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

const KIND_META = {
  new_mail: {
    icon: Mail,
    label: '新邮件',
    dot: 'bg-emerald-600',
    text: 'text-emerald-700',
    bg: 'bg-emerald-50',
  },
  notify_failed: {
    icon: BellOff,
    label: '通知失败',
    dot: 'bg-amber-600',
    text: 'text-amber-700',
    bg: 'bg-amber-50',
  },
  check_failed: {
    icon: AlertTriangle,
    label: '巡检失败',
    dot: 'bg-red-600',
    text: 'text-red-700',
    bg: 'bg-red-50',
  },
  info: {
    icon: Info,
    label: '信息',
    dot: 'bg-stone-400',
    text: 'text-stone-600',
    bg: 'bg-stone-100',
  },
}

function metaOf(kind) {
  return KIND_META[kind] || KIND_META.info
}

export default function EventList({ events, accounts }) {
  // account_id -> 网页版邮箱地址，用于新邮件事件点击跳转
  const webUrlByAccount = useMemo(() => {
    const m = new Map()
    for (const a of accounts || []) {
      if (a.web_url) m.set(a.id, a.web_url)
    }
    return m
  }, [accounts])

  if (events === null) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
        <Skeleton className="h-14" />
      </div>
    )
  }

  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
        <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-muted">
          <ScrollText className="h-5 w-5 text-muted-foreground" />
        </div>
        <p className="text-sm text-muted-foreground">暂无事件</p>
      </div>
    )
  }

  return (
    <Card>
      <CardContent className="max-h-[70vh] divide-y overflow-y-auto p-0">
        {events.map((ev) => {
          const meta = metaOf(ev.kind)
          const Icon = meta.icon
          // 邮件提醒类事件（新邮件/通知失败）配置了网页版地址时可点击跳转
          const jumpUrl =
            (ev.kind === 'new_mail' || ev.kind === 'notify_failed') &&
            webUrlByAccount.get(ev.account_id)
          const Tag = jumpUrl ? 'a' : 'div'
          return (
            <Tag
              key={ev.id}
              {...(jumpUrl
                ? {
                    href: jumpUrl,
                    target: '_blank',
                    rel: 'noopener noreferrer',
                    title: '点击打开网页版邮箱',
                  }
                : {})}
              className={cn(
                'flex items-start gap-3 px-4 py-3',
                jumpUrl && 'cursor-pointer transition-colors hover:bg-muted/60'
              )}
            >
              <div
                className={cn(
                  'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full',
                  meta.bg
                )}
              >
                <Icon className={cn('h-3.5 w-3.5', meta.text)} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-baseline justify-between gap-2">
                  <span className={cn('truncate text-xs font-medium', meta.text)}>
                    {meta.label}
                    {ev.account_name && (
                      <span className="ml-1.5 font-normal text-foreground/70">
                        {ev.account_name}
                      </span>
                    )}
                  </span>
                  <span
                    className="shrink-0 text-xs text-muted-foreground"
                    title={ev.time}
                  >
                    {relativeTime(ev.time, '')}
                  </span>
                </div>
                <p className="mt-0.5 break-all text-xs leading-relaxed text-muted-foreground">
                  {ev.detail}
                </p>
              </div>
              {jumpUrl && (
                <ExternalLink className="mt-1 h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
              )}
            </Tag>
          )
        })}
      </CardContent>
    </Card>
  )
}
