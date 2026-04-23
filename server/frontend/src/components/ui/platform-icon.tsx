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

  const iconBrands: Record<string, string> = {
    windows_pc: 'windows',
    ms_dos: 'ms-dos',
    arcade: 'arcade',
    nes: 'nes',
    snes: 'snes',
    gb: 'gb',
    gbc: 'gbc',
    gba: 'gba',
    n64: 'n64',
    genesis: 'genesis',
    sega_master_system: 'sega_master_system',
    game_gear: 'game_gear',
    sega_cd: 'sega_cd',
    sega_32x: 'sega_32x',
    ps1: 'ps1',
    ps2: 'ps2',
    ps3: 'ps3',
    psp: 'psp',
    scummvm: 'scummvm',
    xbox_360: 'xbox',
    xbox_one: 'xbox',
    xbox_series: 'xbox',
  }
  for (const [platform, brand] of Object.entries(iconBrands)) {
    map[platform] = {
      ...map[platform],
      icon: <BrandIcon brand={brand} className="h-3.5 w-3.5" />,
    }
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
