import { Play } from 'lucide-react'
import { BrandIcon } from '@/components/ui/brand-icon'
import { cn } from '@/lib/utils'

type StatusBadgeKind = 'playable' | 'xcloud' | 'gamepass'

const STATUS_META: Record<
  StatusBadgeKind,
  { label: string; icon: 'play' | 'xcloud' | 'xbox'; className: string }
> = {
  playable: {
    label: 'Playable',
    icon: 'play',
    className: 'border-green-400/30 bg-black/70 text-green-300',
  },
  xcloud: {
    label: 'xCloud',
    icon: 'xcloud',
    className: 'border-sky-400/30 bg-black/70 text-white',
  },
  gamepass: {
    label: 'Game Pass',
    icon: 'xbox',
    className: 'border-emerald-400/30 bg-black/70 text-white',
  },
}

interface StatusBadgeProps {
  kind: StatusBadgeKind
  className?: string
}

function StatusBadgeIcon({ kind }: { kind: StatusBadgeKind }) {
  const icon = STATUS_META[kind].icon
  if (icon === 'xcloud') return <BrandIcon brand="xcloud" className="h-3.5 w-3.5" />
  if (icon === 'xbox') return <BrandIcon brand="xbox" className="h-3.5 w-3.5" />
  return <Play size={12} strokeWidth={2.25} />
}

export function StatusBadge({ kind, className }: StatusBadgeProps) {
  const meta = STATUS_META[kind]

  return (
    <span
      title={meta.label}
      aria-label={meta.label}
      role="img"
      className={cn(
        'inline-flex h-6 w-6 items-center justify-center rounded-full border backdrop-blur',
        meta.className,
        className,
      )}
    >
      <StatusBadgeIcon kind={kind} />
    </span>
  )
}
