import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { PLATFORM_META, platformEmoji, platformLabel } from '@/lib/gameUtils'

// ---------------------------------------------------------------------------
// Icon registry — swap in real SVG imports here later
// ---------------------------------------------------------------------------

type PlatformEntry = {
  icon?: ReactNode
  emoji: string
  label: string
}

/**
 * Build the lookup from PLATFORM_META. When real SVG icons are added,
 * import them and set the `icon` field; consumers won't need to change.
 */
function buildIconMap(): Record<string, PlatformEntry> {
  const map: Record<string, PlatformEntry> = {}
  for (const [key, meta] of Object.entries(PLATFORM_META)) {
    map[key] = { emoji: meta.emoji, label: meta.label }
  }
  return map
}

const ICON_MAP = buildIconMap()

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface PlatformIconProps {
  platform: string
  className?: string
  /** Show label text next to the icon. Default: true. */
  showLabel?: boolean
}

/**
 * Displays a platform icon (when available) or emoji + text fallback.
 * When real SVG icons are provided, only this file needs to change.
 */
export function PlatformIcon({ platform, className, showLabel = true }: PlatformIconProps) {
  const entry = ICON_MAP[platform]

  // Real icon available
  if (entry?.icon) {
    return (
      <span className={cn('inline-flex items-center gap-1 text-xs', className)}>
        {entry.icon}
        {showLabel && <span>{entry.label}</span>}
      </span>
    )
  }

  // Emoji + text fallback
  const emoji = entry?.emoji ?? platformEmoji(platform)
  const label = entry?.label ?? platformLabel(platform)

  return (
    <span className={cn('inline-flex items-center gap-1 text-xs', className)}>
      <span aria-hidden="true">{emoji}</span>
      {showLabel && <span>{label}</span>}
    </span>
  )
}
