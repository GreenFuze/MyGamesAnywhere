import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  listIntegrations,
  listPlugins,
  getIntegrationStatus,
  checkIntegrationStatus,
  deleteIntegration,
  triggerScan,
  getStats,
  getSyncStatus,
  syncPush,
  syncPull,
  storeKey,
  clearKey,
  type Integration,
  type IntegrationStatusEntry,
  type PluginInfo,
} from '@/api/client'
import { CAPABILITY_META } from '@/lib/gameUtils'
import { useSSE } from '@/hooks/useSSE'
import { IntegrationGroupSection } from './IntegrationGroupSection'
import { LibraryStatsSummary } from './LibraryStatsSummary'
import { ScanSummary } from './ScanSummary'
import { AddIntegrationWizard, EditIntegrationDialog } from './IntegrationForm'
import { ConfirmDialog } from '@/components/ui/dialog'
import { ProgressBar } from '@/components/ui/progress-bar'
import { Button } from '@/components/ui/button'
import { Plus, RefreshCw } from 'lucide-react'

// ---------------------------------------------------------------------------
// Scan progress type (migrated from ScanTab)
// ---------------------------------------------------------------------------

type ScanProgress = {
  progress: number
  total: number
}

type ScanEventLogEntry = {
  id: string
  ts: string
  text: string
}

function readTimestamp(data: unknown): string {
  if (data && typeof data === 'object' && typeof (data as { ts?: unknown }).ts === 'string') {
    return (data as { ts: string }).ts
  }
  return new Date().toISOString()
}

function formatLogTime(ts: string): string {
  const parsed = new Date(ts)
  if (Number.isNaN(parsed.getTime())) return ''
  return parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function IntegrationsTab() {
  const queryClient = useQueryClient()
  const { subscribe } = useSSE()

  // ── Data queries ──

  const { data: integrations = [], isLoading: loadingIntegrations } = useQuery({
    queryKey: ['integrations'],
    queryFn: listIntegrations,
  })

  const { data: plugins = [] } = useQuery({
    queryKey: ['plugins'],
    queryFn: listPlugins,
  })

  const { data: stats } = useQuery({
    queryKey: ['stats'],
    queryFn: getStats,
  })

  const { data: syncStatus } = useQuery({
    queryKey: ['sync', 'status'],
    queryFn: getSyncStatus,
  })

  // ── Status check state ──

  const [statusMap, setStatusMap] = useState<Map<string, IntegrationStatusEntry>>(new Map())
  const [checkingAll, setCheckingAll] = useState(false)
  const [checkProgress, setCheckProgress] = useState({ current: 0, total: 0 })
  const [checkingIds, setCheckingIds] = useState<Set<string>>(new Set())

  // ── Scan state (absorbed from ScanTab) ──

  const [scanning, setScanning] = useState(false)
  const [scanError, setScanError] = useState('')
  const [currentPhase, setCurrentPhase] = useState('')
  const [scanStatusText, setScanStatusText] = useState('')
  const [scanTotalCount, setScanTotalCount] = useState(0)
  const [scanCompletedCount, setScanCompletedCount] = useState(0)
  const [integrationProgress, setIntegrationProgress] = useState<Map<string, ScanProgress>>(new Map())
  const [scanningIds, setScanningIds] = useState<Set<string>>(new Set())
  const [scanEventLog, setScanEventLog] = useState<ScanEventLogEntry[]>([])

  // ── Sync state (absorbed from SyncTab) ──

  const [pushing, setPushing] = useState(false)
  const [pulling, setPulling] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [syncMessage, setSyncMessage] = useState('')

  // ── UI state ──

  const [wizardOpen, setWizardOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Integration | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Integration | null>(null)

  const appendScanEvent = useCallback((text: string, data?: unknown) => {
    const ts = readTimestamp(data)
    setScanEventLog((prev) => {
      const next = [...prev, { id: `${ts}:${text}`, ts, text }]
      return next.slice(-5)
    })
  }, [])

  // ── SSE: Status check events ──

  useEffect(() => {
    const unsubs = [
      subscribe('integration_status_run_started', (data: unknown) => {
        const d = data as { total?: number }
        setCheckingAll(true)
        setCheckProgress({ current: 0, total: d.total ?? 0 })
      }),

      subscribe('integration_status_checked', (data: unknown) => {
        const d = data as {
          index?: number; total?: number; integration_id?: string
          plugin_id?: string; label?: string; status?: string; message?: string
        }
        if (d.integration_id) {
          const entry: IntegrationStatusEntry = {
            integration_id: d.integration_id,
            plugin_id: d.plugin_id ?? '',
            label: d.label ?? '',
            status: (d.status as 'ok' | 'error' | 'unavailable') ?? 'error',
            message: d.message ?? '',
          }
          setStatusMap((prev) => { const next = new Map(prev); next.set(d.integration_id!, entry); return next })
          setCheckingIds((prev) => { const next = new Set(prev); next.delete(d.integration_id!); return next })
        }
        if (d.index !== undefined) {
          setCheckProgress((prev) => ({ ...prev, current: d.index! }))
        }
      }),

      subscribe('integration_status_run_complete', () => {
        setCheckingAll(false)
        setCheckingIds(new Set())
      }),
    ]
    return () => unsubs.forEach((u) => u())
  }, [subscribe])

  // ── SSE: Scan events ──

  useEffect(() => {
    const unsubs = [
      // Scan lifecycle.
      subscribe('scan_started', (data: unknown) => {
        const d = data as { integration_count?: number }
        setScanning(true)
        setScanError('')
        setScanStatusText('Starting scan...')
        setScanTotalCount(d.integration_count ?? 0)
        setScanCompletedCount(0)
        setIntegrationProgress(new Map())
        setScanEventLog([])
        appendScanEvent(
          `Scan started${d.integration_count ? ` for ${d.integration_count} integration${d.integration_count === 1 ? '' : 's'}` : ''}.`,
          data,
        )
      }),

      subscribe('scan_integration_started', (data: unknown) => {
        const d = data as { integration_id?: string; label?: string }
        if (d.integration_id) {
          setScanningIds((prev) => new Set([...prev, d.integration_id!]))
          setScanStatusText(`Scanning ${d.label ?? d.integration_id}...`)
          setIntegrationProgress((prev) => {
            const next = new Map(prev)
            next.set(d.integration_id!, { progress: 0, total: 0 })
            return next
          })
        }
        appendScanEvent(`Integration started: ${d.label ?? d.integration_id ?? 'unknown source'}.`, data)
      }),

      // Source discovery progress.
      subscribe('scan_source_list_started', (data: unknown) => {
        const d = data as { integration_id?: string; plugin_id?: string }
        setScanStatusText(`Listing files from ${d.plugin_id ?? 'source'}...`)
        appendScanEvent(`Listing source content from ${d.plugin_id ?? 'source'}.`, data)
      }),
      subscribe('scan_source_list_complete', (data: unknown) => {
        const d = data as { integration_id?: string; file_count?: number; game_count?: number }
        if (d.file_count) {
          setScanStatusText(`Found ${d.file_count} files`)
        } else if (d.game_count) {
          setScanStatusText(`Found ${d.game_count} games`)
        }
        appendScanEvent(
          d.file_count
            ? `Source listing complete: ${d.file_count} files found.`
            : `Source listing complete: ${d.game_count ?? 0} games found.`,
          data,
        )
      }),

      // Scanner pipeline progress.
      subscribe('scan_scanner_started', (data: unknown) => {
        const d = data as { file_count?: number }
        setScanStatusText(`Detecting games in ${d.file_count ?? '?'} files...`)
        appendScanEvent(`Scanner started for ${d.file_count ?? '?'} files.`, data)
      }),
      subscribe('scan_scanner_complete', (data: unknown) => {
        const d = data as { group_count?: number }
        setScanStatusText(`Detected ${d.group_count ?? '?'} games`)
        appendScanEvent(`Scanner grouped ${d.group_count ?? 0} games.`, data)
      }),

      // Metadata enrichment progress.
      subscribe('scan_metadata_started', (data: unknown) => {
        const d = data as { game_count?: number; resolver_count?: number }
        setScanStatusText(`Enriching ${d.game_count ?? '?'} games with ${d.resolver_count ?? '?'} providers...`)
        setCurrentPhase('metadata')
        appendScanEvent(
          `Metadata started for ${d.game_count ?? '?'} games across ${d.resolver_count ?? '?'} providers.`,
          data,
        )
      }),
      subscribe('scan_metadata_phase', (data: unknown) => {
        const d = data as { phase?: string }
        const phaseLabel = d.phase === 'identify' ? 'Identifying' : d.phase === 'consensus' ? 'Building consensus' : d.phase === 'fill' ? 'Filling gaps' : d.phase ?? ''
        setCurrentPhase(d.phase ?? '')
        setScanStatusText(`Metadata: ${phaseLabel}...`)
        appendScanEvent(`Metadata phase: ${phaseLabel || 'working'}.`, data)
      }),
      subscribe('scan_metadata_plugin_started', (data: unknown) => {
        const d = data as { plugin_id?: string; batch_size?: number; phase?: string }
        setScanStatusText(`${d.plugin_id}: looking up ${d.batch_size ?? '?'} games...`)
        appendScanEvent(`${d.plugin_id ?? 'Provider'} started ${d.phase ?? 'metadata'} for ${d.batch_size ?? '?'} games.`, data)
      }),
      subscribe('scan_metadata_game_progress', (data: unknown) => {
        const d = data as {
          integration_id?: string
          plugin_id?: string
          game_index?: number
          game_count?: number
          game_title?: string
        }
        setScanStatusText(
          `${d.plugin_id ?? 'Provider'}: ${d.game_index ?? '?'} / ${d.game_count ?? '?'}${d.game_title ? ` - ${d.game_title}` : ''}`,
        )
        if (d.integration_id) {
          setIntegrationProgress((prev) => {
            const next = new Map(prev)
            next.set(d.integration_id!, {
              progress: d.game_index ?? 0,
              total: d.game_count ?? 0,
            })
            return next
          })
        }
        appendScanEvent(
          `${d.plugin_id ?? 'Provider'} ${d.game_index ?? '?'} / ${d.game_count ?? '?'}${d.game_title ? ` - ${d.game_title}` : ''}`,
          data,
        )
      }),
      subscribe('scan_metadata_plugin_complete', (data: unknown) => {
        const d = data as { plugin_id?: string; matched?: number; total?: number; filled?: number }
        if (d.matched != null) {
          setScanStatusText(`${d.plugin_id}: matched ${d.matched}/${d.total ?? '?'}`)
          appendScanEvent(`${d.plugin_id ?? 'Provider'} matched ${d.matched}/${d.total ?? '?'}.`, data)
        } else if (d.filled != null) {
          setScanStatusText(`${d.plugin_id}: filled ${d.filled} gaps`)
          appendScanEvent(`${d.plugin_id ?? 'Provider'} filled ${d.filled} metadata gaps.`, data)
        }
      }),
      subscribe('scan_metadata_plugin_error', (data: unknown) => {
        const d = data as { plugin_id?: string; error?: string }
        setScanStatusText(`${d.plugin_id ?? 'Provider'} error: ${d.error ?? 'unknown error'}`)
        appendScanEvent(`${d.plugin_id ?? 'Provider'} error: ${d.error ?? 'unknown error'}.`, data)
      }),
      subscribe('scan_metadata_consensus_complete', (data: unknown) => {
        const d = data as { identified?: number; unidentified?: number }
        setScanStatusText(`Consensus: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified`)
        appendScanEvent(
          `Consensus complete: ${d.identified ?? 0} identified, ${d.unidentified ?? 0} unidentified.`,
          data,
        )
      }),

      // Per-integration completion.
      subscribe('scan_integration_complete', (data: unknown) => {
        const d = data as { integration_id?: string; label?: string; games_found?: number }
        if (d.integration_id) {
          setScanningIds((prev) => { const next = new Set(prev); next.delete(d.integration_id!); return next })
          setScanCompletedCount((prev) => prev + 1)
          setIntegrationProgress((prev) => {
            const next = new Map(prev)
            const existing = next.get(d.integration_id!)
            if (existing) next.set(d.integration_id!, { ...existing, progress: existing.total || d.games_found || 0 })
            return next
          })
        }
        appendScanEvent(
          `Integration complete: ${d.label ?? d.integration_id ?? 'unknown'} (${d.games_found ?? 0} games).`,
          data,
        )
      }),

      // Scan finished.
      subscribe('scan_complete', (data: unknown) => {
        setScanning(false)
        setCurrentPhase('')
        setScanStatusText('')
        setScanningIds(new Set())
        appendScanEvent('Scan complete.', data)
        queryClient.invalidateQueries({ queryKey: ['stats'] })
        queryClient.invalidateQueries({ queryKey: ['games'] })
        queryClient.invalidateQueries({ queryKey: ['integration-games'] })
        queryClient.invalidateQueries({ queryKey: ['scan-reports'] })
      }),

      // Scan error.
      subscribe('scan_error', (data: unknown) => {
        const d = data as { error?: string }
        setScanError(d.error ?? 'Scan failed')
        setScanning(false)
        setScanStatusText('')
        appendScanEvent(`Scan failed: ${d.error ?? 'unknown error'}.`, data)
      }),
    ]
    return () => unsubs.forEach((u) => u())
  }, [appendScanEvent, queryClient, subscribe])

  // ── SSE: Sync events ──

  useEffect(() => {
    const unsubs = [
      subscribe('sync_operation_finished', (data: unknown) => {
        const d = data as { operation?: string; ok?: boolean; error?: string }
        if (d.ok) {
          setSyncMessage(`${d.operation === 'push' ? 'Push' : 'Pull'} completed successfully`)
        } else {
          setSyncError(d.error ?? 'Operation failed')
        }
        setPushing(false)
        setPulling(false)
        queryClient.invalidateQueries({ queryKey: ['sync', 'status'] })
      }),
      subscribe('sync_key_stored', () => {
        setSyncMessage('Encryption key stored')
        queryClient.invalidateQueries({ queryKey: ['sync', 'status'] })
      }),
      subscribe('sync_key_cleared', () => {
        setSyncMessage('Encryption key cleared')
        queryClient.invalidateQueries({ queryKey: ['sync', 'status'] })
      }),
    ]
    return () => unsubs.forEach((u) => u())
  }, [subscribe, queryClient])

  // ── Handlers: Status ──

  const handleCheckAll = useCallback(async () => {
    setCheckingAll(true)
    setCheckProgress({ current: 0, total: integrations.length })
    setCheckingIds(new Set(integrations.map((i) => i.id)))
    try {
      const results = await getIntegrationStatus()
      setStatusMap((prev) => {
        const next = new Map(prev)
        for (const r of results) next.set(r.integration_id, r)
        return next
      })
    } catch { /* SSE events may still arrive. */ } finally {
      setCheckingAll(false)
      setCheckingIds(new Set())
    }
  }, [integrations])

  const handleCheckOne = useCallback(async (id: string) => {
    setCheckingIds((prev) => new Set([...prev, id]))
    try {
      const result = await checkIntegrationStatus(id)
      setStatusMap((prev) => { const next = new Map(prev); next.set(result.integration_id, result); return next })
    } catch { /* Keep existing status. */ } finally {
      setCheckingIds((prev) => { const next = new Set(prev); next.delete(id); return next })
    }
  }, [])

  // ── Auto-check on first load ──

  const checkedOnceRef = useRef(false)
  useEffect(() => {
    if (integrations.length > 0 && !checkedOnceRef.current) {
      checkedOnceRef.current = true
      handleCheckAll()
    }
  }, [integrations.length, handleCheckAll])

  // ── Handlers: Scan ──

  const handleScanAll = useCallback(async () => {
    setScanning(true)
    setScanError('')
    try {
      await triggerScan()
    } catch (err) {
      setScanError(err instanceof Error ? err.message : 'Scan failed')
      setScanning(false)
    }
  }, [])

  const handleScanOne = useCallback(async (id: string) => {
    setScanningIds((prev) => new Set([...prev, id]))
    setScanError('')
    try {
      await triggerScan([id])
    } catch (err) {
      setScanError(err instanceof Error ? err.message : 'Scan failed')
    }
  }, [])

  const handleRefreshMetadata = useCallback(async () => {
    setScanning(true)
    setScanError('')
    try {
      await triggerScan(undefined, { metadataOnly: true })
    } catch (err) {
      setScanError(err instanceof Error ? err.message : 'Metadata refresh failed')
      setScanning(false)
    }
  }, [])

  // ── Handlers: Sync ──

  const handlePush = useCallback(async () => {
    setSyncError('')
    setSyncMessage('')
    setPushing(true)
    try {
      const result = await syncPush()
      setSyncMessage(`Push complete: ${result.integrations} integrations, ${result.settings} settings exported`)
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Push failed')
    } finally {
      setPushing(false)
    }
  }, [])

  const handlePull = useCallback(async () => {
    setSyncError('')
    setSyncMessage('')
    setPulling(true)
    try {
      const result = await syncPull()
      const r = result.result
      setSyncMessage(`Pull complete: ${r.integrations_added} added, ${r.integrations_updated} updated, ${r.settings_added + r.settings_updated} settings synced`)
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Pull failed')
    } finally {
      setPulling(false)
    }
  }, [])

  const handleStoreKey = useCallback(async (passphrase: string) => {
    if (!passphrase) return
    setSyncError('')
    setSyncMessage('')
    try {
      await storeKey(passphrase)
      setSyncMessage('Key stored securely')
      queryClient.invalidateQueries({ queryKey: ['sync', 'status'] })
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Failed to store key')
    }
  }, [queryClient])

  const handleClearKey = useCallback(async () => {
    setSyncError('')
    setSyncMessage('')
    try {
      await clearKey()
      setSyncMessage('Key cleared')
      queryClient.invalidateQueries({ queryKey: ['sync', 'status'] })
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Failed to clear key')
    }
  }, [queryClient])

  // ── Handlers: Delete ──

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    try {
      await deleteIntegration(deleteTarget.id)
      queryClient.invalidateQueries({ queryKey: ['integrations'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      queryClient.invalidateQueries({ queryKey: ['games'] })
      queryClient.invalidateQueries({ queryKey: ['integration-games'] })
      setStatusMap((prev) => { const next = new Map(prev); next.delete(deleteTarget.id); return next })
    } catch { /* Fail silently for now. */ }
  }, [deleteTarget, queryClient])

  // ── Group integrations by capability ──

  const groups = useMemo(() => {
    return groupIntegrationsByCapability(integrations, plugins)
  }, [integrations, plugins])

  const sortedGroupKeys = useMemo(() => {
    const keys = Array.from(groups.keys())
    return keys.sort((a, b) => {
      const orderA = CAPABILITY_META[a]?.order ?? 99
      const orderB = CAPABILITY_META[b]?.order ?? 99
      return orderA - orderB
    })
  }, [groups])

  // Build sync state object to pass down.
  const syncStateObj = useMemo(() => ({
    pushing, pulling, message: syncMessage, error: syncError,
  }), [pushing, pulling, syncMessage, syncError])

  // ── Render ──

  if (loadingIntegrations) {
    return <div className="text-mga-muted text-sm py-8 text-center">Loading integrations...</div>
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-mga-text">Integrations</h3>
          <p className="text-xs text-mga-muted mt-0.5">
            {integrations.length} integration{integrations.length !== 1 ? 's' : ''} configured
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={handleCheckAll} disabled={checkingAll || integrations.length === 0}>
            <RefreshCw size={14} className={checkingAll ? 'animate-spin' : ''} />
            {checkingAll ? 'Checking...' : 'Check All'}
          </Button>
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus size={14} />
            Add Integration
          </Button>
        </div>
      </div>

      {/* Check All progress bar */}
      {checkingAll && checkProgress.total > 0 && (
        <ProgressBar
          value={(checkProgress.current / checkProgress.total) * 100}
          label={`Checking ${checkProgress.current}/${checkProgress.total}`}
        />
      )}

      {/* Library stats summary */}
      {stats && stats.canonical_game_count > 0 && (
        <LibraryStatsSummary stats={stats} />
      )}

      {/* Scan summary / history */}
      {!scanning && <ScanSummary />}

      {/* Global scan progress */}
      {scanning && (
        <div className="border border-mga-border rounded-mga bg-mga-surface p-3 space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs font-medium text-mga-text">
              Scanning...{scanTotalCount > 0 && ` (${scanCompletedCount}/${scanTotalCount} integrations)`}
            </p>
            {currentPhase && (
              <span className="text-xs text-mga-accent font-medium capitalize">{currentPhase}</span>
            )}
          </div>
          <ProgressBar
            value={scanTotalCount > 0 ? (scanCompletedCount / scanTotalCount) * 100 : undefined}
            label={scanTotalCount > 0 ? `${scanCompletedCount}/${scanTotalCount}` : 'Scanning...'}
          />
          {scanStatusText && (
            <p className="text-xs text-mga-muted truncate">{scanStatusText}</p>
          )}
          {scanEventLog.length > 0 && (
            <div className="space-y-1 border-t border-mga-border/60 pt-2">
              {scanEventLog.map((entry) => (
                <div key={entry.id} className="flex items-start gap-2 text-xs text-mga-muted">
                  <span className="shrink-0 font-mono text-[10px] uppercase tracking-wide text-mga-accent/80">
                    {formatLogTime(entry.ts)}
                  </span>
                  <span className="min-w-0 flex-1">{entry.text}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Scan error */}
      {scanError && (
        <div className="border border-red-500/30 rounded-mga bg-red-500/10 p-3">
          <p className="text-xs text-red-400">{scanError}</p>
        </div>
      )}

      {/* Groups */}
      {integrations.length === 0 ? (
        <div className="text-center py-12 border border-mga-border rounded-mga bg-mga-surface">
          <p className="text-mga-muted text-sm mb-3">No integrations configured yet.</p>
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus size={14} />
            Add Your First Integration
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {sortedGroupKeys.map((cap) => (
            <IntegrationGroupSection
              key={cap}
              capability={cap}
              integrations={groups.get(cap)!}
              plugins={plugins}
              statusMap={statusMap}
              checkingIds={checkingIds}
              onCheck={handleCheckOne}
              onEdit={(integ) => setEditTarget(integ)}
              onDelete={(integ) => setDeleteTarget(integ)}
              // Scan props.
              stats={stats}
              scanningIds={scanningIds}
              integrationProgress={integrationProgress}
              onScan={handleScanOne}
              onScanGroup={cap === 'source' ? handleScanAll : undefined}
              onRefreshMetadata={cap === 'metadata' ? handleRefreshMetadata : undefined}
              scanning={scanning}
              // Sync props.
              syncStatus={syncStatus}
              syncState={syncStateObj}
              onPush={handlePush}
              onPull={handlePull}
              onStoreKey={handleStoreKey}
              onClearKey={handleClearKey}
            />
          ))}
        </div>
      )}

      {/* Add Integration Wizard */}
      {wizardOpen && (
        <AddIntegrationWizard
          onClose={() => setWizardOpen(false)}
          onSaved={() => {
            setWizardOpen(false)
            queryClient.invalidateQueries({ queryKey: ['integrations'] })
          }}
        />
      )}

      {/* Edit Integration Dialog */}
      {editTarget && (
        <EditIntegrationDialog
          integration={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={() => {
            setEditTarget(null)
            queryClient.invalidateQueries({ queryKey: ['integrations'] })
          }}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="Delete Integration"
        message={`Remove "${deleteTarget?.label}"? This cannot be undone.`}
        confirmLabel="Delete"
        confirmVariant="danger"
      />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Grouping helper
// ---------------------------------------------------------------------------

function groupIntegrationsByCapability(
  integrations: Integration[],
  plugins: PluginInfo[],
): Map<string, Integration[]> {
  const pluginMap = new Map(plugins.map((p) => [p.plugin_id, p]))
  const groups = new Map<string, Integration[]>()

  for (const integ of integrations) {
    const plugin = pluginMap.get(integ.plugin_id)
    // Use the plugin's primary capability, falling back to integration_type.
    const capability = plugin?.capabilities?.[0] ?? integ.integration_type ?? 'other'

    if (!groups.has(capability)) {
      groups.set(capability, [])
    }
    groups.get(capability)!.push(integ)
  }

  return groups
}
