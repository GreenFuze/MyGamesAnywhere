import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getScanReports, type ScanReport } from '@/api/client'
import { Skeleton } from '@/components/ui/skeleton'
import { useDateTimeFormat } from '@/hooks/useDateTimeFormat'
import { ChevronDown, ChevronRight, Clock, History } from 'lucide-react'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const secs = seconds % 60
  return secs > 0 ? `${minutes}m ${secs}s` : `${minutes}m`
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

// ---------------------------------------------------------------------------
// Single report card
// ---------------------------------------------------------------------------

function ReportCard({ report, expanded, onToggle }: {
  report: ScanReport
  expanded: boolean
  onToggle: () => void
}) {
  const { format } = useDateTimeFormat()

  const hasChanges = report.games_added > 0 || report.games_removed > 0

  return (
    <div className="border border-mga-border rounded-mga bg-mga-surface overflow-hidden">
      {/* Header — clickable toggle */}
      <button
        onClick={onToggle}
        className="w-full flex items-center justify-between p-3 text-left hover:bg-mga-hover transition-colors"
      >
        <div className="flex items-center gap-2 min-w-0">
          {expanded ? <ChevronDown size={14} className="text-mga-muted shrink-0" /> : <ChevronRight size={14} className="text-mga-muted shrink-0" />}
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-mga-text">
                {report.metadata_only ? 'Metadata Refresh' : 'Source Scan'}
              </span>
              <span className="text-xs text-mga-muted">{timeAgo(report.finished_at)}</span>
            </div>

            {/* Diff summary */}
            <div className="flex items-center gap-2 mt-0.5">
              {hasChanges ? (
                <>
                  {report.games_added > 0 && (
                    <span className="text-xs text-green-400">+{report.games_added} added</span>
                  )}
                  {report.games_removed > 0 && (
                    <span className="text-xs text-red-400">-{report.games_removed} removed</span>
                  )}
                </>
              ) : (
                <span className="text-xs text-mga-muted">No changes</span>
              )}
              <span className="text-xs text-mga-muted">&middot; {report.total_games} total</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-1 text-mga-muted shrink-0">
          <Clock size={12} />
          <span className="text-xs">{formatDuration(report.duration_ms)}</span>
        </div>
      </button>

      {/* Expanded: per-integration breakdown */}
      {expanded && report.integration_results && report.integration_results.length > 0 && (
        <div className="border-t border-mga-border px-3 py-2 space-y-1">
          <p className="text-xs text-mga-muted font-medium mb-1">
            {format(report.finished_at)}
          </p>
          {report.integration_results.map((r) => (
            <div key={r.integration_id} className="flex items-center justify-between text-xs">
              <span className="text-mga-text truncate">{r.label || r.integration_id}</span>
              <div className="flex items-center gap-2 shrink-0">
                <span className="text-mga-muted">{r.games_found} games</span>
                {r.games_added > 0 && <span className="text-green-400">+{r.games_added}</span>}
                {r.games_removed > 0 && <span className="text-red-400">-{r.games_removed}</span>}
                {r.error && <span className="text-red-400">error</span>}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// ScanSummary — shows most recent scan report + history toggle
// ---------------------------------------------------------------------------

export function ScanSummary() {
  const { data: reports = [], isPending } = useQuery({
    queryKey: ['scan-reports'],
    queryFn: () => getScanReports(10),
  })

  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [showHistory, setShowHistory] = useState(false)

  if (isPending) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 2 }, (_, index) => (
          <div
            key={index}
            className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface"
          >
            <div className="flex items-center justify-between gap-3 p-3">
              <div className="min-w-0 flex-1 space-y-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-3 w-48" />
              </div>
              <Skeleton className="h-4 w-14" />
            </div>
          </div>
        ))}
      </div>
    )
  }

  if (reports.length === 0) return null

  const latest = reports[0]
  const history = reports.slice(1)

  return (
    <div className="space-y-2">
      {/* Most recent report — always visible */}
      <ReportCard
        report={latest}
        expanded={expandedId === latest.id}
        onToggle={() => setExpandedId((prev) => (prev === latest.id ? null : latest.id))}
      />

      {/* History toggle + older reports */}
      {history.length > 0 && (
        <>
          <button
            onClick={() => setShowHistory((prev) => !prev)}
            className="flex items-center gap-1 text-xs text-mga-accent hover:underline"
          >
            <History size={12} />
            {showHistory ? 'Hide history' : `View ${history.length} older scan${history.length !== 1 ? 's' : ''}`}
          </button>

          {showHistory && (
            <div className="space-y-2">
              {history.map((r) => (
                <ReportCard
                  key={r.id}
                  report={r}
                  expanded={expandedId === r.id}
                  onToggle={() => setExpandedId((prev) => (prev === r.id ? null : r.id))}
                />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
