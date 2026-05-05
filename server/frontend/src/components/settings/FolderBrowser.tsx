import { useState, useEffect, useCallback } from 'react'
import { browsePlugin, type BrowseFolder, type BrowseResponse } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Folder, ChevronRight, Home } from 'lucide-react'

interface FolderBrowserProps {
  pluginId: string
  initialPath?: string
  onSelect: (path: string) => void
  onSkip?: () => void
  browse?: (path: string) => Promise<BrowseResponse>
}

export function FolderBrowser({ pluginId, initialPath = '', onSelect, onSkip, browse }: FolderBrowserProps) {
  const [currentPath, setCurrentPath] = useState(initialPath)
  const [folders, setFolders] = useState<BrowseFolder[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

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

  // Fetch folders when path changes.
  useEffect(() => {
    fetchFolders(currentPath)
  }, [currentPath, fetchFolders])

  const navigateTo = (path: string) => {
    setCurrentPath(path)
  }

  // Build breadcrumb segments from the current path.
  const segments = currentPath ? currentPath.split('/') : []
  const breadcrumbs = segments.map((seg, i) => ({
    label: seg,
    path: segments.slice(0, i + 1).join('/'),
  }))

  return (
    <div className="space-y-4">
      {/* Breadcrumb navigation */}
      <div className="flex items-center gap-1 text-xs text-mga-muted overflow-x-auto">
        <button
          type="button"
          onClick={() => navigateTo('')}
          className="flex items-center gap-1 hover:text-mga-text transition-colors shrink-0"
        >
          <Home size={12} />
          <span>My Drive</span>
        </button>
        {breadcrumbs.map((bc) => (
          <span key={bc.path} className="flex items-center gap-1 shrink-0">
            <ChevronRight size={10} className="text-mga-border" />
            <button
              type="button"
              onClick={() => navigateTo(bc.path)}
              className="hover:text-mga-text transition-colors"
            >
              {bc.label}
            </button>
          </span>
        ))}
      </div>

      {/* Folder list */}
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
              onClick={() => fetchFolders(currentPath)}
              className="text-xs text-mga-accent mt-2 hover:underline"
            >
              Retry
            </button>
          </div>
        ) : folders.length === 0 ? (
          <div className="p-6 text-center text-mga-muted text-sm">
            No subfolders found
          </div>
        ) : (
          folders.map((folder) => (
            <button
              key={folder.path}
              type="button"
              onClick={() => navigateTo(folder.path)}
              className="w-full flex items-center gap-2 px-3 py-2 text-left text-sm text-mga-text hover:bg-mga-elevated transition-colors border-b border-mga-border last:border-b-0"
            >
              <Folder size={16} className="text-mga-accent shrink-0" />
              <span className="truncate">{folder.name}</span>
              <ChevronRight size={14} className="text-mga-muted ml-auto shrink-0" />
            </button>
          ))
        )}
      </div>

      {/* Actions */}
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
          <Button size="sm" onClick={() => onSelect(currentPath)}>
            Select This Folder
          </Button>
        </div>
      </div>

      {/* Current path display */}
      <p className="text-xs text-mga-muted">
        Selected: <span className="font-mono">{currentPath || '/ (root)'}</span>
      </p>
    </div>
  )
}
