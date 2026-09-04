import { CheckCircle2, CircleAlert, CircleDashed, LoaderCircle, MinusCircle } from 'lucide-react'
import type { ScanJobStatus } from '@/api/client'
import { ProgressBar } from '@/components/ui/progress-bar'
import { PluginIcon } from '@/components/settings/PluginIcon'
import { describeScanProgress, type ScanConnectionView, type ScanTone } from '@/lib/scanProgress'
import { cn } from '@/lib/utils'

const TONE_ICON: Record<ScanTone, typeof LoaderCircle> = {
  pending: CircleDashed,
  running: LoaderCircle,
  done: CheckCircle2,
  skipped: MinusCircle,
  failed: CircleAlert,
}

const TONE_CLASS: Record<ScanTone, string> = {
  pending: 'text-mga-muted',
  running: 'text-mga-accent animate-spin',
  done: 'text-emerald-400',
  skipped: 'text-mga-muted',
  failed: 'text-rose-400',
}

/**
 * Shows what a library scan is actually doing.
 *
 * The server has always published per-connection phases, counts and the title
 * of the game being matched; the console rendered none of it, so a scan that
 * was working looked exactly like one that was stuck.
 */
export function ScanProgressPanel({ job, className }: { job: ScanJobStatus; className?: string }) {
  const view = describeScanProgress(job)

  return (
    <div
      className={cn('rounded-lg border border-mga-border bg-mga-elevated/40 p-4', className)}
      role="status"
      aria-live="polite"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="text-sm font-medium text-mga-text">{view.headline}</p>
        {!view.finished && view.gamesFound > 0 && (
          <p className="text-xs text-mga-muted">
            {view.gamesFound} {view.gamesFound === 1 ? 'game' : 'games'} so far
          </p>
        )}
      </div>

      <ProgressBar
        className="mt-3"
        value={view.overall.value}
        label={view.overall.label}
      />

      {/* Naming the item is the difference between "it is working" and "it is
          stuck", which is the whole point of this panel. */}
      {view.currentItem && (
        <p className="mt-2 truncate text-xs text-mga-muted" title={view.currentItem}>
          Working on {view.currentItem}
        </p>
      )}

      {view.connections.length > 0 && (
        <ul className="mt-4 space-y-3 border-t border-mga-border/70 pt-3">
          {view.connections.map((connection) => (
            <ConnectionRow key={connection.id} connection={connection} />
          ))}
        </ul>
      )}
    </div>
  )
}

function ConnectionRow({ connection }: { connection: ScanConnectionView }) {
  const Icon = TONE_ICON[connection.tone]

  return (
    <li className="flex items-start gap-3">
      <PluginIcon pluginId={connection.pluginId} capability="source" size={16} className="mt-0.5 shrink-0 text-mga-muted" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3">
          <span className="truncate text-xs font-medium text-mga-text">{connection.label}</span>
          {connection.gamesFound !== null && (
            <span className="shrink-0 text-xs text-mga-muted">
              {connection.gamesFound} {connection.gamesFound === 1 ? 'game' : 'games'}
            </span>
          )}
        </div>
        {connection.bar && (
          <ProgressBar className="mt-1.5" value={connection.bar.value} label={connection.bar.label} />
        )}
        {!connection.bar && connection.detail && (
          <p className={cn('mt-0.5 text-xs', connection.tone === 'failed' ? 'text-rose-300' : 'text-mga-muted')}>
            {connection.detail}
          </p>
        )}
      </div>
      <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', TONE_CLASS[connection.tone])} aria-hidden />
    </li>
  )
}
