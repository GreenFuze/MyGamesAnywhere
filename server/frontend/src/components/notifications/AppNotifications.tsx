import { useEffect, useRef } from 'react'
import { useSSE } from '@/hooks/useSSE'
import { useToast } from '@/components/ui/toast'

type EventPayload = Record<string, unknown>

function readString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim().length > 0 ? value : undefined
}

function readNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

export function AppNotifications() {
  const { subscribe } = useSSE()
  const { notify } = useToast()
  const previousStatusesRef = useRef(new Map<string, string>())

  useEffect(() => {
    const unsubs = [
      subscribe('scan_complete', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const count = readNumber(data.canonical_games)
        const duration = readNumber(data.duration_ms)
        notify({
          tone: 'success',
          title: 'Scan complete',
          description:
            count !== undefined || duration !== undefined
              ? `${count !== undefined ? `${count} canonical games` : 'Catalog updated'}${duration !== undefined ? ` in ${Math.round(duration / 1000)}s` : ''}`
              : 'The library scan finished successfully.',
        })
      }),
      subscribe('scan_error', (raw) => {
        const data = (raw ?? {}) as EventPayload
        notify({
          tone: 'error',
          title: 'Scan failed',
          description: readString(data.error) ?? 'The scan ended with an error.',
        })
      }),
      subscribe('sync_operation_finished', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const operation = readString(data.operation) ?? 'sync'
        const ok = data.ok === true
        notify({
          tone: ok ? 'success' : 'error',
          title: ok ? `${capitalize(operation)} complete` : `${capitalize(operation)} failed`,
          description: ok
            ? `Cloud ${operation} finished successfully.`
            : readString(data.error) ?? `Cloud ${operation} failed.`,
        })
      }),
      subscribe('plugin_process_exited', (raw) => {
        const data = (raw ?? {}) as EventPayload
        notify({
          tone: 'error',
          title: 'Plugin process exited',
          description:
            readString(data.plugin_id) ??
            readString(data.detail) ??
            'A plugin process disconnected unexpectedly.',
        })
      }),
      subscribe('operation_error', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const scope = readString(data.scope)
        notify({
          tone: 'error',
          title: scope ? `${humanize(scope)} error` : 'Operation error',
          description: readString(data.error) ?? 'An operation failed.',
        })
      }),
      subscribe('integration_status_checked', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const integrationId = readString(data.integration_id)
        const label = readString(data.label) ?? readString(data.plugin_id) ?? 'Integration'
        const nextStatus = readString(data.status)
        if (!integrationId || !nextStatus) return

        const previous = previousStatusesRef.current.get(integrationId)
        previousStatusesRef.current.set(integrationId, nextStatus)

        if (!previous || previous === nextStatus) return

        if (nextStatus === 'ok') {
          notify({
            tone: 'success',
            title: `${label} is available`,
            description: readString(data.message) ?? 'Connectivity recovered.',
          })
          return
        }

        notify({
          tone: 'error',
          title: `${label} needs attention`,
          description: readString(data.message) ?? `Status changed to ${nextStatus}.`,
        })
      }),
    ]

    return () => {
      unsubs.forEach((unsub) => unsub())
    }
  }, [notify, subscribe])

  return null
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

function humanize(value: string): string {
  return value
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => capitalize(part))
    .join(' ')
}
