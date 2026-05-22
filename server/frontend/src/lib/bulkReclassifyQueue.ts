export type BulkReclassifyQueueItem = {
  gameId: string
  sourceGameId: string
  title: string
  platform: string
  pluginId?: string
}

const STORAGE_KEY = 'mga.bulkReclassifyQueue'

function parseQueue(raw: string | null): BulkReclassifyQueueItem[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .map((item): BulkReclassifyQueueItem | null => {
        if (!item || typeof item !== 'object') return null
        const value = item as Record<string, unknown>
        if (
          typeof value.gameId !== 'string' ||
          typeof value.sourceGameId !== 'string' ||
          typeof value.title !== 'string' ||
          typeof value.platform !== 'string'
        ) {
          return null
        }
        return {
          gameId: value.gameId,
          sourceGameId: value.sourceGameId,
          title: value.title,
          platform: value.platform,
          pluginId: typeof value.pluginId === 'string' ? value.pluginId : undefined,
        }
      })
      .filter((item): item is BulkReclassifyQueueItem => item !== null)
  } catch {
    return []
  }
}

export function readBulkReclassifyQueue(): BulkReclassifyQueueItem[] {
  if (typeof window === 'undefined') return []
  return parseQueue(window.sessionStorage.getItem(STORAGE_KEY))
}

export function writeBulkReclassifyQueue(items: BulkReclassifyQueueItem[]): void {
  if (typeof window === 'undefined') return
  if (items.length === 0) {
    window.sessionStorage.removeItem(STORAGE_KEY)
    return
  }
  window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(items))
}

export function clearBulkReclassifyQueue(): void {
  writeBulkReclassifyQueue([])
}

export function removeBulkReclassifyQueueItem(sourceGameId: string): BulkReclassifyQueueItem[] {
  const next = readBulkReclassifyQueue().filter((item) => item.sourceGameId !== sourceGameId)
  writeBulkReclassifyQueue(next)
  return next
}

export function buildBulkReclassifySearchParams(item: BulkReclassifyQueueItem): URLSearchParams {
  const params = new URLSearchParams()
  params.set('tab', 'undetected')
  params.set('scope', 'active')
  params.set('candidate_id', item.sourceGameId)
  params.set('reclassify_game_id', item.gameId)
  params.set('reclassify_title', item.title)
  params.set('reclassify_platform', item.platform)
  params.set('bulk_reclassify', '1')
  if (item.pluginId) params.set('reclassify_source', item.pluginId)
  return params
}
