import type { ReactNode } from 'react'
import { AlertTriangle, Inbox, LoaderCircle, WifiOff } from 'lucide-react'
import { ApiError } from '@/api/client'
import { cn } from '@/lib/utils'

export function PageIntro({ eyebrow, title, description, actions }: {
  eyebrow: string
  title: string
  description: string
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-col gap-5 border-b border-mga-border/80 pb-6 sm:flex-row sm:items-end sm:justify-between">
      <div className="max-w-3xl">
        <p className="mb-2 text-[0.68rem] font-semibold uppercase tracking-[0.24em] text-mga-accent">{eyebrow}</p>
        <h1 className="text-2xl font-semibold tracking-tight text-mga-text sm:text-3xl">{title}</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-mga-muted">{description}</p>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  )
}

export function MetricCard({ label, value, detail, tone = 'neutral', icon }: {
  label: string
  value: ReactNode
  detail: string
  tone?: 'neutral' | 'good' | 'attention' | 'danger'
  icon?: ReactNode
}) {
  const toneClass = {
    neutral: 'border-mga-border/80',
    good: 'border-emerald-400/25',
    attention: 'border-amber-400/30',
    danger: 'border-rose-400/30',
  }[tone]
  return (
    <article className={cn('relative overflow-hidden rounded-xl border bg-mga-surface/80 p-5 shadow-sm', toneClass)}>
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-mga-accent/30 to-transparent" />
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.14em] text-mga-muted">{label}</p>
          <div className="mt-3 text-3xl font-semibold tracking-tight text-mga-text">{value}</div>
        </div>
        {icon && <div className="rounded-lg border border-mga-border bg-mga-elevated p-2 text-mga-accent">{icon}</div>}
      </div>
      <p className="mt-3 text-xs leading-5 text-mga-muted">{detail}</p>
    </article>
  )
}

export function SectionCard({ title, description, children, className }: {
  title: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('rounded-xl border border-mga-border/80 bg-mga-surface/65', className)}>
      <div className="border-b border-mga-border/70 px-5 py-4">
        <h2 className="text-sm font-semibold text-mga-text">{title}</h2>
        {description && <p className="mt-1 text-xs leading-5 text-mga-muted">{description}</p>}
      </div>
      <div className="p-5">{children}</div>
    </section>
  )
}

export function QueryFeedback({ pending, error, empty, emptyTitle, emptyDescription }: {
  pending: boolean
  error: unknown
  empty: boolean
  emptyTitle: string
  emptyDescription: string
}) {
  if (pending) {
    return (
      <div className="flex min-h-40 items-center justify-center gap-3 rounded-xl border border-dashed border-mga-border text-sm text-mga-muted" role="status">
        <LoaderCircle className="h-4 w-4 animate-spin" /> Loading management data…
      </div>
    )
  }
  if (error) {
    const unauthorized = error instanceof ApiError && (error.status === 401 || error.status === 403)
    const offline = typeof navigator !== 'undefined' && !navigator.onLine
    const Icon = offline ? WifiOff : AlertTriangle
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl border border-rose-400/25 bg-rose-500/5 px-6 text-center" role="alert">
        <Icon className="h-6 w-6 text-rose-300" />
        <p className="mt-3 text-sm font-semibold text-mga-text">{offline ? 'Server is offline' : unauthorized ? 'Profile is not authorized' : 'This data could not be loaded'}</p>
        <p className="mt-1 max-w-lg text-xs leading-5 text-mga-muted">{offline ? 'The last known data is preserved. Reconnect to refresh it.' : unauthorized ? 'Switch to an authorized profile or ask an administrator to update its policy.' : error instanceof Error ? error.message : 'The server returned an unexpected response.'}</p>
      </div>
    )
  }
  if (empty) {
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl border border-dashed border-mga-border px-6 text-center">
        <Inbox className="h-6 w-6 text-mga-muted" />
        <p className="mt-3 text-sm font-semibold text-mga-text">{emptyTitle}</p>
        <p className="mt-1 max-w-lg text-xs leading-5 text-mga-muted">{emptyDescription}</p>
      </div>
    )
  }
  return null
}

export function StatusPill({ label, tone = 'neutral' }: { label: string; tone?: 'neutral' | 'good' | 'attention' | 'danger' }) {
  const styles = {
    neutral: 'border-mga-border bg-mga-elevated text-mga-muted',
    good: 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300',
    attention: 'border-amber-400/25 bg-amber-400/10 text-amber-200',
    danger: 'border-rose-400/25 bg-rose-400/10 text-rose-200',
  }[tone]
  return <span className={cn('inline-flex items-center rounded-full border px-2.5 py-1 text-[0.68rem] font-semibold uppercase tracking-wider', styles)}>{label}</span>
}

export function formatCount(value: number | undefined): string {
  return new Intl.NumberFormat().format(value ?? 0)
}

export function formatDate(value?: string): string {
  if (!value) return 'Never'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Unknown' : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}
