import { Gamepad2, HardDrive, Play } from 'lucide-react'
import { BrandIcon } from '@/components/ui/brand-icon'
import { cn } from '@/lib/utils'

type StatusBadgeKind = 'playable' | 'xcloud' | 'gamepass' | 'emulator' | 'installed'

const STATUS_META: Record<
  StatusBadgeKind,
  { label: string; icon: 'play' | 'xcloud' | 'xbox' | 'emulator' | 'installed'; className: string }
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
  emulator: {
    label: 'Playable with an emulator',
    icon: 'emulator',
    className: 'border-violet-400/30 bg-black/70 text-violet-200',
  },
  installed: {
    label: 'Installed on a device',
    icon: 'installed',
    className: 'border-amber-400/30 bg-black/70 text-amber-200',
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
  if (icon === 'emulator') return <Gamepad2 size={13} strokeWidth={2.25} />
  if (icon === 'installed') return <HardDrive size={13} strokeWidth={2.25} />
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
