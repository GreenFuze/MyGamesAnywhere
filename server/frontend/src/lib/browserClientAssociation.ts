const ASSOCIATION_KEY = 'mga.browserClientEndpoint'
const ASSOCIATION_EVENT = 'mga:browser-client-association-change'

type AssociationChangeDetail = {
  endpointID: string
}

type PairingCandidate = {
  id: string
  status: string
}

const connectedStates = new Set(['ready', 'busy', 'update_required', 'error'])

/**
 * Records which endpoint proved it handled an mga:// action initiated by this
 * browser origin. This is presence evidence only; server-side profile grants
 * remain the authorization boundary.
 */
export class BrowserClientAssociation {
  static get(): string {
    if (typeof window === 'undefined') return ''
    try {
      return window.localStorage.getItem(ASSOCIATION_KEY) ?? ''
    } catch {
      return ''
    }
  }

  static set(endpointID: string): void {
    if (!endpointID || typeof window === 'undefined') return
    try {
      window.localStorage.setItem(ASSOCIATION_KEY, endpointID)
    } catch {
      // Presence is a browser convenience; the server remains authoritative.
    }
    window.dispatchEvent(new CustomEvent<AssociationChangeDetail>(ASSOCIATION_EVENT, {
      detail: { endpointID },
    }))
  }

  static subscribe(listener: () => void): () => void {
    if (typeof window === 'undefined') return () => undefined
    const onAssociationChange = () => listener()
    const onStorage = (event: StorageEvent) => {
      if (event.key === ASSOCIATION_KEY) listener()
    }
    window.addEventListener(ASSOCIATION_EVENT, onAssociationChange)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener(ASSOCIATION_EVENT, onAssociationChange)
      window.removeEventListener('storage', onStorage)
    }
  }
}

export function resolveBrowserClientEndpointID(storedID: string, endpointIDs: string[]): string {
  return storedID && endpointIDs.includes(storedID) ? storedID : ''
}

export function findNewConnectedEndpointIDs(
  baselineEndpointIDs: string[],
  endpoints: PairingCandidate[],
): string[] {
  const baseline = new Set(baselineEndpointIDs)
  return endpoints
    .filter((endpoint) => !baseline.has(endpoint.id) && connectedStates.has(endpoint.status))
    .map((endpoint) => endpoint.id)
}

export function resolveNewlyPairedEndpointID(
  baselineEndpointIDs: string[],
  endpoints: PairingCandidate[],
): string {
  const candidates = findNewConnectedEndpointIDs(baselineEndpointIDs, endpoints)
  return candidates.length === 1 ? candidates[0] : ''
}
