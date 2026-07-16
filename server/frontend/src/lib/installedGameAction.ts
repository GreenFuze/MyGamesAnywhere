export type InstalledGameActionIntent = 'launch' | 'device' | 'details' | 'disabled'

export type InstalledGameAction = {
  label: string
  intent: InstalledGameActionIntent
  disabled: boolean
  kind: 'play' | 'details'
}

export function resolveInstalledGameAction(input: {
  launching: boolean
  deviceStatus: string
  connected: boolean
  accessLevel: string
  launchSupported: boolean
  launchTarget: string
  canPlay: boolean
}): InstalledGameAction {
  if (input.launching) {
    return { label: 'Starting…', intent: 'disabled', disabled: true, kind: 'play' }
  }
  if (input.deviceStatus === 'update_required' || !input.launchSupported) {
    return { label: 'Needs update', intent: 'device', disabled: false, kind: 'details' }
  }
  if (!input.connected || input.deviceStatus === 'offline' || input.deviceStatus === 'error') {
    return { label: 'Offline', intent: 'device', disabled: false, kind: 'details' }
  }
  if (input.accessLevel === 'view') {
    return { label: 'View only', intent: 'disabled', disabled: true, kind: 'details' }
  }
  if (!input.launchTarget.trim()) {
    return { label: 'Choose executable', intent: 'details', disabled: false, kind: 'details' }
  }
  if (input.canPlay) {
    return { label: 'Play', intent: 'launch', disabled: false, kind: 'play' }
  }
  return { label: 'Unavailable', intent: 'device', disabled: false, kind: 'details' }
}
