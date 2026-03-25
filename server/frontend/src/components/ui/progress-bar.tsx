import { cn } from '@/lib/utils'

interface ProgressBarProps {
  /** Progress value 0–100. If undefined, shows indeterminate animation. */
  value?: number
  className?: string
  label?: string
}

/** Horizontal progress bar using mga accent color. */
export function ProgressBar({ value, className, label }: ProgressBarProps) {
  const indeterminate = value === undefined

  return (
    <div className={cn('w-full', className)}>
      {label && (
        <div className="flex justify-between mb-1">
          <span className="text-xs text-mga-muted">{label}</span>
          {!indeterminate && (
            <span className="text-xs text-mga-muted">{Math.round(value)}%</span>
          )}
        </div>
      )}
      <div className="h-2 rounded-full bg-mga-elevated overflow-hidden">
        {indeterminate ? (
          <div className="h-full w-1/3 rounded-full bg-mga-accent animate-indeterminate" />
        ) : (
          <div
            className="h-full rounded-full bg-mga-accent transition-all duration-300"
            style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
          />
        )}
      </div>
    </div>
  )
}
