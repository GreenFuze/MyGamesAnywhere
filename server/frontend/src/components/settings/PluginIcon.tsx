import { PLUGIN_LUCIDE_ICONS, CAPABILITY_META } from '@/lib/gameUtils'
import { cn } from '@/lib/utils'
import type { LucideIcon } from 'lucide-react'
import {
  Server,
  Gamepad2,
  Gamepad,
  Zap,
  Cloud,
  Search,
  Dice5,
  BookOpen,
  Store,
  Timer,
  Package,
  Joystick,
  Database,
  Trophy,
  RefreshCw,
  Puzzle,
} from 'lucide-react'

/** Registry of lucide icon name → component. Only icons we actually use. */
const ICON_REGISTRY: Record<string, LucideIcon> = {
  Server,
  Gamepad2,
  Gamepad,
  Zap,
  Cloud,
  Search,
  Dice5,
  BookOpen,
  Store,
  Timer,
  Package,
  Joystick,
  Database,
  Trophy,
  RefreshCw,
  Puzzle,
}

interface PluginIconProps {
  /** Plugin ID to look up icon for. */
  pluginId?: string
  /** Capability key (source, metadata, etc.) — fallback if no pluginId. */
  capability?: string
  size?: number
  className?: string
}

/**
 * Swappable icon component for plugins and capabilities.
 * Currently uses lucide-react; designed to be replaced with custom icons later.
 */
export function PluginIcon({ pluginId, capability, size = 20, className }: PluginIconProps) {
  // Try plugin-specific icon first.
  let iconName = pluginId ? PLUGIN_LUCIDE_ICONS[pluginId] : undefined

  // Fall back to capability icon.
  if (!iconName && capability) {
    iconName = CAPABILITY_META[capability]?.icon
  }

  // Final fallback.
  if (!iconName) iconName = 'Puzzle'

  const Icon = ICON_REGISTRY[iconName] ?? Puzzle

  return <Icon size={size} className={cn('text-mga-muted', className)} />
}
