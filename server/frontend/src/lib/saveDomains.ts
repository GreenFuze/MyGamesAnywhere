import type { GameDetailResponse, SaveDomainCapability } from '../api/client.ts'
import { launchOptionVersionContext, sourceVersionContext } from './sourceCapabilities.ts'

export type SaveDomainView = SaveDomainCapability & {
  context?: string
}

export function collectSaveDomains(game: Pick<GameDetailResponse, 'source_games' | 'play'>): SaveDomainView[] {
  const items: SaveDomainView[] = []
  for (const source of game.source_games ?? []) {
    if (source.save) items.push({ ...source.save, context: sourceVersionContext(source) })
  }
  for (const option of game.play?.options ?? []) {
	if (option.launchable && option.save) {
      items.push({ ...option.save, context: launchOptionVersionContext(option, game.source_games ?? []) })
    }
  }
  const seen = new Set<string>()
  return items.filter((item) => {
    const key = item.domain_id
    if (!key || seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export function saveDomainSummary(domains: SaveDomainView[]): string | null {
  if (domains.some((domain) => domain.access === 'mga_managed' && domain.mga_write)) return 'MGA save backup available'
  if (domains.some((domain) => domain.access === 'provider_opaque')) return 'Provider-managed saves'
  if (domains.some((domain) => domain.access === 'local_files')) return 'Local saves need setup'
  return domains.length > 0 ? 'Save handling unknown' : null
}

export function saveDomainStatusLabel(domain: SaveDomainCapability): string {
  switch (domain.status) {
    case 'available': return 'MGA backup available'
    case 'provider_managed': return 'Managed by provider'
    case 'needs_adapter': return 'Backup not set up yet'
    case 'unsupported': return 'Not supported'
    default: return 'Not known yet'
  }
}
