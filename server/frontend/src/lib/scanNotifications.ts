export type ScanConnectionFailure = {
  integrationId?: string
  pluginId?: string
  label: string
  reason?: string
  error?: string
}

export type ScanConnectionFailureNotification = {
  title: string
  description: string
  action: {
    label: string
    href: string
  }
  detail?: string
}

function connectionPath(integrationId?: string, pluginId?: string): string {
  const params = new URLSearchParams({ tab: 'connections' })
  if (integrationId) params.set('integration', integrationId)
  else if (pluginId) params.set('plugin', pluginId)
  return `/settings?${params.toString()}`
}

export class ScanConnectionFailurePresenter {
  present(failure: ScanConnectionFailure): ScanConnectionFailureNotification | null {
    if (!this.requiresAttention(failure)) return null

    const action = {
      label: failure.reason === 'auth_required' ? 'Sign in again' : 'Review connection',
      href: connectionPath(failure.integrationId, failure.pluginId),
    }
    const preservedLibrary = 'Your existing games were kept.'

    switch (failure.reason) {
      case 'auth_required':
        return {
          title: `${failure.label} needs you to sign in`,
          description: `MGA could not check this connection. Sign in again, then rescan. ${preservedLibrary}`,
          action,
          detail: failure.error,
        }
      case 'invalid_config':
        return {
          title: `${failure.label} setup is incomplete`,
          description: `Open the connection, finish its setup, then rescan. ${preservedLibrary}`,
          action,
          detail: failure.error,
        }
      case 'plugin_not_found':
        return {
          title: `${failure.label} is unavailable`,
          description: `The connector is missing from this MGA Server. Check the connection or update MGA, then rescan. ${preservedLibrary}`,
          action,
          detail: failure.error,
        }
      default:
        return {
          title: `${failure.label} could not update`,
          description: `Open the connection to check it, then rescan. ${preservedLibrary}`,
          action,
          detail: failure.error,
        }
    }
  }

  private requiresAttention(failure: ScanConnectionFailure): boolean {
    return failure.reason !== 'no_source_capability' && failure.reason !== 'no_games'
  }
}
