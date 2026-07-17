import { Clock3, RefreshCw } from 'lucide-react'
import type { InstallationValidationScheduleConfig, InstallationValidationScheduleStatus } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useDateTimeFormat } from '@/hooks/useDateTimeFormat'

type Props = {
  status?: InstallationValidationScheduleStatus
  loading: boolean
  saving: boolean
  error?: string
  onChange: (config: InstallationValidationScheduleConfig) => void
}

const intervalOptions = [5, 15, 30, 60, 180, 360, 720, 1440]

function intervalLabel(minutes: number): string {
  if (minutes < 60) return `${minutes} minutes`
  if (minutes === 60) return '1 hour'
  if (minutes < 1440) return `${minutes / 60} hours`
  return '24 hours'
}

export function InstallationValidationScheduleCard({ status, loading, saving, error, onChange }: Props) {
  const { format } = useDateTimeFormat()
  if (loading && !status) {
    return <div className="rounded-mga border border-mga-border bg-mga-surface p-4 text-sm text-mga-muted">Loading automatic game checks…</div>
  }
  const enabled = status?.enabled ?? true
  const intervalMinutes = status?.interval_minutes ?? 15
  const running = status?.devices.some((device) => device.state === 'running') ?? false
  const nextRun = status?.devices.map((device) => device.next_run_at).filter(Boolean).sort()[0]
  const lastFinished = status?.devices.map((device) => device.last_finished_at).filter(Boolean).sort().at(-1)
  const options = intervalOptions.includes(intervalMinutes) ? intervalOptions : [...intervalOptions, intervalMinutes].sort((a, b) => a - b)

  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-lg">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            {running ? <RefreshCw className="h-4 w-4 animate-spin text-mga-accent" /> : <Clock3 className="h-4 w-4 text-emerald-400" />}
            <h3 className="font-bold text-mga-text">Automatic game checks</h3>
            <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${running ? 'bg-mga-accent/15 text-mga-accent' : enabled ? 'bg-emerald-500/15 text-emerald-300' : 'bg-mga-elevated text-mga-muted'}`}>
              {running ? 'Checking' : enabled ? 'Scheduled' : 'Paused'}
            </span>
          </div>
          <p className="mt-1 max-w-2xl text-sm text-mga-muted">MGA checks that installed game folders and launch files still work. It never removes or repairs anything automatically.</p>
          {enabled && nextRun ? <p className="mt-2 text-xs text-mga-muted">Next check: {format(nextRun)}</p> : null}
          {lastFinished ? <p className="mt-1 text-xs text-mga-muted">Last check: {format(lastFinished)}</p> : null}
          {error ? <p className="mt-2 text-xs text-red-400">{error}</p> : null}
        </div>
        <div className="flex items-center gap-2">
          <select
            aria-label="Automatic game check interval"
            className="h-9 rounded-mga border border-mga-border bg-mga-elevated px-2 text-xs text-mga-text disabled:opacity-50"
            value={intervalMinutes}
            disabled={!enabled || saving}
            onChange={(event) => onChange({ enabled, interval_minutes: Number(event.target.value) })}
          >
            {options.map((minutes) => <option key={minutes} value={minutes}>Every {intervalLabel(minutes)}</option>)}
          </select>
          <Button variant="outline" size="sm" disabled={saving} onClick={() => onChange({ enabled: !enabled, interval_minutes: intervalMinutes })}>
            {saving ? 'Saving…' : enabled ? 'Pause' : 'Enable'}
          </Button>
        </div>
      </div>
    </section>
  )
}
