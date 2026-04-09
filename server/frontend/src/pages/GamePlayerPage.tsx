import { useEffect, useMemo, useRef, useState, type LegacyRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Download, ExternalLink, Maximize2, Minimize2, PlayCircle, Upload } from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
  getCacheJob,
  type FrontendConfig,
  getFrontendConfig,
  getGame,
  getGameSaveSyncSlot,
  listGameSaveSyncSlots,
  prepareGameCache,
  putGameSaveSyncSlot,
  type SaveSyncSlotSummary,
} from '@/api/client'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import {
  browserPlaySourceContext,
  browserPlaySourceLabel,
  browserPlaySourceOptionLabel,
  browserPlayRuntimeLabel,
  browserPlaySelectionIsReady,
  browserPlaySelectionRequiresPrepare,
  buildBrowserPlaySession,
  buildBrowserPlayerUrl,
  clearBrowserPlaySession,
  getBrowserPlaySelectionIssue,
  listBrowserPlaySelections,
  persistBrowserPlaySession,
  readBrowserPlaySourcePreference,
  sessionSupportsSaveSync,
  selectBrowserPlaySelection,
  writeBrowserPlaySourcePreference,
  type BrowserPlaySession,
} from '@/lib/browserPlay'
import { hasBrowserPlaySupport } from '@/lib/gameUtils'
import {
  SAVE_SYNC_SLOT_IDS,
  buildSaveSyncSnapshot,
  computeLocalSnapshotHash,
  extractRuntimeFilesFromSnapshot,
  type RuntimeBridgeCommand,
  type RuntimeBridgeEvent,
  type RuntimeSaveSnapshot,
} from '@/lib/saveSync'

function BrowserRuntimeFrame({
  runtime,
  iframeRef,
  src,
  title,
}: {
  runtime: BrowserPlaySession['runtime']
  iframeRef: LegacyRef<HTMLIFrameElement>
  src: string
  title: string
}) {
  // Absolute positioning ensures the iframe fills its wrapper's physical
  // dimensions without depending on CSS percentage height resolution through
  // the flex chain. Runtime-specific classes can be added per branch later.
  const className =
    runtime === 'emulatorjs'
      ? 'absolute inset-0 h-full w-full border-0 bg-black'
      : 'absolute inset-0 h-full w-full border-0 bg-black'

  return (
    <iframe
      ref={iframeRef}
      src={src}
      title={title}
      allow="autoplay; fullscreen; gamepad"
      className={className}
    />
  )
}

type PendingBridgeRequest = {
  resolve: (value?: any) => void
  reject: (reason?: unknown) => void
  kind: 'export' | 'import'
}

function activeSaveSyncIntegrationId(frontendConfig: FrontendConfig | undefined): string | null {
  const value = frontendConfig?.saveSyncActiveIntegrationId
  return typeof value === 'string' && value.trim().length > 0 ? value : null
}

export function GamePlayerPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const { recordLaunch } = useRecentPlayed()
  const [sessionToken, setSessionToken] = useState<string | null>(null)
  const [selectedSlot, setSelectedSlot] = useState<(typeof SAVE_SYNC_SLOT_IDS)[number]>('autosave')
  const [bridgeReady, setBridgeReady] = useState(false)
  const [bridgeSupportsSaveSync, setBridgeSupportsSaveSync] = useState(false)
  const [saveSyncBusy, setSaveSyncBusy] = useState(false)
  const [saveSyncMessage, setSaveSyncMessage] = useState('')
  const [saveSyncError, setSaveSyncError] = useState('')
  const [baselineLocalHash, setBaselineLocalHash] = useState<string | null>(null)
  const [baselineRemoteManifestHash, setBaselineRemoteManifestHash] = useState<string | null>(null)
  const [runtimeError, setRuntimeError] = useState('')
  const [prepareBusy, setPrepareBusy] = useState(false)
  const [prepareReady, setPrepareReady] = useState(false)
  const [prepareError, setPrepareError] = useState('')
  const [prepareJobId, setPrepareJobId] = useState<string | null>(null)
  const [prepareStatusMessage, setPrepareStatusMessage] = useState('')
  const [prepareProgress, setPrepareProgress] = useState<{ current: number; total: number } | null>(null)
  const [pendingSourceGameId, setPendingSourceGameId] = useState<string | null>(null)
  const recordedRef = useRef<string | null>(null)
  const tokenRef = useRef<string | null>(null)
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const playerShellRef = useRef<HTMLElement | null>(null)
  const pendingBridgeRef = useRef<Map<string, PendingBridgeRequest>>(new Map())
  const [playerFullscreen, setPlayerFullscreen] = useState(false)

  const frontendConfig = useQuery({
    queryKey: ['frontend-config'],
    queryFn: getFrontendConfig,
  })

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: async () => {
      try {
        return await getGame(id)
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          return getGame(id)
        }
        throw error
      }
    },
    enabled: id.length > 0,
  })

  const sourceParam = useMemo(() => {
    const params = new URLSearchParams(location.search)
    const value = params.get('source')
    return value && value.trim().length > 0 ? value : null
  }, [location.search])
  const availableSelections = useMemo(
    () => (game.data ? listBrowserPlaySelections(game.data) : []),
    [game.data],
  )
  const selection = useMemo(
    () => {
      if (!game.data) return null
      const runtime = availableSelections[0]?.runtime ?? null
      const storedSourceId = runtime ? readBrowserPlaySourcePreference(game.data.id, runtime) : null
      return selectBrowserPlaySelection(game.data, sourceParam ?? storedSourceId)
    },
    [availableSelections, game.data, sourceParam],
  )
  const selectionIssue = useMemo(
    () => (game.data ? getBrowserPlaySelectionIssue(game.data, selection) : null),
    [game.data, selection],
  )
  const runtimeLabel = selection ? browserPlayRuntimeLabel(selection.runtime) : null
  const requiresPrepare = selection ? browserPlaySelectionRequiresPrepare(selection) : false
  const selectionReady = selection ? (browserPlaySelectionIsReady(selection) || prepareReady) : false
  const session = useMemo<BrowserPlaySession | null>(
    () => (game.data && selection && selectionReady ? buildBrowserPlaySession(game.data, selection) : null),
    [game.data, selection, selectionReady],
  )
  const playerUrl = useMemo(() => {
    if (!sessionToken || !session) return null
    return buildBrowserPlayerUrl(session.runtime, sessionToken)
  }, [session, sessionToken])
  const hasPendingSourceChange = Boolean(
    selection && pendingSourceGameId && pendingSourceGameId !== selection.sourceGame.id,
  )
  const activeIntegrationId = activeSaveSyncIntegrationId(frontendConfig.data)
  const saveSyncRuntimeSupported = session ? sessionSupportsSaveSync(session) : false
  const saveSyncEnabled = Boolean(activeIntegrationId && session && saveSyncRuntimeSupported)
  const saveSyncSlotsEnabled = saveSyncEnabled && bridgeReady && bridgeSupportsSaveSync

  useEffect(() => {
    setPendingSourceGameId(selection?.sourceGame.id ?? null)
  }, [selection?.sourceGame.id])

  useEffect(() => {
    function syncFullscreenState() {
      setPlayerFullscreen(document.fullscreenElement === playerShellRef.current)
    }

    document.addEventListener('fullscreenchange', syncFullscreenState)
    syncFullscreenState()
    return () => document.removeEventListener('fullscreenchange', syncFullscreenState)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function prepareSelection() {
      if (!game.data || !selection) {
        setPrepareBusy(false)
        setPrepareReady(false)
        setPrepareError('')
        setPrepareJobId(null)
        setPrepareStatusMessage('')
        setPrepareProgress(null)
        return
      }
      if (!browserPlaySelectionRequiresPrepare(selection)) {
        setPrepareBusy(false)
        setPrepareReady(true)
        setPrepareError('')
        setPrepareJobId(null)
        setPrepareStatusMessage('')
        setPrepareProgress(null)
        return
      }

      if (browserPlaySelectionIsReady(selection)) {
        setPrepareBusy(false)
        setPrepareReady(true)
        setPrepareError('')
        setPrepareJobId(null)
        setPrepareStatusMessage('Cached source is ready.')
        setPrepareProgress(null)
        return
      }

      setPrepareBusy(true)
      setPrepareReady(false)
      setPrepareError('')
      setPrepareJobId(null)
      setPrepareStatusMessage('Preparing cached source...')
      setPrepareProgress(null)

      try {
        const result = await prepareGameCache({
          gameId: game.data.id,
          sourceGameId: selection.sourceGame.id,
          profile: selection.profile,
        })
        if (cancelled) return
        if (result.immediate || result.job?.status === 'completed') {
          setPrepareBusy(false)
          setPrepareReady(true)
          setPrepareJobId(result.job?.job_id ?? null)
          setPrepareStatusMessage(result.job?.message ?? 'Cached source is ready.')
          setPrepareProgress(
            result.job
              ? { current: result.job.progress_current ?? 0, total: result.job.progress_total ?? 0 }
              : null,
          )
          return
        }
        const jobId = result.job?.job_id
        if (!jobId) {
          throw new Error('Cache prepare did not return a job id.')
        }
        setPrepareJobId(jobId)
        while (!cancelled) {
          const status = await getCacheJob(jobId)
          if (cancelled) return
          setPrepareStatusMessage(status.message ?? 'Preparing cached source...')
          setPrepareProgress({ current: status.progress_current ?? 0, total: status.progress_total ?? 0 })
          if (status.status === 'completed') {
            setPrepareBusy(false)
            setPrepareReady(true)
            setPrepareStatusMessage(status.message ?? 'Cached source is ready.')
            return
          }
          if (status.status === 'failed') {
            throw new Error(status.error || status.message || 'Cache prepare failed.')
          }
          await new Promise((resolve) => window.setTimeout(resolve, 750))
        }
      } catch (error) {
        if (cancelled) return
        setPrepareBusy(false)
        setPrepareReady(false)
        setPrepareError(error instanceof Error ? error.message : 'Cache prepare failed.')
      }
    }

    void prepareSelection()

    return () => {
      cancelled = true
    }
  }, [game.data, selection])

  useEffect(() => {
    if (!game.data || !selection) return
    writeBrowserPlaySourcePreference(game.data.id, selection.runtime, selection.sourceGame.id)
  }, [game.data, selection])

  useEffect(() => {
    if (!selection || !sourceParam || sourceParam === selection.sourceGame.id) return
    const params = new URLSearchParams(location.search)
    params.set('source', selection.sourceGame.id)
    navigate(
      {
        pathname: location.pathname,
        search: `?${params.toString()}`,
      },
      { replace: true, state: location.state },
    )
  }, [location.pathname, location.search, location.state, navigate, selection, sourceParam])

  const slotsQuery = useQuery({
    queryKey: ['save-sync-slots', id, activeIntegrationId, session?.sourceGameId, session?.runtime],
    queryFn: async () => {
      if (!session || !activeIntegrationId) return []
      return listGameSaveSyncSlots({
        gameId: id,
        integrationId: activeIntegrationId,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
      })
    },
    enabled: saveSyncSlotsEnabled,
    retry: false,
  })

  const currentSlot = useMemo<SaveSyncSlotSummary | null>(() => {
    return slotsQuery.data?.find((slot) => slot.slot_id === selectedSlot) ?? null
  }, [selectedSlot, slotsQuery.data])

  const togglePlayerFullscreen = async () => {
    const playerShell = playerShellRef.current
    if (!playerShell) return

    try {
      if (document.fullscreenElement === playerShell) {
        await document.exitFullscreen()
        return
      }
      if (document.fullscreenElement && document.fullscreenElement !== playerShell) {
        await document.exitFullscreen()
      }
      await playerShell.requestFullscreen()
    } catch (error) {
      console.warn('Unable to toggle browser-player fullscreen.', error)
    }
  }

  useEffect(() => {
    if (tokenRef.current) {
      clearBrowserPlaySession(tokenRef.current)
      tokenRef.current = null
    }

    if (!session) {
      setSessionToken(null)
      return
    }

    const nextToken = persistBrowserPlaySession(session)
    tokenRef.current = nextToken
    setSessionToken(nextToken)
    setBridgeReady(false)
    setBridgeSupportsSaveSync(false)
    setRuntimeError('')
    setSaveSyncMessage('')
    setSaveSyncError('')

    return () => {
      clearBrowserPlaySession(nextToken)
      if (tokenRef.current === nextToken) {
        tokenRef.current = null
      }
    }
  }, [session])

  useEffect(() => {
    if (!game.data || !session || !playerUrl) return
    if (recordedRef.current === playerUrl) return
    recordedRef.current = playerUrl
    recordLaunch({
      gameId: game.data.id,
      title: game.data.title,
      platform: game.data.platform,
      launchKind: 'browser',
      launchUrl: `/game/${encodeURIComponent(game.data.id)}/play?source=${encodeURIComponent(session.sourceGameId)}`,
    })
  }, [game.data, playerUrl, recordLaunch, session])

  useEffect(() => {
    const onMessage = (event: MessageEvent<RuntimeBridgeEvent>) => {
      if (event.source !== iframeRef.current?.contentWindow) return
      const message = event.data
      if (!message || typeof message !== 'object' || !('type' in message)) return

      if (message.type === 'ready') {
        setBridgeReady(true)
        setBridgeSupportsSaveSync(message.saveAdapter === true)
        return
      }

      if (message.type === 'runtime-error') {
        setRuntimeError(message.error)
        return
      }

      if (message.type !== 'export-result' && message.type !== 'import-result') {
        return
      }

      const pending = pendingBridgeRef.current.get(message.requestId)
      if (!pending) return
      pendingBridgeRef.current.delete(message.requestId)

      if (message.type === 'export-result') {
        if (message.error || !message.snapshot) {
          pending.reject(new Error(message.error || 'Runtime export failed.'))
        } else {
          pending.resolve(message.snapshot)
        }
        return
      }

      if (!message.ok) {
        pending.reject(new Error(message.error || 'Runtime import failed.'))
      } else {
        pending.resolve()
      }
    }

    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [])

  const sendBridgeCommand = (command: RuntimeBridgeCommand) => {
    const target = iframeRef.current?.contentWindow
    if (!target) {
      throw new Error('Player frame is not available.')
    }
    target.postMessage(command, window.location.origin)
  }

  const exportRuntimeSnapshot = async (): Promise<RuntimeSaveSnapshot> => {
    if (!bridgeReady || !bridgeSupportsSaveSync) {
      throw new Error('This runtime is not ready for save sync yet.')
    }
    const requestId =
      typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`

    const promise = new Promise<RuntimeSaveSnapshot>((resolve, reject) => {
      pendingBridgeRef.current.set(requestId, { resolve, reject, kind: 'export' })
    })
    sendBridgeCommand({ type: 'export-save-snapshot', requestId })
    return promise
  }

  const importRuntimeSnapshot = async (snapshot: RuntimeSaveSnapshot) => {
    if (!bridgeReady || !bridgeSupportsSaveSync) {
      throw new Error('This runtime is not ready for save sync yet.')
    }
    const requestId =
      typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`

    const promise = new Promise<void>((resolve, reject) => {
      pendingBridgeRef.current.set(requestId, { resolve, reject, kind: 'import' })
    })
    sendBridgeCommand({ type: 'import-save-snapshot', requestId, files: snapshot.files })
    return promise
  }

  useEffect(() => {
    setBaselineRemoteManifestHash(currentSlot?.manifest_hash ?? null)
  }, [currentSlot?.manifest_hash])

  const handleBack = () => {
    navigate(`/game/${encodeURIComponent(id)}`, { state: location.state })
  }

  const handleSourceChange = (sourceGameId: string) => {
    setPendingSourceGameId(sourceGameId)
  }

  const handleSourceApply = () => {
    if (!game.data || !selection || !pendingSourceGameId || pendingSourceGameId === selection.sourceGame.id) return
    writeBrowserPlaySourcePreference(game.data.id, selection.runtime, pendingSourceGameId)
    const params = new URLSearchParams(location.search)
    params.set('source', pendingSourceGameId)
    navigate(
      {
        pathname: location.pathname,
        search: `?${params.toString()}`,
      },
      { state: location.state },
    )
  }

  const handleSourceReset = () => {
    setPendingSourceGameId(selection?.sourceGame.id ?? null)
  }

  const handleSave = async () => {
    if (!session || !activeIntegrationId) return
    setSaveSyncBusy(true)
    setSaveSyncError('')
    setSaveSyncMessage('')
    try {
      const local = await exportRuntimeSnapshot()
      const snapshot = await buildSaveSyncSnapshot({
        canonicalGameId: id,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        slotId: selectedSlot,
        files: local.files,
      })

      let result = await putGameSaveSyncSlot({
        gameId: id,
        slotId: selectedSlot,
        integrationId: activeIntegrationId,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        baseManifestHash: baselineRemoteManifestHash ?? undefined,
        snapshot,
      })

      if (result.conflict) {
        const confirmed = window.confirm(
          `Remote ${selectedSlot} changed on ${new Date(result.conflict.remote_updated_at).toLocaleString()}. Overwrite it with local data?`,
        )
        if (!confirmed) {
          setSaveSyncMessage('Save canceled.')
          return
        }
        result = await putGameSaveSyncSlot({
          gameId: id,
          slotId: selectedSlot,
          integrationId: activeIntegrationId,
          sourceGameId: session.sourceGameId,
          runtime: session.runtime,
          force: true,
          snapshot,
        })
      }

      if (!result.ok) {
        throw new Error(result.conflict?.message || 'Save sync failed.')
      }

      setBaselineRemoteManifestHash(result.summary.manifest_hash ?? null)
      setBaselineLocalHash(await computeLocalSnapshotHash(local.files))
      setSaveSyncMessage(`Saved ${selectedSlot} to the active integration.`)
      await slotsQuery.refetch()
    } catch (error) {
      setSaveSyncError(error instanceof Error ? error.message : 'Save failed.')
    } finally {
      setSaveSyncBusy(false)
    }
  }

  const handleLoad = async () => {
    if (!session || !activeIntegrationId || !currentSlot?.exists) return
    setSaveSyncBusy(true)
    setSaveSyncError('')
    setSaveSyncMessage('')
    try {
      const local = await exportRuntimeSnapshot()
      const localHash = await computeLocalSnapshotHash(local.files)
      if (baselineLocalHash && localHash !== baselineLocalHash) {
        const confirmed = window.confirm(
          `Local save files changed since the last save or load. Replace them with remote ${selectedSlot}?`,
        )
        if (!confirmed) {
          setSaveSyncMessage('Load canceled.')
          return
        }
      }

      const remote = await getGameSaveSyncSlot({
        gameId: id,
        integrationId: activeIntegrationId,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        slotId: selectedSlot,
      })
      const files = extractRuntimeFilesFromSnapshot(remote)
      await importRuntimeSnapshot({ files })
      setBaselineLocalHash(await computeLocalSnapshotHash(files))
      setBaselineRemoteManifestHash(remote.manifest_hash ?? null)
      setSaveSyncMessage(`Loaded ${selectedSlot} from the active integration.`)
      await slotsQuery.refetch()
    } catch (error) {
      setSaveSyncError(error instanceof Error ? error.message : 'Load failed.')
    } finally {
      setSaveSyncBusy(false)
    }
  }

  if (game.isPending) {
    return (
      <div className="min-h-screen bg-mga-bg text-mga-text">
        <div className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 p-4 md:p-6">
          <Button variant="outline" size="sm" onClick={handleBack} className="w-fit">
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          <div className="rounded-mga border border-mga-border bg-mga-surface p-6">
            <div className="space-y-4">
              <Skeleton className="h-6 w-40" />
              <Skeleton className="h-4 w-72" />
              <Skeleton className="h-[70vh] w-full rounded-[1.25rem]" />
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (game.isError || !game.data) {
    return (
      <div className="min-h-screen bg-mga-bg text-mga-text">
        <div className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 p-4 md:p-6">
          <Button variant="outline" size="sm" onClick={handleBack} className="w-fit">
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300">
            {game.isError ? game.error.message : 'Game not found.'}
          </div>
        </div>
      </div>
    )
  }

  const data = game.data

  return (
    <div className="h-screen overflow-hidden bg-mga-bg text-mga-text">
      <div className="mx-auto flex h-full min-h-0 max-w-[1600px] flex-col gap-4 overflow-y-auto p-4 md:p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button variant="outline" size="sm" onClick={handleBack}>
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          {data.xcloud_url && (
            <a href={data.xcloud_url} target="_blank" rel="noreferrer">
              <Button variant="outline" size="sm">
                <ExternalLink size={14} />
                Play on xCloud
              </Button>
            </a>
          )}
        </div>

        <section className="rounded-mga border border-mga-border bg-mga-surface p-4 md:p-5">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="platform">
              <PlatformIcon platform={data.platform} showLabel />
            </Badge>
            {runtimeLabel && <Badge variant="playable">{runtimeLabel}</Badge>}
            {data.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" />}
          </div>
          <div className="mt-3">
            <h1 className="text-2xl font-semibold tracking-tight md:text-3xl">{data.title}</h1>
            <p className="mt-2 text-sm text-mga-muted">
              Dedicated browser player route for fullscreen play, runtime lifecycle, and explicit save sync.
            </p>
            {selection && (
              <p className="mt-2 text-xs text-mga-muted">
                Source: {browserPlaySourceLabel(selection)}
                {browserPlaySourceContext(selection) ? ` · ${browserPlaySourceContext(selection)}` : ''}
              </p>
            )}
            {availableSelections.length > 1 && selection && (
              <div className="mt-4 max-w-xl">
                <label className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">Source</label>
                <select
                  value={pendingSourceGameId ?? selection.sourceGame.id}
                  onChange={(event) => handleSourceChange(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                >
                  {availableSelections.map((option) => (
                    <option key={option.sourceGame.id} value={option.sourceGame.id}>
                      {browserPlaySourceOptionLabel(option)}
                    </option>
                  ))}
                </select>
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleSourceApply}
                    disabled={!hasPendingSourceChange}
                  >
                    Apply Source
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleSourceReset}
                    disabled={!hasPendingSourceChange}
                  >
                    Reset
                  </Button>
                  <span className="text-xs text-mga-muted">
                    Applying a different source restarts the runtime.
                  </span>
                </div>
              </div>
            )}
          </div>
        </section>

        {!hasBrowserPlaySupport(data) ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            {selectionIssue?.message ?? 'This platform is not part of the supported browser-play set for Phase 6.'}
          </section>
        ) : !selection ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            {selectionIssue?.message ?? 'Browser Play is supported for this platform, but no launchable source file was found for this game yet.'}
          </section>
        ) : requiresPrepare && !selectionReady ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            <p className="font-medium text-mga-text">{prepareBusy ? 'Preparing cached source...' : 'Cached source is not ready yet.'}</p>
            <p className="mt-2">{prepareStatusMessage || 'Preparing remote source files before launch.'}</p>
            {prepareProgress && prepareProgress.total > 0 && (
              <p className="mt-2 text-xs text-mga-muted">
                {prepareProgress.current}/{prepareProgress.total} files prepared
              </p>
            )}
            {prepareJobId && <p className="mt-2 text-xs text-mga-muted">Job: {prepareJobId}</p>}
            {prepareError && <p className="mt-3 text-xs text-red-400">{prepareError}</p>}
          </section>
        ) : !session || !playerUrl ? (
          <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-200">
            {selectionIssue?.message ?? 'Failed to assemble a browser-play launch session for this game.'}
          </section>
        ) : (
          <>
            <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <div className="flex flex-wrap items-center gap-3">
                <div className="min-w-[12rem]">
                  <label className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">Save Slot</label>
                  <select
                    value={selectedSlot}
                    onChange={(event) => setSelectedSlot(event.target.value as (typeof SAVE_SYNC_SLOT_IDS)[number])}
                    className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                  >
                    {SAVE_SYNC_SLOT_IDS.map((slot) => (
                      <option key={slot} value={slot}>{slot}</option>
                    ))}
                  </select>
                </div>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleLoad}
                  disabled={!saveSyncEnabled || !currentSlot?.exists || saveSyncBusy || !bridgeReady || !bridgeSupportsSaveSync}
                >
                  <Download size={14} />
                  {saveSyncBusy ? 'Working...' : 'Load'}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleSave}
                  disabled={!saveSyncEnabled || saveSyncBusy || !bridgeReady || !bridgeSupportsSaveSync}
                >
                  <Upload size={14} />
                  {saveSyncBusy ? 'Working...' : 'Save'}
                </Button>

                <div className="min-w-[14rem] text-xs text-mga-muted">
                  {!activeIntegrationId && 'Choose an active Save Sync integration in Settings to enable remote saves.'}
                  {activeIntegrationId && !bridgeReady && 'Waiting for runtime bridge...'}
                  {activeIntegrationId && bridgeReady && !bridgeSupportsSaveSync && 'This runtime page does not support save import/export yet.'}
                  {activeIntegrationId && bridgeReady && bridgeSupportsSaveSync && currentSlot?.exists && (
                    <>
                      Remote {selectedSlot}: {currentSlot.file_count ?? 0} files, {currentSlot.total_size ?? 0} bytes
                      {currentSlot.updated_at ? `, updated ${new Date(currentSlot.updated_at).toLocaleString()}` : ''}
                    </>
                  )}
                  {activeIntegrationId && bridgeReady && bridgeSupportsSaveSync && currentSlot && !currentSlot.exists && (
                    <>Remote {selectedSlot} is empty.</>
                  )}
                </div>
              </div>

              {slotsQuery.isLoading && (
                <p className="mt-3 text-xs text-mga-muted">Loading save slot metadata...</p>
              )}
              {saveSyncMessage && <p className="mt-3 text-xs text-green-400">{saveSyncMessage}</p>}
              {saveSyncError && <p className="mt-3 text-xs text-red-400">{saveSyncError}</p>}
              {runtimeError && <p className="mt-3 text-xs text-red-400">{runtimeError}</p>}
            </section>

            <section
              ref={playerShellRef}
              className="flex min-h-[70vh] min-w-0 flex-1 flex-col overflow-hidden rounded-[1.25rem] border border-mga-border bg-black shadow-lg shadow-black/25"
            >
              <div className="flex items-center justify-between border-b border-white/10 bg-black/80 px-4 py-3 text-sm text-white/80">
                <div className="flex items-center gap-2">
                  <PlayCircle size={16} />
                  <span>{data.title}</span>
                </div>
                <div className="flex items-center gap-2">
                  {session.runtime === 'scummvm' && (
                    <Button
                      variant="ghost"
                      size="icon"
                      type="button"
                      onClick={togglePlayerFullscreen}
                      aria-label={playerFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
                      title={playerFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
                      className="h-8 w-8 rounded-full text-white/60 hover:bg-white/10 hover:text-white"
                    >
                      {playerFullscreen ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
                    </Button>
                  )}
                  <span className="text-xs uppercase tracking-wide text-white/50">{runtimeLabel}</span>
                </div>
              </div>
              <div className="relative min-h-0 min-w-0 flex-1">
                <BrowserRuntimeFrame
                  runtime={session.runtime}
                  iframeRef={iframeRef}
                  src={playerUrl}
                  title={`${data.title} browser player`}
                />
              </div>
            </section>
          </>
        )}

        {hasBrowserPlaySupport(data) && data.xcloud_url && (
          <p className="text-xs text-mga-muted">
            xCloud stays external in Phase 6. Browser Play and xCloud are separate launch paths.
          </p>
        )}
      </div>
    </div>
  )
}
