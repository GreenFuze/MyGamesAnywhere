import type {
  GameDetailResponse,
  SaveDomainCapability,
  SourceCacheEntry,
  SourceGameDetailDTO,
} from '../api/client.ts'
import { sourceVersionContext } from './sourceCapabilities.ts'

export type GameFileLocationKind = 'source' | 'prepared' | 'installed' | 'emulator'

export type GameFileLocationView = {
  id: string
  kind: GameFileLocationKind
  title: string
  context: string
  status: string
  ownerProfileId: string
  ownerProfileName: string
  sourceGameId: string
  integrationId?: string
  integrationLabel?: string
  deviceId?: string
  deviceName?: string
  osUser?: string
  routeId?: string
  routeLabel?: string
  path?: string
  fileCount: number
  size: number
  save?: SaveDomainCapability
  manageHref: string
  manageLabel: string
  accessEvidence: string[]
}

type ProfileIdentity = {
  id: string
  displayName: string
}

function requireProfile(profile: ProfileIdentity): ProfileIdentity {
  const id = profile.id.trim()
  const displayName = profile.displayName.trim()
  if (!id || !displayName) {
    throw new Error('A selected profile identity is required to build game file locations.')
  }
  return { id, displayName }
}

function sumSourceSize(source: SourceGameDetailDTO): number {
  return source.files.reduce((total, file) => total + (Number.isFinite(file.size) ? file.size : 0), 0)
}

function sourceStatus(source: SourceGameDetailDTO): string {
  switch (source.status) {
    case 'found': return source.files.length > 0 ? 'Available' : 'Connected'
    case 'missing': return 'No longer found'
    default: return source.status || 'Unknown'
  }
}

function preparedStatus(entry: SourceCacheEntry): string {
  switch (entry.status) {
    case 'ready': return 'Ready for play'
    case 'preparing':
    case 'running': return 'Preparing'
    case 'failed': return 'Preparation needs attention'
    default: return entry.status || 'Unknown'
  }
}

function sourceLocation(
  source: SourceGameDetailDTO,
  profile: ProfileIdentity,
): GameFileLocationView {
  const label = source.integration_label || source.integration_id
  return {
    id: `source:${source.id}`,
    kind: 'source',
    title: label,
    context: sourceVersionContext(source),
    status: sourceStatus(source),
    ownerProfileId: profile.id,
    ownerProfileName: profile.displayName,
    sourceGameId: source.id,
    integrationId: source.integration_id,
    integrationLabel: label,
    path: source.root_path,
    fileCount: source.files.length,
    size: sumSourceSize(source),
    save: source.save,
    manageHref: `/settings?tab=integrations&integration=${encodeURIComponent(source.integration_id)}`,
    manageLabel: 'Manage connection',
    accessEvidence: [
      `Profile: ${profile.displayName}`,
      `Connection: ${label}`,
      `Copy: ${source.id}`,
    ],
  }
}

function preparedLocation(
  entry: SourceCacheEntry,
  source: SourceGameDetailDTO,
  profile: ProfileIdentity,
): GameFileLocationView {
  const label = source.integration_label || source.integration_id
  return {
    id: `prepared:${entry.id}`,
    kind: 'prepared',
    title: 'Prepared copy',
    context: `${sourceVersionContext(source)} · ${entry.profile}`,
    status: preparedStatus(entry),
    ownerProfileId: profile.id,
    ownerProfileName: profile.displayName,
    sourceGameId: source.id,
    integrationId: source.integration_id,
    integrationLabel: label,
    path: entry.source_path,
    fileCount: entry.file_count,
    size: entry.size,
    save: source.save,
    manageHref: '/settings?tab=cache',
    manageLabel: 'Manage storage',
    accessEvidence: [
      `Profile: ${profile.displayName}`,
      `Connection: ${label}`,
      `Copy: ${source.id}`,
      `Prepared for: ${entry.profile}`,
    ],
  }
}

export function buildGameFileLocations(
  game: Pick<GameDetailResponse, 'id' | 'source_games'>,
  preparedEntries: SourceCacheEntry[],
  selectedProfile: ProfileIdentity,
): GameFileLocationView[] {
  const profile = requireProfile(selectedProfile)
  const locations = game.source_games.map((source) => sourceLocation(source, profile))
  const sources = new Map(game.source_games.map((source) => [source.id, source]))

  for (const entry of preparedEntries) {
    if (entry.canonical_game_id !== game.id) continue
    const source = sources.get(entry.source_game_id)
    if (!source || entry.integration_id !== source.integration_id) continue
    locations.push(preparedLocation(entry, source, profile))
  }

  return locations
}

export function gameFileLocationSaveLabel(location: Pick<GameFileLocationView, 'save'>): string {
  if (!location.save) return 'Save compatibility not known'
  switch (location.save.status) {
    case 'available': return 'MGA save backup available'
    case 'provider_managed': return 'Saves managed by provider'
    case 'needs_adapter': return 'Save backup needs setup'
    case 'unsupported': return 'Save backup not supported'
    default: return 'Save compatibility not known'
  }
}
