import type {
  IntegrationRefreshJobStatus, ScanJobIntegrationStatus, ScanJobProgress, ScanJobStatus,
} from '@/api/client'

/** Scan states after which nothing more will happen. */
export const SCAN_TERMINAL_STATES = new Set(['completed', 'failed', 'cancelled'])

export type ScanTone = 'pending' | 'running' | 'done' | 'skipped' | 'failed'

export interface ScanBar {
  /** Percentage 0-100, or undefined when the total is genuinely unknown. */
  value?: number
  label: string
}

export interface ScanConnectionView {
  id: string
  label: string
  pluginId?: string
  tone: ScanTone
  /** What this connection is doing right now, or why it stopped. */
  detail: string | null
  bar: ScanBar | null
  gamesFound: number | null
}

export interface ScanProgressView {
  headline: string
  /** The item being worked on right now, when the server can name one. */
  currentItem: string | null
  overall: ScanBar
  gamesFound: number
  connections: ScanConnectionView[]
  finished: boolean
}

const TONE_BY_STATUS: Record<string, ScanTone> = {
  running: 'running',
  completed: 'done',
  skipped: 'skipped',
  cancelled: 'skipped',
  failed: 'failed',
  error: 'failed',
}

/** Reasons the server reports for skipping a connection, in plain words. */
const SKIP_REASONS: Record<string, string> = {
  plugin_not_found: 'Its provider plugin is not installed.',
  invalid_config: 'Its configuration could not be read.',
  auth_required: 'It needs you to sign in again.',
  no_source_capability: 'This provider does not supply games.',
  source_error: 'The provider returned an error.',
}

/**
 * Server phase strings are internal vocabulary. Say what is happening instead.
 * Anything unrecognised falls through unchanged rather than being hidden.
 */
function readablePhase(phase: string | undefined, subject: string): string | null {
  const value = phase?.trim().toLowerCase()
  if (!value) return null
  if (value.startsWith('metadata via ')) return 'Matching metadata…'
  switch (value) {
    case 'queued':
    case 'starting':
    case 'scan started':
      return 'Starting…'
    case 'listing source content':
      return `Reading ${subject}…`
    case 'scanning files':
      return 'Scanning files…'
    case 'metadata enrichment':
      return 'Matching metadata…'
    case 'persisting results':
      return 'Saving results…'
    case 'cancelling':
      return 'Cancelling…'
    case 'refreshing_metadata':
      return 'Refreshing metadata…'
    case 'refreshing_achievements':
      return 'Refreshing achievements…'
    default:
      return phase!.trim()
  }
}

function percentage(progress: ScanJobProgress | undefined): number | undefined {
  if (!progress || progress.indeterminate) return undefined
  const total = progress.total ?? 0
  if (total <= 0) return undefined
  return Math.min(100, Math.max(0, (progress.current / total) * 100))
}

function barFor(progress: ScanJobProgress | undefined, fallbackLabel: string): ScanBar | null {
  if (!progress) return null
  const unit = progress.unit ?? 'items'
  const total = progress.total ?? 0
  if (progress.indeterminate || total <= 0) {
    // A walk knows what it has seen but not what remains. The running count is
    // the clearest evidence of movement, so keep it even without a percentage.
    if (progress.current > 0) {
      return { label: `${fallbackLabel} ${progress.current} ${unit} so far` }
    }
    return { label: fallbackLabel }
  }
  return { value: percentage(progress), label: `${progress.current} of ${total} ${unit}` }
}

/**
 * Describes one connection's share of the scan.
 *
 * A storefront connection reports nothing at all between "started listing" and
 * "finished listing" — that is one blocking call into the provider and it can
 * take minutes. Rather than render a dead panel, say which connection is busy
 * and show an indeterminate bar, so the operator can tell the difference
 * between working and stuck.
 */
export function describeConnection(entry: ScanJobIntegrationStatus): ScanConnectionView {
  const label = entry.label?.trim() || entry.integration_id
  const tone = TONE_BY_STATUS[entry.status] ?? 'pending'

  let detail: string | null = readablePhase(entry.phase, `your ${label} library`)
  if (tone === 'failed' && entry.error) detail = entry.error
  else if (tone === 'skipped') detail = SKIP_REASONS[entry.reason ?? ''] ?? entry.reason ?? 'Skipped.'
  // A finished row already carries a tick and a game count; repeating
  // "completed" underneath says nothing.
  else if (tone === 'done') detail = null

  let bar: ScanBar | null = null
  if (tone === 'running') {
    // Metadata runs after listing, so when it is active it is the more
    // informative of the two.
    const metadata = barFor(entry.metadata_progress, 'Matching metadata…')
    const source = barFor(entry.source_progress, detail ?? `Reading ${label}…`)
    bar = metadata ?? source ?? { label: detail ?? `Working on ${label}…` }
  }

  return {
    id: entry.integration_id,
    label,
    pluginId: entry.plugin_id,
    tone,
    detail,
    bar,
    gamesFound: typeof entry.games_found === 'number' ? entry.games_found : null,
  }
}

/** The last game title the server named, if it named one recently. */
function currentItemFrom(job: ScanJobStatus): string | null {
  const events = job.recent_events ?? []
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index]
    if (event.type !== 'scan_metadata_game_progress') continue
    const title = event.data?.game_title
    if (typeof title === 'string' && title.trim() !== '') return title.trim()
  }
  return null
}

/**
 * Turns a scan job into something an operator can read at a glance.
 *
 * The server already publishes per-connection phases, counts and game titles;
 * none of it was rendered, so a scan looked identical to nothing happening.
 */
export function describeScanProgress(job: ScanJobStatus): ScanProgressView {
  const connections = (job.integrations ?? []).map(describeConnection)
  const gamesFound = connections.reduce((total, entry) => total + (entry.gamesFound ?? 0), 0)
  const finished = SCAN_TERMINAL_STATES.has(job.status)

  const total = job.integration_count
  const completed = job.integrations_completed
  const overall: ScanBar = finished
    // The headline already states the outcome; the bar reports the count.
    ? { value: 100, label: total > 0 ? `${total} ${total === 1 ? 'connection' : 'connections'}` : 'Finished' }
    : total > 0
      ? { value: Math.min(100, (completed / total) * 100), label: `${completed} of ${total} connections` }
      : { label: 'Preparing…' }

  let headline: string
  if (finished) headline = describeOutcome(job, gamesFound)
  else if (job.current_integration_label) headline = `Scanning ${job.current_integration_label}`
  else if (job.current_phase) headline = readablePhase(job.current_phase, 'your library') ?? 'Scanning'
  else if (job.status === 'cancelling') headline = 'Cancelling the scan'
  else headline = 'Starting the scan'

  return {
    headline,
    currentItem: finished ? null : currentItemFrom(job),
    overall,
    gamesFound,
    connections,
    finished,
  }
}

function describeOutcome(job: ScanJobStatus, gamesFound: number): string {
  if (job.status === 'cancelled') return 'Scan cancelled'
  if (job.status === 'failed') return job.error?.trim() || 'Scan failed'
  return `Scan complete · ${gamesFound} ${gamesFound === 1 ? 'game' : 'games'} found`
}

export interface RefreshProgressView {
  headline: string
  currentItem: string | null
  bar: ScanBar
  finished: boolean
  failed: boolean
}

/**
 * The same treatment for a single-connection refresh.
 *
 * This job already reports items_completed, items_total and the name of the
 * item in flight. The console said only "Refresh started." and then went quiet,
 * which is the same blindness a scan had.
 */
export function describeRefreshProgress(job: IntegrationRefreshJobStatus): RefreshProgressView {
  const finished = SCAN_TERMINAL_STATES.has(job.status)
  const failed = job.status === 'failed'
  const total = job.items_total
  const completed = job.items_completed

  let headline: string
  if (failed) headline = job.error?.trim() || 'Refresh failed'
  else if (job.status === 'cancelled') headline = 'Refresh cancelled'
  else if (finished) headline = `Refreshed ${total} ${total === 1 ? 'game' : 'games'}`
  else headline = readablePhase(job.phase, `your ${job.label} library`) ?? 'Refreshing…'

  const bar: ScanBar = finished
    ? { value: 100, label: headline }
    : total > 0
      ? { value: Math.min(100, (completed / total) * 100), label: `${completed} of ${total} games` }
      : { label: 'Preparing…' }

  return {
    headline,
    currentItem: finished ? null : job.current_item?.trim() || null,
    bar,
    finished,
    failed,
  }
}
