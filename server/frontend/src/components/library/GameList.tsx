import type { GameDetailResponse, LibraryPrefs } from '@/api/client'
import { Skeleton } from '@/components/ui/skeleton'
import { GameRow } from '@/components/library/GameRow'
import { cn } from '@/lib/utils'

// ---------------------------------------------------------------------------
// Column definitions
// ---------------------------------------------------------------------------

type ColumnKey = LibraryPrefs['sortBy']

const COLUMNS: { key: ColumnKey | null; label: string; sortable: boolean }[] = [
  { key: 'title',        label: 'Title',      sortable: true },
  { key: 'platform',     label: 'Platform',   sortable: true },
  { key: null,            label: 'Sources',    sortable: false },
  { key: null,            label: 'Flags',      sortable: false },
  { key: null,            label: 'HLTB',       sortable: false },
  { key: null,            label: 'Conf.',      sortable: false },
  { key: null,            label: '',           sortable: false }, // action
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface GameListProps {
  games: GameDetailResponse[]
  isLoading: boolean
  sortBy: LibraryPrefs['sortBy']
  sortDir: LibraryPrefs['sortDir']
  onSortChange: (by: LibraryPrefs['sortBy'], dir: LibraryPrefs['sortDir']) => void
}

export function GameList({ games, isLoading, sortBy, sortDir, onSortChange }: GameListProps) {
  const handleHeaderClick = (key: ColumnKey | null) => {
    if (!key) return
    if (key === sortBy) {
      onSortChange(key, sortDir === 'asc' ? 'desc' : 'asc')
    } else {
      onSortChange(key, 'asc')
    }
  }

  // Sort arrow indicator
  const arrow = (key: ColumnKey | null) => {
    if (!key || key !== sortBy) return null
    return sortDir === 'asc' ? ' \u25B2' : ' \u25BC'
  }

  if (isLoading) {
    return (
      <div className="overflow-x-auto rounded-mga border border-mga-border">
        <table className="w-full min-w-[700px] border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-mga-border bg-mga-elevated/80">
              {COLUMNS.map((col, i) => (
                <th key={i} className="px-3 py-2 font-medium text-mga-muted">
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 8 }, (_, i) => (
              <tr key={i} className="border-b border-mga-border/80">
                <td className="px-3 py-2"><Skeleton className="h-4 w-40" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-16" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-12" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-20" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-8" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-6" /></td>
                <td className="px-3 py-2"><Skeleton className="h-4 w-12" /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <div className="overflow-x-auto rounded-mga border border-mga-border">
      <table className="w-full min-w-[700px] border-collapse text-left text-sm">
        <thead>
          <tr className="border-b border-mga-border bg-mga-elevated/80">
            {COLUMNS.map((col, i) => (
              <th
                key={i}
                className={cn(
                  'px-3 py-2 font-medium text-mga-muted',
                  col.sortable && 'cursor-pointer select-none hover:text-mga-text',
                )}
                onClick={() => handleHeaderClick(col.key)}
              >
                {col.label}
                {arrow(col.key)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {games.map((game) => (
            <GameRow key={game.id} game={game} />
          ))}
        </tbody>
      </table>
    </div>
  )
}
