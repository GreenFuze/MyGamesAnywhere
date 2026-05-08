const activeScanJobStorageKey = 'mga.activeScanJobId'

export function readStoredScanJobId(): string | null {
  if (typeof window === 'undefined') return null
  return window.sessionStorage.getItem(activeScanJobStorageKey)
}

export function writeStoredScanJobId(jobId: string | null) {
  if (typeof window === 'undefined') return
  if (jobId) {
    window.sessionStorage.setItem(activeScanJobStorageKey, jobId)
  } else {
    window.sessionStorage.removeItem(activeScanJobStorageKey)
  }
}
