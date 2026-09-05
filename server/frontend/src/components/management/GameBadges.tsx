import {
  Ban, BadgeCheck, Cloud, Hourglass, ImageOff, Puzzle, SearchX, ShoppingCart, Star, Ticket, Timer,
  Trophy, Users,
} from 'lucide-react'
import type { ComponentType } from 'react'
import { cn } from '@/lib/utils'
import { resolveBrandDefinition } from '@/lib/brands'
import type { BadgeIcon, GameBadge, GameSource } from '@/lib/gameBadges'

/**
 * A game's badges, each with a picture.
 *
 * The pictures come from the icon set the console already uses, so they take
 * the theme colour, stay sharp at any size, and add nothing to download. Brand
 * marks are the one thing an icon set cannot supply, and those already live as
 * real logos under /brands.
 *
 * Every badge keeps its words. An icon alone would make "Trial" and "Leaving
 * soon" a guessing game, and a picture cannot be read by a screen reader.
 */

const ICONS: Record<BadgeIcon, ComponentType<{ className?: string }>> = {
  missing: SearchX,
  unavailable: Ban,
  leaving: Timer,
  owned: BadgeCheck,
  purchase: ShoppingCart,
  trial: Hourglass,
  shared: Users,
  'game-pass': Ticket,
  cloud: Cloud,
  achievements: Trophy,
  kind: Puzzle,
  'no-artwork': ImageOff,
  favorite: Star,
}

const TONES = {
  neutral: 'border-mga-border bg-mga-elevated text-mga-muted',
  good: 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300',
  attention: 'border-amber-400/25 bg-amber-400/10 text-amber-200',
  danger: 'border-rose-400/25 bg-rose-400/10 text-rose-200',
}

export function GameBadgePill({ badge }: { badge: GameBadge }) {
  const Icon = ICONS[badge.icon]
  return (
    <span
      title={badge.title}
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-wider',
        TONES[badge.tone],
      )}
    >
      {/* Decorative: the label beside it already says what this is. */}
      {Icon && <Icon className="h-3 w-3 shrink-0" aria-hidden="true" />}
      {badge.label}
    </span>
  )
}

export function GameBadgeRow({ badges, className }: { badges: GameBadge[]; className?: string }) {
  if (badges.length === 0) return null
  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {badges.map((badge) => <GameBadgePill key={badge.id} badge={badge} />)}
    </div>
  )
}

/**
 * Where a game came from, with the provider's own mark.
 *
 * The label is whatever the user called the connection — "GF Google Drive" —
 * so the logo is looked up from the plugin behind it rather than from the
 * words. These are real logos that already ship with the console; an icon set
 * cannot supply a brand.
 */
export function SourcePill({ source }: { source: GameSource }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-mga-border bg-mga-elevated px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-wider text-mga-muted">
      <SourceMark pluginId={source.pluginId} />
      {source.name}
    </span>
  )
}

/** A provider's own logo, where one ships. */
export function SourceMark({ pluginId, className }: { pluginId: string; className?: string }) {
  const brand = resolveBrandDefinition(pluginId)
  if (!brand?.iconPath) return null
  return (
    <img
      src={brand.iconPath}
      alt=""
      aria-hidden="true"
      className={cn(
        'h-3.5 w-3.5 shrink-0 object-contain',
        brand.presentation === 'light_tile' && 'rounded-sm bg-white/90 p-px',
        className,
      )}
    />
  )
}

/** The platform, with its own mark where one ships. */
export function PlatformMark({ platform, className }: { platform: string; className?: string }) {
  const brand = resolveBrandDefinition(platform)
  if (!brand?.iconPath) return null
  return (
    <img
      src={brand.iconPath}
      alt=""
      aria-hidden="true"
      className={cn('h-3.5 w-3.5 shrink-0 object-contain', className)}
    />
  )
}
