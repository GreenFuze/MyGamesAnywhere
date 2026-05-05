import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'

/** Parsed SSE message from GET /api/events. */
export type SSEMessage = {
  type: string
  data: unknown
}

type Listener = (data: unknown) => void

type SSEContextValue = {
  lastEvent: SSEMessage | null
  subscribe: (eventType: string, callback: Listener) => () => void
  connected: boolean
}

const knownTypes = [
  // Scan lifecycle (from scan_events.md).
  'scan_started',
  'scan_integration_started',
  'scan_integration_skipped',
  'scan_source_list_started',
  'scan_source_list_complete',
  'scan_scanner_started',
  'scan_scanner_progress',
  'scan_scanner_complete',
  'scan_metadata_started',
  'scan_metadata_phase',
  'scan_metadata_plugin_started',
  'scan_metadata_game_progress',
  'scan_metadata_plugin_complete',
  'scan_metadata_plugin_error',
  'scan_metadata_consensus_complete',
  'scan_metadata_finished',
  'scan_persist_started',
  'scan_integration_complete',
  'scan_cancel_requested',
  'scan_cancelled',
  'scan_complete',
  'scan_error',

  // Integration CRUD notifications.
  'integration_created',
  'integration_updated',
  'integration_deleted',

  // Integration status check.
  'integration_status_run_started',
  'integration_status_checked',
  'integration_status_run_complete',

  // Sync notifications.
  'sync_operation_started',
  'sync_operation_finished',
  'settings_sync_auto_push_complete',
  'settings_sync_auto_push_failed',
  'sync_key_stored',
  'sync_key_cleared',

  // Save sync migration notifications.
  'save_sync_migration_started',
  'save_sync_migration_progress',
  'save_sync_migration_completed',
  'save_sync_migration_failed',

  // Plugin lifecycle and generic errors.
  'plugin_process_exited',
  'operation_error',

  // OAuth flow events.
  'oauth_complete',
  'oauth_error',
] as const

const SSEContext = createContext<SSEContextValue | null>(null)

export function SSEProvider({ children }: { children: ReactNode }) {
  const [connected, setConnected] = useState(false)
  const [lastEvent, setLastEvent] = useState<SSEMessage | null>(null)

  const listenersRef = useRef<Map<string, Set<Listener>>>(new Map())
  const esRef = useRef<EventSource | null>(null)
  const retryRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const dispatch = useCallback((msg: SSEMessage) => {
    setLastEvent(msg)

    const listeners = listenersRef.current.get(msg.type)
    if (listeners) {
      for (const cb of listeners) cb(msg.data)
    }

    const wildcards = listenersRef.current.get('*')
    if (wildcards) {
      for (const cb of wildcards) cb(msg)
    }
  }, [])

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }

    const es = new EventSource('/api/events')
    esRef.current = es

    es.onopen = () => {
      setConnected(true)
      retryRef.current = 0
    }

    for (const eventType of knownTypes) {
      es.addEventListener(eventType, (e: MessageEvent) => {
        let data: unknown
        try {
          data = JSON.parse(e.data)
        } catch {
          data = e.data
        }
        dispatch({ type: eventType, data })
      })
    }

    es.onmessage = (e: MessageEvent) => {
      let data: unknown
      try {
        data = JSON.parse(e.data)
      } catch {
        data = e.data
      }
      dispatch({ type: 'message', data })
    }

    es.onerror = () => {
      setConnected(false)
      es.close()
      esRef.current = null

      const delay = Math.min(1000 * 2 ** retryRef.current, 30_000)
      retryRef.current += 1
      timerRef.current = setTimeout(connect, delay)
    }
  }, [dispatch])

  useEffect(() => {
    connect()
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
      if (esRef.current) {
        esRef.current.close()
        esRef.current = null
      }
    }
  }, [connect])

  const subscribe = useCallback((eventType: string, callback: Listener) => {
    const map = listenersRef.current
    if (!map.has(eventType)) {
      map.set(eventType, new Set())
    }
    map.get(eventType)?.add(callback)

    return () => {
      const set = map.get(eventType)
      if (!set) return
      set.delete(callback)
      if (set.size === 0) map.delete(eventType)
    }
  }, [])

  const value = useMemo(
    () => ({ lastEvent, subscribe, connected }),
    [connected, lastEvent, subscribe],
  )

  return <SSEContext.Provider value={value}>{children}</SSEContext.Provider>
}

export function useSSE() {
  const ctx = useContext(SSEContext)
  if (!ctx) {
    throw new Error('useSSE must be used inside <SSEProvider>')
  }
  return ctx
}
