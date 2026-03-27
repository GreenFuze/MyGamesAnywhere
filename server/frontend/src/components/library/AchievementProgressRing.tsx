import type { AchievementSummaryDTO } from '@/api/client'
import { cn } from '@/lib/utils'

interface AchievementProgressRingProps {
  summary?: AchievementSummaryDTO
  className?: string
  size?: number
  strokeWidth?: number
  showLabel?: boolean
}

export function AchievementProgressRing({
  summary,
  className,
  size = 40,
  strokeWidth = 4,
  showLabel = true,
}: AchievementProgressRingProps) {
  if (!summary || summary.total_count <= 0) return null

  const progress = Math.max(0, Math.min(1, summary.unlocked_count / summary.total_count))
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const dashOffset = circumference * (1 - progress)

  return (
    <div className={cn('inline-flex items-center gap-2', className)}>
      <div className="relative shrink-0" style={{ width: size, height: size }}>
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="-rotate-90">
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke="currentColor"
            strokeWidth={strokeWidth}
            className="text-mga-border/80"
          />
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke="currentColor"
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={dashOffset}
            className="text-mga-accent transition-all duration-300"
          />
        </svg>
        <div className="absolute inset-0 flex items-center justify-center text-[10px] font-semibold text-mga-text">
          {Math.round(progress * 100)}%
        </div>
      </div>
      {showLabel && (
        <div className="min-w-0">
          <p className="text-[11px] font-medium uppercase tracking-wide text-mga-muted">
            Achievements
          </p>
          <p className="text-xs text-mga-text">
            {summary.unlocked_count}/{summary.total_count}
          </p>
        </div>
      )}
    </div>
  )
}
