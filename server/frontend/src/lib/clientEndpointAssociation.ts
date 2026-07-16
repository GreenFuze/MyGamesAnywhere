const ASSOCIATION_EVENT = 'mga:client-endpoint-association-change'

type AssociationChangeDetail = {
  profileID: string
  endpointID: string
}

/** Shared browser/profile preference used by both the top bar and Play page. */
export class ClientEndpointAssociation {
  static key(profileID: string): string {
    return `mga.clientEndpoint.${profileID}`
  }

  static get(profileID: string): string {
    if (!profileID || typeof window === 'undefined') return ''
    try {
      return window.localStorage.getItem(this.key(profileID)) ?? ''
    } catch {
      return ''
    }
  }

  static set(profileID: string, endpointID: string): void {
    if (!profileID || !endpointID || typeof window === 'undefined') return
    try {
      window.localStorage.setItem(this.key(profileID), endpointID)
    } catch {
      // This is a convenience preference; the server remains authoritative.
    }
    window.dispatchEvent(new CustomEvent<AssociationChangeDetail>(ASSOCIATION_EVENT, {
      detail: { profileID, endpointID },
    }))
  }

  static subscribe(profileID: string, listener: () => void): () => void {
    if (!profileID || typeof window === 'undefined') return () => undefined
    const onAssociationChange = (event: Event) => {
      const detail = (event as CustomEvent<AssociationChangeDetail>).detail
      if (detail?.profileID === profileID) listener()
    }
    const onStorage = (event: StorageEvent) => {
      if (event.key === this.key(profileID)) listener()
    }
    window.addEventListener(ASSOCIATION_EVENT, onAssociationChange)
    window.addEventListener('storage', onStorage)
    return () => {
      window.removeEventListener(ASSOCIATION_EVENT, onAssociationChange)
      window.removeEventListener('storage', onStorage)
    }
  }
}

export function resolveAssociatedEndpointID(storedID: string, endpointIDs: string[]): string {
  if (storedID && endpointIDs.includes(storedID)) return storedID
  return endpointIDs.length === 1 ? endpointIDs[0] : ''
}
