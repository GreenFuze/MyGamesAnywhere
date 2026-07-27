import { Database, FolderArchive, Gamepad2, HardDrive, Monitor } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import {
  gameFileLocationSaveLabel,
  type GameFileLocationKind,
  type GameFileLocationView,
} from '@/lib/gameFileLocations'
import { cn } from '@/lib/utils'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const amount = bytes / 1024 ** exponent
  return `${amount.toFixed(amount >= 10 || exponent === 0 ? 0 : 1)} ${units[exponent]}`
}

function locationIcon(kind: GameFileLocationKind) {
  switch (kind) {
    case 'source': return <Database size={18} />
    case 'prepared': return <FolderArchive size={18} />
    case 'installed': return <HardDrive size={18} />
    case 'emulator': return <Gamepad2 size={18} />
  }
}

function locationKindLabel(kind: GameFileLocationKind): string {
  switch (kind) {
    case 'source': return 'Original copy'
    case 'prepared': return 'Temporary copy'
    case 'installed': return 'Installed copy'
    case 'emulator': return 'Emulator choice'
  }
}

function LocationCard({ location }: { location: GameFileLocationView }) {
  const saveLabel = gameFileLocationSaveLabel(location)
  const knownSave = saveLabel !== 'Save compatibility not known'
  return (
    <article className="rounded-[18px] border border-white/[0.06] bg-black/10 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 rounded-xl bg-white/[0.05] p-2 text-mga-accent">{locationIcon(location.kind)}</span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-sm font-semibold text-white">{location.title}</p>
              <Badge variant="muted">{locationKindLabel(location.kind)}</Badge>
            </div>
            <p className="mt-1 text-xs leading-5 text-white/52">{location.context}</p>
          </div>
        </div>
        <Badge variant={/ready|installed|available|connected/i.test(location.status) ? 'playable' : 'muted'}>
          {location.status}
        </Badge>
      </div>

      <div className="mt-4 grid gap-2 text-xs text-white/58 sm:grid-cols-2">
        <p><span className="text-white/40">Player</span><br />{location.ownerProfileName}</p>
        {location.deviceName ? <p><span className="text-white/40">Device</span><br />{location.deviceName} · {location.osUser}</p> : null}
        {location.fileCount > 0 ? (
          <p>
            <span className="text-white/40">Files</span><br />
            {location.fileCount}{formatBytes(location.size) ? ` · ${formatBytes(location.size)}` : ''}
          </p>
        ) : null}
        <p>
          <span className="text-white/40">Saves</span><br />
          <span className={knownSave ? 'text-white/72' : 'text-amber-100/80'}>{saveLabel}</span>
        </p>
      </div>

      {location.path ? (
        <details className="mt-3 rounded-xl border border-white/[0.06] bg-black/10 px-3 py-2">
          <summary className="cursor-pointer text-xs font-medium text-white/68">Show location</summary>
          <p className="mt-2 break-all font-mono text-xs leading-5 text-white/48">{location.path}</p>
        </details>
      ) : null}

      <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
        <details className="text-xs text-white/48">
          <summary className="cursor-pointer">Why you can see this</summary>
          <ul className="mt-2 space-y-1 pl-4">
            {location.accessEvidence.map((evidence) => <li key={evidence}>{evidence}</li>)}
          </ul>
        </details>
        <a href={location.manageHref} className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}>
          {location.kind === 'installed' ? <Monitor size={14} /> : null}
          {location.manageLabel}
        </a>
      </div>
    </article>
  )
}

export function GameFileLocations({ locations }: { locations: GameFileLocationView[] }) {
  if (locations.length === 0) {
    return <p className="text-sm text-white/58">No copies or file locations are available for this game.</p>
  }
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {locations.map((location) => <LocationCard key={location.id} location={location} />)}
    </div>
  )
}
