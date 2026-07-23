import { useEffect, useRef } from 'react'
import { useSSE } from '@/hooks/useSSE'
import { useToast, type NotificationDetail } from '@/components/ui/toast'

type EventPayload = Record<string, unknown>

function readString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim().length > 0 ? value : undefined
}

function readNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function connectionPath(integrationId?: string, pluginId?: string): string {
  const params = new URLSearchParams({ tab: 'integrations' })
  if (integrationId) params.set('integration', integrationId)
  else if (pluginId) params.set('plugin', pluginId)
  return `/settings?${params.toString()}`
}

function readScanDetails(value: unknown): NotificationDetail[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((candidate) => {
    if (!candidate || typeof candidate !== 'object') return []
    const change = candidate as EventPayload
    const kind = readString(change.kind)
    const title = readString(change.title)
    if (!title || (kind !== 'added' && kind !== 'removed')) return []
    return [{
      kind,
      title,
      context: readString(change.integration_label) ?? readString(change.integration_id),
    } satisfies NotificationDetail]
  })
}

function isAuthenticationError(value?: string): boolean {
  if (!value) return false
  return /AUTH_(?:FAILED|REQUIRED)|not authenticated|re-?auth|sign[- ]?in required/i.test(value)
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
        const added = readNumber(data.games_added) ?? 0
        const removed = readNumber(data.games_removed) ?? 0
        const details = readScanDetails(data.changes)
        const detailsOmitted = readNumber(data.changes_omitted) ?? 0
        const automatic = readString(data.trigger) === 'background'
        if (automatic && added === 0 && removed === 0) return
        notify({
          tone: 'success',
          title: automatic ? 'Library updated automatically' : 'Scan complete',
          description:
            automatic
              ? `${added > 0 ? `${added} added` : 'No additions'}${removed > 0 ? `, ${removed} removed` : ''}${details.length > 0 ? '. Expand to see which games changed.' : ''}`
              : count !== undefined || duration !== undefined
              ? `${count !== undefined ? `${count} games in your library` : 'Library updated'}${duration !== undefined ? ` in ${Math.round(duration / 1000)}s` : ''}${details.length > 0 ? `. ${added} added, ${removed} removed.` : ''}`
              : 'The library scan finished successfully.',
          details,
          detailsOmitted,
        })
      }),
      subscribe('scan_error', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const integrationId = readString(data.integration_id)
        notify({
          tone: 'error',
          title: integrationId ? 'A connection could not update your library' : 'Library update failed',
          description: integrationId ? 'Open the connection to check it and try again.' : (readString(data.error) ?? 'MGA could not update the library.'),
          action: integrationId ? { label: 'Review connection', href: connectionPath(integrationId) } : undefined,
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
      subscribe('settings_sync_auto_push_failed', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const integrationId = readString(data.integration_id)
        const label = readString(data.label) ?? 'Settings sync'
        notify({
          tone: 'error',
          title: `${label} needs attention`,
          description: 'MGA saved your changes locally, but could not back them up. Open the connection to fix it.',
          action: { label: 'Review connection', href: connectionPath(integrationId, readString(data.plugin_id)) },
        })
      }),
      subscribe('plugin_process_exited', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const pluginId = readString(data.plugin_id)
        notify({
          tone: 'error',
          title: pluginId ? `${humanize(pluginId)} stopped` : 'A connection helper stopped',
          description: 'MGA could not continue using this connection. Open Connections to check it and try again.',
          action: { label: 'Review connections', href: connectionPath(undefined, pluginId) },
        })
      }),
      subscribe('operation_error', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const scope = readString(data.scope)
        const integrationId = readString(data.integration_id)
        const pluginId = readString(data.plugin_id)
        const integrationLabel = readString(data.integration_label) ?? (pluginId ? humanize(pluginId) : 'Connection')
        const error = readString(data.error)
        const authError = scope === 'achievements' && isAuthenticationError(error)
        if (scope === 'achievements') {
          notify({
            tone: 'error',
            title: `${integrationLabel} achievements need attention`,
            description: authError
              ? `Sign in to ${integrationLabel} again so MGA can update achievements${readString(data.game_title) ? ` for ${readString(data.game_title)}` : ''}.`
              : `MGA could not update achievements from ${integrationLabel}. Open the connection to review it.`,
            action: {
              label: authError ? 'Open sign-in controls' : 'Review connection',
              href: connectionPath(integrationId, pluginId),
            },
          })
          return
        }
        notify({
          tone: 'error',
          title: scope ? `${humanize(scope)} needs attention` : 'Something needs attention',
          description: error ?? 'MGA could not finish the operation.',
          action: scope === 'sync_key' ? { label: 'Review connection', href: connectionPath() } : undefined,
        })
      }),
      subscribe('installation_validation_finished', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const status = readString(data.status)
        if (status && status !== 'succeeded') {
          const endpointId = readString(data.endpoint_id)
          notify({
            tone: 'error',
            title: 'Installed game check failed',
            description: readString(data.error) ?? 'MGA could not check the installed games on this device.',
            action: endpointId ? { label: 'Review device', href: `/settings?tab=devices&device=${encodeURIComponent(endpointId)}` } : undefined,
          })
          return
        }
        const missing = readNumber(data.changed_missing) ?? 0
        const repair = readNumber(data.changed_needs_repair) ?? 0
        const restored = readNumber(data.restored) ?? 0
        if (missing === 0 && repair === 0 && restored === 0) return
        notify({
          tone: missing > 0 || repair > 0 ? 'error' : 'success',
          title: missing > 0 || repair > 0 ? 'Installed games need attention' : 'Installed games are available again',
          description: [
            missing > 0 ? `${missing} missing` : '',
            repair > 0 ? `${repair} need repair` : '',
            restored > 0 ? `${restored} restored` : '',
          ].filter(Boolean).join(', '),
          action: readString(data.endpoint_id) ? { label: 'Review installed games', href: `/settings?tab=devices&device=${encodeURIComponent(readString(data.endpoint_id)!)}` } : undefined,
        })
      }),
      subscribe('update_available', (raw) => {
        const data = (raw ?? {}) as EventPayload
        const version = readString(data.latest_version)
        notify({
          tone: 'info',
          title: version ? `MGA ${version} is available` : 'An MGA update is available',
          description: 'A newer MGA Server version is ready when you are.',
          action: { label: 'Open updates', href: '/settings?tab=update' },
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
            action: { label: 'Open connection', href: connectionPath(integrationId) },
          })
          return
        }

        notify({
          tone: 'error',
          title: `${label} needs attention`,
          description: readString(data.message) ?? `Status changed to ${nextStatus}.`,
          action: { label: nextStatus === 'oauth_required' ? 'Open sign-in controls' : 'Review connection', href: connectionPath(integrationId) },
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
