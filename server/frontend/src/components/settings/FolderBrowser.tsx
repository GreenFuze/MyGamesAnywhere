import { useState, useEffect, useCallback, useMemo } from 'react'
import { browsePlugin, type BrowseFolder, type BrowseResponse } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Folder, ChevronRight, Home, Users } from 'lucide-react'
import {
  buildInitialFolderHistory,
  filterBrowsableFolders,
  type FolderBrowseLocation,
} from '@/lib/driveFolderBrowse'

export type FolderSelection = {
  path: string
  object_id?: string
}

interface FolderBrowserProps {
  pluginId: string
  initialPath?: string
  initialObjectId?: string
  onSelect: (selection: FolderSelection) => void
  onSkip?: () => void
  browse?: (path: string) => Promise<BrowseResponse>
  allowSharedLocations?: boolean
}

export function FolderBrowser({
  pluginId,
  initialPath = '',
  initialObjectId,
  onSelect,
  onSkip,
  browse,
  allowSharedLocations = false,
}: FolderBrowserProps) {
  const [history, setHistory] = useState<FolderBrowseLocation[]>(() => (
    buildInitialFolderHistory(initialPath, initialObjectId, allowSharedLocations)
  ))
  const [folders, setFolders] = useState<BrowseFolder[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const current = history[history.length - 1]

  const fetchFolders = useCallback(async (path: string) => {
    setLoading(true)
    setError(null)
    try {
      const result = browse ? await browse(path) : await browsePlugin(pluginId, path)
      setFolders(result.folders ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to browse folders')
      setFolders([])
    } finally {
      setLoading(false)
    }
  }, [browse, pluginId])

  useEffect(() => {
    fetchFolders(current.browsePath)
  }, [current.browsePath, fetchFolders])

  const visibleFolders = useMemo(
    () => filterBrowsableFolders(folders, allowSharedLocations),
    [allowSharedLocations, folders],
  )

  const navigateTo = (folder: BrowseFolder) => {
    setHistory((previous) => [...previous, {
      name: folder.name,
      browsePath: folder.path,
      displayPath: folder.display_path ?? folder.path,
      objectId: folder.object_id,
      selectable: folder.selectable !== false,
      locationKind: folder.location_kind,
    }])
  }

  const navigateToHistoryIndex = (index: number) => {
    setHistory((previous) => previous.slice(0, index + 1))
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-1 text-xs text-mga-muted overflow-x-auto">
        {history.map((location, index) => (
          <span key={`${index}:${location.browsePath}`} className="flex items-center gap-1 shrink-0">
            {index > 0 && <ChevronRight size={10} className="text-mga-border" />}
            <button
              type="button"
              onClick={() => navigateToHistoryIndex(index)}
              className="flex items-center gap-1 hover:text-mga-text transition-colors"
            >
              {index === 0 && <Home size={12} />}
              <span>{location.name}</span>
            </button>
          </span>
        ))}
      </div>

      <div className="border border-mga-border rounded-mga overflow-hidden max-h-64 overflow-y-auto">
        {loading ? (
          <div className="p-6 text-center text-mga-muted text-sm animate-pulse">
            Loading folders...
          </div>
        ) : error ? (
          <div className="p-4 text-center">
            <p className="text-sm text-red-400">{error}</p>
            <button
              type="button"
              onClick={() => fetchFolders(current.browsePath)}
              className="text-xs text-mga-accent mt-2 hover:underline"
            >
              Retry
            </button>
          </div>
        ) : visibleFolders.length === 0 ? (
          <div className="p-6 text-center text-mga-muted text-sm">
            No subfolders found
          </div>
        ) : (
          visibleFolders.map((folder) => {
            const isSharedLocation = folder.location_kind === 'shared_with_me'
            const Icon = isSharedLocation ? Users : Folder
            return (
              <button
                key={folder.path}
                type="button"
                onClick={() => navigateTo(folder)}
                className="w-full flex items-center gap-2 px-3 py-2 text-left text-sm text-mga-text hover:bg-mga-elevated transition-colors border-b border-mga-border last:border-b-0"
              >
                <Icon size={16} className="text-mga-accent shrink-0" />
                <span className="truncate">{folder.name}</span>
                {folder.location_kind && <span className="ml-auto text-xs text-mga-muted">Location</span>}
                <ChevronRight size={14} className={`${folder.location_kind ? '' : 'ml-auto '}text-mga-muted shrink-0`} />
              </button>
            )
          })
        )}
      </div>

      <div className="flex items-center justify-between pt-1">
        {onSkip && (
          <button
            type="button"
            onClick={onSkip}
            className="text-xs text-mga-muted hover:text-mga-text transition-colors"
          >
            Skip (use entire Drive)
          </button>
        )}
        <div className="flex gap-2 ml-auto">
          <Button
            size="sm"
            disabled={!current.selectable}
            onClick={() => onSelect({ path: current.displayPath, object_id: current.objectId })}
          >
            {current.selectable ? 'Select This Folder' : 'Choose a Folder'}
          </Button>
        </div>
      </div>

      <p className="text-xs text-mga-muted">
        Selected: <span className="font-mono">{current.displayPath || '/ (My Drive root)'}</span>
      </p>
    </div>
  )
}
