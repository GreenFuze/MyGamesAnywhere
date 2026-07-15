import { cn } from '@/lib/utils'

export type StatusTone = 'success' | 'warning' | 'danger' | 'muted' | 'accent' | 'purple'

const toneClasses: Record<StatusTone, string> = {
  success: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300',
  warning: 'border-amber-500/30 bg-amber-500/10 text-amber-200',
  danger: 'border-red-500/30 bg-red-500/10 text-red-300',
  muted: 'border-mga-border bg-mga-elevated text-mga-muted',
  accent: 'border-mga-accent/30 bg-mga-accent/10 text-mga-accent',
  purple: 'border-purple-500/35 bg-purple-500/10 text-purple-300',
}

interface StatusPillProps {
  label: string
  tone?: StatusTone
  detail?: string
  className?: string
}

export function StatusPill({ label, tone = 'muted', detail, className }: StatusPillProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs font-medium leading-none',
        toneClasses[tone],
        className,
      )}
      title={detail}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
      {label}
    </span>
  )
}
