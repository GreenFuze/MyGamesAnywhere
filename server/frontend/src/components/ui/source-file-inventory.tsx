import { useMemo } from 'react'
import type { GameFileDTO } from '@/api/client'
import { Badge } from '@/components/ui/badge'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const amount = bytes / 1024 ** exponent
  return `${amount.toFixed(amount >= 10 || exponent === 0 ? 0 : 1)} ${units[exponent]}`
}

export function SourceFileInventory({
  files,
  emptyMessage = 'No source files are available.',
}: {
  files: GameFileDTO[]
  emptyMessage?: string
}) {
  const summary = useMemo(() => {
    const sorted = [...files].sort((a, b) => a.path.localeCompare(b.path))
    return {
      paths: sorted.map((file) => file.path).join('\n'),
      totalSize: sorted.reduce((total, file) => total + (Number.isFinite(file.size) ? file.size : 0), 0),
      rows: Math.min(Math.max(sorted.length, 4), 14),
    }
  }, [files])

  if (files.length === 0) {
    return <p className="text-sm text-mga-muted">{emptyMessage}</p>
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        <Badge variant="muted">{files.length} {files.length === 1 ? 'file' : 'files'}</Badge>
        <Badge variant="muted">Total size: {formatBytes(summary.totalSize)}</Badge>
      </div>
      <textarea
        readOnly
        rows={summary.rows}
        value={summary.paths}
        className="min-h-28 w-full resize-y rounded-mga border border-mga-border bg-mga-bg px-3 py-2 font-mono text-xs leading-6 text-mga-text outline-none"
      />
    </div>
  )
}
