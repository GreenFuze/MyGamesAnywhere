import { cn } from '@/lib/utils'

type Status = 'ok' | 'error' | 'unavailable' | 'pending'

interface StatusDotProps {
  status: Status
  className?: string
  label?: string
}

const colors: Record<Status, string> = {
  ok: 'bg-green-500',
  error: 'bg-red-500',
  unavailable: 'bg-yellow-500',
  pending: 'bg-mga-muted animate-pulse',
}

/** Small colored dot indicating status. */
export function StatusDot({ status, className, label }: StatusDotProps) {
  return (
    <span className={cn('inline-flex items-center gap-1.5', className)}>
      <span
        className={cn('inline-block h-2 w-2 rounded-full', colors[status])}
        aria-hidden="true"
      />
      {label && <span className="text-xs text-mga-muted">{label}</span>}
    </span>
  )
}
