import { profileStorageKey } from '@/lib/profileStorage'

const activeScanJobStorageKey = 'activeScanJobId'

export function readStoredScanJobId(): string | null {
  if (typeof window === 'undefined') return null
  return window.sessionStorage.getItem(profileStorageKey(activeScanJobStorageKey))
}

export function writeStoredScanJobId(jobId: string | null) {
  if (typeof window === 'undefined') return
  if (jobId) {
    window.sessionStorage.setItem(profileStorageKey(activeScanJobStorageKey), jobId)
  } else {
    window.sessionStorage.removeItem(profileStorageKey(activeScanJobStorageKey))
  }
}
