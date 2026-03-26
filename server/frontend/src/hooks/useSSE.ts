import { useEffect, useRef, useCallback, useState } from 'react'

/** Parsed SSE message from GET /api/events. */
export type SSEMessage = {
  type: string
  data: unknown
}

type Listener = (data: unknown) => void

/**
 * Hook that maintains a persistent EventSource connection to the server's
 * SSE endpoint (GET /api/events). Auto-reconnects on disconnect with
 * exponential backoff.
 *
 * Returns:
 *  - lastEvent: the most recent SSE message (any type)
 *  - subscribe(eventType, callback): register a listener; returns unsubscribe fn
 *  - connected: whether the EventSource is currently open
 */
export function useSSE() {
  const [connected, setConnected] = useState(false)
  const [lastEvent, setLastEvent] = useState<SSEMessage | null>(null)

  // Map of event type → set of listener callbacks.
  const listenersRef = useRef<Map<string, Set<Listener>>>(new Map())

  // EventSource ref for cleanup.
  const esRef = useRef<EventSource | null>(null)

  // Reconnect state.
  const retryRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  /** Dispatch a parsed SSE message to all matching listeners. */
  const dispatch = useCallback((msg: SSEMessage) => {
    setLastEvent(msg)

    // Notify listeners registered for this specific event type.
    const listeners = listenersRef.current.get(msg.type)
    if (listeners) {
      for (const cb of listeners) cb(msg.data)
    }

    // Notify wildcard listeners (type === '*').
    const wildcards = listenersRef.current.get('*')
    if (wildcards) {
      for (const cb of wildcards) cb(msg)
    }
  }, [])

  /** Connect / reconnect the EventSource. */
  const connect = useCallback(() => {
    // Close existing connection if any.
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

    // The server sends named events (event: <type>\ndata: <json>).
    // EventSource only fires 'message' for unnamed events, so we use
    // the raw onmessage + addEventListener approach. Since we don't know
    // all event types upfront, we parse the raw stream via onmessage for
    // unnamed events and rely on the fact that named events need explicit
    // addEventListener. Instead, we'll hook into the lower level.
    //
    // Actually, EventSource with named events requires addEventListener
    // per event type. Since we don't know them upfront, we'll use a
    // different approach: fetch + ReadableStream for full control.
    // But for simplicity and browser compat, let's keep EventSource and
    // re-register listeners dynamically.
    //
    // Simpler approach: we know the server event types from the codebase.
    // Register all known event types.
    const knownTypes = [
      // Scan lifecycle (from scan_events.md).
      'scan_started',
      'scan_integration_started',
      'scan_integration_skipped',
      'scan_source_list_started',
      'scan_source_list_complete',
      'scan_scanner_started',
      'scan_scanner_complete',
      'scan_metadata_started',
      'scan_metadata_phase',
      'scan_metadata_plugin_started',
      'scan_metadata_plugin_complete',
      'scan_metadata_plugin_error',
      'scan_metadata_consensus_complete',
      'scan_metadata_finished',
      'scan_persist_started',
      'scan_integration_complete',
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
      'sync_key_stored',
      'sync_key_cleared',

      // Generic error.
      'operation_error',

      // OAuth flow events.
      'oauth_complete',
      'oauth_error',
    ]

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

    // Fallback: unnamed events (just in case).
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

      // Exponential backoff: 1s, 2s, 4s, 8s, ... capped at 30s.
      const delay = Math.min(1000 * 2 ** retryRef.current, 30_000)
      retryRef.current++
      timerRef.current = setTimeout(connect, delay)
    }
  }, [dispatch])

  // Establish connection on mount, clean up on unmount.
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

  /** Subscribe to a specific event type (or '*' for all). Returns unsubscribe fn. */
  const subscribe = useCallback((eventType: string, callback: Listener) => {
    const map = listenersRef.current
    if (!map.has(eventType)) {
      map.set(eventType, new Set())
    }
    map.get(eventType)!.add(callback)

    return () => {
      const set = map.get(eventType)
      if (set) {
        set.delete(callback)
        if (set.size === 0) map.delete(eventType)
      }
    }
  }, [])

  return { lastEvent, subscribe, connected }
}
