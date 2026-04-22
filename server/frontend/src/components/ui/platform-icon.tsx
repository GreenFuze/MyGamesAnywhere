import type { ReactNode } from 'react'
import { BrandIcon } from '@/components/ui/brand-icon'
import { PLATFORM_META, platformEmoji, platformLabel } from '@/lib/displayText'
import { cn } from '@/lib/utils'

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

  map.windows_pc = {
    ...map.windows_pc,
    icon: <BrandIcon brand="windows" className="h-3.5 w-3.5" />,
  }
  map.ms_dos = {
    ...map.ms_dos,
    icon: <BrandIcon brand="ms-dos" className="h-3.5 w-3.5" />,
  }
  map.arcade = {
    ...map.arcade,
    icon: <BrandIcon brand="arcade" className="h-3.5 w-3.5" />,
  }
  map.gba = {
    ...map.gba,
    icon: <BrandIcon brand="gba" className="h-3.5 w-3.5" />,
  }
  map.scummvm = {
    ...map.scummvm,
    icon: <BrandIcon brand="scummvm" className="h-3.5 w-3.5" />,
  }
  map.xbox_360 = {
    ...map.xbox_360,
    icon: <BrandIcon brand="xbox" className="h-3.5 w-3.5" />,
  }
  map.xbox_one = {
    ...map.xbox_one,
    icon: <BrandIcon brand="xbox" className="h-3.5 w-3.5" />,
  }
  map.xbox_series = {
    ...map.xbox_series,
    icon: <BrandIcon brand="xbox" className="h-3.5 w-3.5" />,
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
