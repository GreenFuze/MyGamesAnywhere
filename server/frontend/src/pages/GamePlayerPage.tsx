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
  getSaveSyncPrefetchStatus,
  listGameSaveSyncSlots,
  prepareGameCache,
  putGameSaveSyncSlot,
  startGameSaveSyncPrefetch,
  type SaveSyncSlotSummary,
} from '@/api/client'
import { BrandBadge, BrandIcon } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import {
  browserPlayJsdosExecutableLabel,
  clearBrowserPlaySourcePreference,
  browserPlaySourceOptionLabel,
  browserPlayRuntimeLabel,
  browserPlaySelectionIsReady,
  browserPlaySelectionRequiresPrepare,
  buildBrowserPlaySession,
  buildBrowserPlayerUrl,
  clearBrowserPlaySession,
  getBrowserPlayPreferenceRuntime,
  listBrowserPlayJsdosExecutables,
  persistBrowserPlaySession,
  readBrowserPlayJsdosExecutablePreference,
  readBrowserPlaySourcePreference,
  resolveBrowserPlaySelection,
  sessionSupportsSaveSync,
  writeBrowserPlayJsdosExecutablePreference,
  writeBrowserPlaySourcePreference,
  type BrowserPlaySession,
} from '@/lib/browserPlay'
import { hasBrowserPlaySupport } from '@/lib/gameUtils'
import {
  EMULATORJS_SAVE_RAM_SLOT_ID,
  SAVE_SYNC_SLOT_IDS,
  buildSaveSyncSnapshot,
  computeLocalSnapshotHash,
  emulatorJsStateSlotId,
  extractRuntimeFilesFromSnapshot,
  type RuntimeBridgeCommand,
  type RuntimeBridgeEvent,
  type RuntimeSaveFile,
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

function browserPlayRuntimeBrand(runtime: BrowserPlaySession['runtime']): string {
  switch (runtime) {
    case 'emulatorjs':
      return 'emulatorjs'
    case 'jsdos':
      return 'js-dos'
    case 'scummvm':
      return 'scummvm'
  }
}

type PendingBridgeRequest = {
  resolve: (value?: any) => void
  reject: (reason?: unknown) => void
  kind: 'export' | 'import'
}

type NativeBridgeReply =
  | { type: 'native-save-result'; requestId: string; ok: boolean; error?: string }
  | { type: 'native-load-state-result'; requestId: string; ok: boolean; stateBase64?: string; error?: string }
  | { type: 'native-load-ram-result'; requestId: string; ok: boolean; files?: RuntimeSaveFile[]; error?: string }

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
  const [nativeSaveSyncPrefetchBusy, setNativeSaveSyncPrefetchBusy] = useState(false)
  const [nativeSaveSyncPrefetchReady, setNativeSaveSyncPrefetchReady] = useState(false)
  const [nativeSaveSyncPrefetchSucceeded, setNativeSaveSyncPrefetchSucceeded] = useState(false)
  const [nativeSaveSyncPrefetchMessage, setNativeSaveSyncPrefetchMessage] = useState('')
  const [nativeSaveSyncPrefetchError, setNativeSaveSyncPrefetchError] = useState('')
  const [nativeSaveSyncPrefetchProgress, setNativeSaveSyncPrefetchProgress] = useState<{ current: number; total: number } | null>(null)
  const [pendingSourceGameId, setPendingSourceGameId] = useState<string | null>(null)
  const [appliedJsdosExecutablePath, setAppliedJsdosExecutablePath] = useState<string | null>(null)
  const [pendingJsdosExecutablePath, setPendingJsdosExecutablePath] = useState<string | null>(null)
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
  const browserPlayResolution = useMemo(() => {
    if (!game.data) return null
    const runtime = getBrowserPlayPreferenceRuntime(game.data)
    const rememberedSourceId =
      sourceParam || !runtime ? null : readBrowserPlaySourcePreference(game.data.id, runtime)
    return resolveBrowserPlaySelection(game.data, {
      requestedSourceGameId: sourceParam,
      rememberedSourceGameId: rememberedSourceId,
    })
  }, [game.data, sourceParam])
  const availableSelections = browserPlayResolution?.selections ?? []
  const selection = browserPlayResolution?.selection ?? null
  const selectionIssue = browserPlayResolution?.issue ?? null
  const runtimeLabel = browserPlayResolution?.runtime
    ? browserPlayRuntimeLabel(browserPlayResolution.runtime)
    : null
  const runtimeBrand = browserPlayResolution?.runtime
    ? browserPlayRuntimeBrand(browserPlayResolution.runtime)
    : null
  const jsdosExecutableOptions = useMemo(
    () => (selection?.runtime === 'jsdos' ? listBrowserPlayJsdosExecutables(selection.sourceGame.files) : []),
    [selection],
  )
  const requiresPrepare = selection ? browserPlaySelectionRequiresPrepare(selection) : false
  const selectionReady = selection ? (browserPlaySelectionIsReady(selection) || prepareReady) : false
  const baseSession = useMemo<BrowserPlaySession | null>(
    () =>
      game.data && selection && selectionReady
        ? buildBrowserPlaySession(game.data, selection, {
            jsdosExecutablePath: appliedJsdosExecutablePath,
          })
        : null,
    [appliedJsdosExecutablePath, game.data, selection, selectionReady],
  )
  const activeIntegrationId = activeSaveSyncIntegrationId(frontendConfig.data)
  const baseSaveSyncRuntimeSupported = baseSession ? sessionSupportsSaveSync(baseSession) : false
  const baseSaveSyncEnabled = Boolean(activeIntegrationId && baseSession && baseSaveSyncRuntimeSupported)
  const slotsQuery = useQuery({
    queryKey: ['save-sync-slots', id, activeIntegrationId, baseSession?.sourceGameId, baseSession?.runtime],
    queryFn: async () => {
      if (!baseSession || !activeIntegrationId) return []
      return listGameSaveSyncSlots({
        gameId: id,
        integrationId: activeIntegrationId,
        sourceGameId: baseSession.sourceGameId,
        runtime: baseSession.runtime,
      })
    },
    enabled: baseSaveSyncEnabled && baseSession?.runtime !== 'emulatorjs',
    retry: false,
  })
  const nativeSaveSyncPrefetchRequired = Boolean(baseSession?.runtime === 'emulatorjs' && baseSaveSyncEnabled)
  const nativeSaveSyncPending = Boolean(nativeSaveSyncPrefetchRequired && !nativeSaveSyncPrefetchReady)
  const nativeSaveSyncAvailable = Boolean(nativeSaveSyncPrefetchRequired && nativeSaveSyncPrefetchSucceeded)
  const session = useMemo<BrowserPlaySession | null>(() => {
    if (!baseSession) return null
    if (baseSession.runtime !== 'emulatorjs') return baseSession
    if (nativeSaveSyncPending) return null
    return {
      ...baseSession,
      nativeSaveSync: nativeSaveSyncAvailable,
    }
  }, [baseSession, nativeSaveSyncAvailable, nativeSaveSyncPending])
  const playerUrl = useMemo(() => {
    if (!sessionToken || !session) return null
    return buildBrowserPlayerUrl(session.runtime, sessionToken)
  }, [session, sessionToken])
  const hasPendingSourceChange = Boolean(
    pendingSourceGameId && pendingSourceGameId !== (selection?.sourceGame.id ?? null),
  )
  const hasPendingJsdosExecutableChange = Boolean(
    selection?.runtime === 'jsdos' &&
      (!session || (session.runtime === 'jsdos' && !session.bundleUrl)) &&
      appliedJsdosExecutablePath &&
      pendingJsdosExecutablePath &&
      pendingJsdosExecutablePath !== appliedJsdosExecutablePath,
  )
  const saveSyncRuntimeSupported = session ? sessionSupportsSaveSync(session) : false
  const saveSyncEnabled = Boolean(activeIntegrationId && session && saveSyncRuntimeSupported)
  const usesSingleSaveSyncSnapshot = session?.runtime === 'jsdos'
  const effectiveSelectedSlot = usesSingleSaveSyncSnapshot ? 'autosave' : selectedSlot
  const saveSyncSnapshotLabel = usesSingleSaveSyncSnapshot ? 'DOS save snapshot' : selectedSlot

  useEffect(() => {
    setPendingSourceGameId(selection?.sourceGame.id ?? null)
  }, [selection?.sourceGame.id])

  useEffect(() => {
    if (!game.data || !browserPlayResolution?.runtime || !browserPlayResolution.invalidRememberedSourceGameId) {
      return
    }
    clearBrowserPlaySourcePreference(game.data.id, browserPlayResolution.runtime)
  }, [browserPlayResolution, game.data])

  useEffect(() => {
    if (!game.data || !selection || selection.runtime !== 'jsdos') {
      setAppliedJsdosExecutablePath(null)
      setPendingJsdosExecutablePath(null)
      return
    }

    const storedExecutablePath = readBrowserPlayJsdosExecutablePreference(game.data.id, selection.sourceGame.id)
    const availableExecutablePaths = listBrowserPlayJsdosExecutables(selection.sourceGame.files)
    const normalizedExecutablePaths = new Set(availableExecutablePaths)
    const preferredExecutablePath =
      storedExecutablePath && normalizedExecutablePaths.has(storedExecutablePath) ? storedExecutablePath : null
    const nextExecutablePath =
      preferredExecutablePath ??
      (availableExecutablePaths.includes(selection.rootFile?.path ?? '')
        ? (selection.rootFile?.path ?? null)
        : (availableExecutablePaths[0] ?? selection.rootFile?.path ?? null))

    setAppliedJsdosExecutablePath(nextExecutablePath)
    setPendingJsdosExecutablePath(nextExecutablePath)
  }, [game.data, selection])

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
    let cancelled = false

    async function prefetchNativeSaveSync() {
      if (!baseSession || baseSession.runtime !== 'emulatorjs' || !activeIntegrationId || !baseSaveSyncEnabled) {
        setNativeSaveSyncPrefetchBusy(false)
        setNativeSaveSyncPrefetchReady(false)
        setNativeSaveSyncPrefetchSucceeded(false)
        setNativeSaveSyncPrefetchMessage('')
        setNativeSaveSyncPrefetchError('')
        setNativeSaveSyncPrefetchProgress(null)
        return
      }

      setNativeSaveSyncPrefetchBusy(true)
      setNativeSaveSyncPrefetchReady(false)
      setNativeSaveSyncPrefetchSucceeded(false)
      setNativeSaveSyncPrefetchMessage('Prefetching save slots...')
      setNativeSaveSyncPrefetchError('')
      setNativeSaveSyncPrefetchProgress(null)

      try {
        const started = await startGameSaveSyncPrefetch({
          gameId: id,
          integrationId: activeIntegrationId,
          sourceGameId: baseSession.sourceGameId,
          runtime: baseSession.runtime,
        })
        if (cancelled) return
        setNativeSaveSyncPrefetchProgress({
          current: started.progress_current ?? 0,
          total: started.progress_total ?? 0,
        })

        let status = started
        while (!cancelled && status.status !== 'completed' && status.status !== 'failed') {
          await new Promise((resolve) => window.setTimeout(resolve, 500))
          status = await getSaveSyncPrefetchStatus(started.job_id)
          if (cancelled) return
          setNativeSaveSyncPrefetchMessage(status.message || 'Prefetching save slots...')
          setNativeSaveSyncPrefetchProgress({
            current: status.progress_current ?? 0,
            total: status.progress_total ?? 0,
          })
        }
        if (cancelled) return
        setNativeSaveSyncPrefetchBusy(false)
        setNativeSaveSyncPrefetchReady(true)
        if (status.status === 'completed') {
          setNativeSaveSyncPrefetchSucceeded(true)
          setNativeSaveSyncPrefetchMessage('Save slots prefetched.')
          return
        }
        setNativeSaveSyncPrefetchSucceeded(false)
        setNativeSaveSyncPrefetchError(
          status.error || status.message || 'MGA save-sync prefetch failed. Native save-sync is disabled for this session.',
        )
      } catch (error) {
        if (cancelled) return
        setNativeSaveSyncPrefetchBusy(false)
        setNativeSaveSyncPrefetchReady(true)
        setNativeSaveSyncPrefetchSucceeded(false)
        setNativeSaveSyncPrefetchError(
          error instanceof Error
            ? `MGA save-sync prefetch failed: ${error.message}`
            : 'MGA save-sync prefetch failed. Native save-sync is disabled for this session.',
        )
      }
    }

    void prefetchNativeSaveSync()

    return () => {
      cancelled = true
    }
  }, [activeIntegrationId, baseSaveSyncEnabled, baseSession, id])

  const currentSlot = useMemo<SaveSyncSlotSummary | null>(() => {
    return slotsQuery.data?.find((slot) => slot.slot_id === effectiveSelectedSlot) ?? null
  }, [effectiveSelectedSlot, slotsQuery.data])

  const togglePlayerFullscreen = async () => {
    const playerShell = playerShellRef.current
    if (!playerShell) return

    try {
      if (document.fullscreenElement === playerShell) {
        await document.exitFullscreen()
        setPlayerFullscreen(false)
        return
      }
      if (document.fullscreenElement && document.fullscreenElement !== playerShell) {
        await document.exitFullscreen()
      }
      await playerShell.requestFullscreen()
      setPlayerFullscreen(true)
    } catch (error) {
      console.warn('Unable to toggle browser-player fullscreen.', error)
    }
  }

  useEffect(() => {
    setPlayerFullscreen(document.fullscreenElement === playerShellRef.current)
  }, [playerUrl])

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
    const postNativeReply = (reply: NativeBridgeReply) => {
      iframeRef.current?.contentWindow?.postMessage(reply, window.location.origin)
    }

    const saveNativeFiles = async (slotId: string, files: RuntimeSaveFile[]) => {
      if (!session || !activeIntegrationId) {
        throw new Error('Save sync is not available for this runtime session.')
      }
      const snapshot = await buildSaveSyncSnapshot({
        canonicalGameId: id,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        slotId,
        files,
      })
      const result = await putGameSaveSyncSlot({
        gameId: id,
        slotId,
        integrationId: activeIntegrationId,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        force: true,
        snapshot,
      })
      if (!result.ok) {
        throw new Error(result.conflict?.message || 'Save sync failed.')
      }
    }

    const loadNativeFiles = async (slotId: string): Promise<RuntimeSaveFile[] | null> => {
      if (!session || !activeIntegrationId) {
        throw new Error('Save sync is not available for this runtime session.')
      }
      try {
        const remote = await getGameSaveSyncSlot({
          gameId: id,
          integrationId: activeIntegrationId,
          sourceGameId: session.sourceGameId,
          runtime: session.runtime,
          slotId,
        })
        return extractRuntimeFilesFromSnapshot(remote)
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          return null
        }
        throw error
      }
    }

    const handleNativeBridgeMessage = async (message: RuntimeBridgeEvent) => {
      if (
        message.type !== 'native-save-state' &&
        message.type !== 'native-load-state' &&
        message.type !== 'native-save-ram' &&
        message.type !== 'native-load-ram'
      ) {
        return
      }

      if (!session || session.runtime !== 'emulatorjs' || !session.nativeSaveSync) {
        postNativeReply({
          type:
            message.type === 'native-load-state'
              ? 'native-load-state-result'
              : message.type === 'native-load-ram'
                ? 'native-load-ram-result'
                : 'native-save-result',
          requestId: message.requestId,
          ok: false,
          error: 'Native save sync is not enabled for this EmulatorJS session.',
        })
        return
      }

      try {
        if (message.type === 'native-save-state') {
          const slotId = emulatorJsStateSlotId(message.slot)
          await saveNativeFiles(slotId, [{ path: 'state.state', base64: message.stateBase64 }])
          postNativeReply({ type: 'native-save-result', requestId: message.requestId, ok: true })
          return
        }
        if (message.type === 'native-load-state') {
          const slotId = emulatorJsStateSlotId(message.slot)
          const files = await loadNativeFiles(slotId)
          postNativeReply({
            type: 'native-load-state-result',
            requestId: message.requestId,
            ok: true,
            stateBase64: files?.[0]?.base64,
          })
          return
        }
        if (message.type === 'native-save-ram') {
          const files =
            message.files && message.files.length > 0
              ? message.files
              : message.saveBase64
                ? [{ path: message.savePath || 'save.ram', base64: message.saveBase64 }]
                : []
          if (files.length === 0) {
            throw new Error('No save RAM data to sync.')
          }
          await saveNativeFiles(EMULATORJS_SAVE_RAM_SLOT_ID, files)
          postNativeReply({ type: 'native-save-result', requestId: message.requestId, ok: true })
          return
        }
        if (message.type === 'native-load-ram') {
          const files = await loadNativeFiles(EMULATORJS_SAVE_RAM_SLOT_ID)
          postNativeReply({
            type: 'native-load-ram-result',
            requestId: message.requestId,
            ok: true,
            files: files ?? [],
          })
        }
      } catch (error) {
        postNativeReply({
          type:
            message.type === 'native-load-state'
              ? 'native-load-state-result'
              : message.type === 'native-load-ram'
                ? 'native-load-ram-result'
                : 'native-save-result',
          requestId: message.requestId,
          ok: false,
          error: error instanceof Error ? error.message : 'Native save sync failed.',
        })
      }
    }

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

      if (
        message.type === 'native-save-state' ||
        message.type === 'native-load-state' ||
        message.type === 'native-save-ram' ||
        message.type === 'native-load-ram'
      ) {
        void handleNativeBridgeMessage(message)
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
  }, [activeIntegrationId, id, session])

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
    if (
      !game.data ||
      !browserPlayResolution?.runtime ||
      !pendingSourceGameId ||
      pendingSourceGameId === selection?.sourceGame.id
    ) {
      return
    }
    writeBrowserPlaySourcePreference(game.data.id, browserPlayResolution.runtime, pendingSourceGameId)
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

  const handleJsdosExecutableChange = (executablePath: string) => {
    setPendingJsdosExecutablePath(executablePath)
  }

  const handleJsdosExecutableApply = () => {
    if (
      !game.data ||
      !selection ||
      selection.runtime !== 'jsdos' ||
      !pendingJsdosExecutablePath ||
      pendingJsdosExecutablePath === appliedJsdosExecutablePath
    ) {
      return
    }

    const confirmed = window.confirm(
      'Changing the DOS executable will restart the emulator. Continue?',
    )
    if (!confirmed) return

    writeBrowserPlayJsdosExecutablePreference(
      game.data.id,
      selection.sourceGame.id,
      pendingJsdosExecutablePath,
    )
    setAppliedJsdosExecutablePath(pendingJsdosExecutablePath)
  }

  const handleJsdosExecutableReset = () => {
    setPendingJsdosExecutablePath(appliedJsdosExecutablePath)
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
        slotId: effectiveSelectedSlot,
        files: local.files,
      })

      let result = await putGameSaveSyncSlot({
        gameId: id,
        slotId: effectiveSelectedSlot,
        integrationId: activeIntegrationId,
        sourceGameId: session.sourceGameId,
        runtime: session.runtime,
        baseManifestHash: baselineRemoteManifestHash ?? undefined,
        snapshot,
      })

      if (result.conflict) {
        const confirmed = window.confirm(
          `Remote ${saveSyncSnapshotLabel} changed on ${new Date(result.conflict.remote_updated_at).toLocaleString()}. Overwrite it with local data?`,
        )
        if (!confirmed) {
          setSaveSyncMessage('Save canceled.')
          return
        }
        result = await putGameSaveSyncSlot({
          gameId: id,
          slotId: effectiveSelectedSlot,
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
      setSaveSyncMessage(`Saved ${saveSyncSnapshotLabel} to the active integration.`)
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
          `Local save files changed since the last save or load. Replace them with remote ${saveSyncSnapshotLabel}?`,
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
        slotId: effectiveSelectedSlot,
      })
      const files = extractRuntimeFilesFromSnapshot(remote)
      await importRuntimeSnapshot({ files })
      setBaselineLocalHash(await computeLocalSnapshotHash(files))
      setBaselineRemoteManifestHash(remote.manifest_hash ?? null)
      setSaveSyncMessage(`Loaded ${saveSyncSnapshotLabel} from the active integration.`)
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
  const showExternalSaveSyncBar = Boolean(session && session.runtime !== 'emulatorjs' && saveSyncRuntimeSupported)
  const playerWindowTitle =
    selection && availableSelections.length > 0
      ? `${data.title} · ${browserPlaySourceOptionLabel(selection, availableSelections)}`
      : data.title

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
            {runtimeBrand && runtimeLabel && (
              <Badge variant="playable" title={runtimeLabel} aria-label={runtimeLabel}>
                <BrandIcon brand={runtimeBrand} className="h-4 w-4" />
              </Badge>
            )}
            {data.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" />}
          </div>
          <div className="mt-3">
            <h1 className="text-2xl font-semibold tracking-tight md:text-3xl">{data.title}</h1>
            {(availableSelections.length > 1 || selectionIssue?.code === 'invalid_remembered_source') && (
              <div className="mt-4 max-w-xl">
                <label className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">Source</label>
                <select
                  value={pendingSourceGameId ?? selection?.sourceGame.id ?? ''}
                  onChange={(event) => handleSourceChange(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                >
                  {!selection && (
                    <option value="" disabled>
                      Choose a source to start browser play
                    </option>
                  )}
                  {availableSelections.map((option) => (
                    <option key={option.sourceGame.id} value={option.sourceGame.id}>
                      {browserPlaySourceOptionLabel(option, availableSelections)}
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
                    {selection
                      ? 'Applying a different source restarts the runtime.'
                      : 'Choose a source, then apply it to continue.'}
                  </span>
                </div>
              </div>
            )}
            {selection &&
              selection.runtime === 'jsdos' &&
              (!session || (session.runtime === 'jsdos' && !session.bundleUrl)) &&
              jsdosExecutableOptions.length > 0 && (
              <div className="mt-4 max-w-xl">
                <label className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">
                  DOS Executable
                </label>
                <select
                  value={pendingJsdosExecutablePath ?? appliedJsdosExecutablePath ?? ''}
                  onChange={(event) => handleJsdosExecutableChange(event.target.value)}
                  className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text"
                >
                  {jsdosExecutableOptions.map((option) => (
                    <option key={option} value={option}>
                      {browserPlayJsdosExecutableLabel(option, selection.sourceGame.files)}
                    </option>
                  ))}
                </select>
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleJsdosExecutableApply}
                    disabled={!hasPendingJsdosExecutableChange}
                  >
                    Apply Executable
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleJsdosExecutableReset}
                    disabled={!hasPendingJsdosExecutableChange}
                  >
                    Reset
                  </Button>
                  <span className="text-xs text-mga-muted">
                    Applying a different executable restarts the runtime.
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
        ) : nativeSaveSyncPending ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="font-medium text-mga-text">
                  {nativeSaveSyncPrefetchBusy ? 'Prefetching save slots...' : 'Preparing save sync...'}
                </p>
                <p className="mt-2">
                  {nativeSaveSyncPrefetchMessage || 'Loading cached saves before starting EmulatorJS.'}
                </p>
              </div>
              {nativeSaveSyncPrefetchProgress && nativeSaveSyncPrefetchProgress.total > 0 && (
                <span className="text-xs text-mga-muted">
                  {nativeSaveSyncPrefetchProgress.current}/{nativeSaveSyncPrefetchProgress.total}
                </span>
              )}
            </div>
            {nativeSaveSyncPrefetchProgress && nativeSaveSyncPrefetchProgress.total > 0 && (
              <div className="mt-4 h-2 overflow-hidden rounded-full bg-mga-bg">
                <div
                  className="h-full rounded-full bg-mga-accent transition-all"
                  style={{
                    width: `${Math.min(
                      100,
                      Math.max(
                        0,
                        (nativeSaveSyncPrefetchProgress.current / nativeSaveSyncPrefetchProgress.total) * 100,
                      ),
                    )}%`,
                  }}
                />
              </div>
            )}
          </section>
        ) : !session || !playerUrl ? (
          <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-200">
            {selectionIssue?.message ?? 'Failed to assemble a browser-play launch session for this game.'}
          </section>
        ) : (
          <>
            {session.runtime === 'emulatorjs' && nativeSaveSyncPrefetchError && (
              <section className="rounded-mga border border-amber-400/30 bg-amber-400/10 p-4 text-sm text-amber-100">
                {nativeSaveSyncPrefetchError}
              </section>
            )}
            {showExternalSaveSyncBar && (
            <section className="rounded-mga border border-mga-border bg-mga-surface p-4">
              <div className="flex flex-wrap items-end gap-3">
                {usesSingleSaveSyncSnapshot ? (
                  <div className="min-w-[12rem]">
                    <span className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">Save Sync</span>
                    <div className="flex h-9 items-center rounded-mga border border-mga-border bg-mga-bg px-3 text-sm text-mga-text">
                      DOS save snapshot
                    </div>
                  </div>
                ) : (
                  <div className="min-w-[12rem]">
                    <label className="mb-1 block text-xs uppercase tracking-wide text-mga-muted">Save Slot</label>
                    <select
                      value={selectedSlot}
                      onChange={(event) => setSelectedSlot(event.target.value as (typeof SAVE_SYNC_SLOT_IDS)[number])}
                      className="h-9 w-full rounded-mga border border-mga-border bg-mga-bg px-3 text-sm text-mga-text"
                    >
                      {SAVE_SYNC_SLOT_IDS.map((slot) => (
                        <option key={slot} value={slot}>{slot}</option>
                      ))}
                    </select>
                  </div>
                )}

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

                <div className="min-w-[14rem] pb-2 text-xs text-mga-muted">
                  {!activeIntegrationId && 'Choose an active Save Sync integration in Settings to enable remote saves.'}
                  {activeIntegrationId && !bridgeReady && 'Waiting for runtime bridge...'}
                  {activeIntegrationId &&
                    bridgeReady &&
                    selection?.runtime !== 'jsdos' &&
                    !bridgeSupportsSaveSync &&
                    'This runtime page does not support save import/export yet.'}
                  {activeIntegrationId &&
                    bridgeReady &&
                    !saveSyncRuntimeSupported &&
                    'This launch does not support save import/export. js-dos save sync requires a bundle-backed session.'}
                  {activeIntegrationId && bridgeReady && bridgeSupportsSaveSync && currentSlot?.exists && (
                    <>
                      Remote {saveSyncSnapshotLabel}: {currentSlot.file_count ?? 0} files, {currentSlot.total_size ?? 0} bytes
                      {currentSlot.updated_at ? `, updated ${new Date(currentSlot.updated_at).toLocaleString()}` : ''}
                    </>
                  )}
                  {activeIntegrationId && bridgeReady && bridgeSupportsSaveSync && currentSlot && !currentSlot.exists && (
                    <>Remote {saveSyncSnapshotLabel} is empty.</>
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
            )}
            {!showExternalSaveSyncBar && runtimeError && (
              <p className="text-xs text-red-400">{runtimeError}</p>
            )}

            <section
              ref={playerShellRef}
              className="flex min-h-[70vh] min-w-0 flex-1 flex-col overflow-hidden rounded-[1.25rem] border border-mga-border bg-black shadow-lg shadow-black/25"
            >
              <div className="flex items-center justify-between border-b border-white/10 bg-black/80 px-4 py-3 text-sm text-white/80">
                <div className="flex min-w-0 items-center gap-2">
                  <PlayCircle size={16} />
                  <span className="truncate" title={playerWindowTitle}>{playerWindowTitle}</span>
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
                  {session && runtimeLabel ? (
                    <span
                      className="inline-flex h-7 min-w-7 items-center justify-center rounded-full border border-white/10 bg-white/5 px-2"
                      title={runtimeLabel}
                      aria-label={runtimeLabel}
                    >
                      <BrandIcon brand={browserPlayRuntimeBrand(session.runtime)} className="h-4 w-4" />
                    </span>
                  ) : null}
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
