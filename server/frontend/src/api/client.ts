/** Same-origin in prod (SPA behind Go); Vite proxy in dev. */
const base = ''

export async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(`${base}${path}`, {
    headers: { Accept: 'application/json' },
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function postJson<T>(path: string, body: unknown): Promise<T | void> {
  const res = await fetch(`${base}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  if (res.status === 204 || res.headers.get('content-length') === '0') {
    return
  }
  const text = await res.text()
  if (!text) return
  return JSON.parse(text) as T
}

export async function putJson<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${base}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function deleteRequest(path: string): Promise<void> {
  const res = await fetch(`${base}${path}`, { method: 'DELETE' })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
}

export async function getHealth(): Promise<string> {
  const res = await fetch(`${base}/health`)
  if (!res.ok) throw new Error(`health: ${res.status}`)
  return res.text()
}

/** Lightweight row returned by POST /api/scan (not used for GET /api/games/{id}). */
export type GameSummary = {
  id: string
  title: string
  platform: string
  kind: string
  parent_game_id?: string
  group_kind?: string
  root_path?: string
  files?: GameFileDTO[]
  external_ids?: ExternalIDDTO[]
  is_game_pass?: boolean
  xcloud_available?: boolean
  store_product_id?: string
  xcloud_url?: string
}

export type GameFileDTO = {
  path: string
  role: string
  file_kind?: string
  size: number
}

export type ExternalIDDTO = {
  source: string
  external_id: string
  url?: string
}

export type GameMediaDetailDTO = {
  asset_id: number
  type: string
  url: string
  source?: string
  width?: number
  height?: number
  local_path?: string
  hash?: string
  mime_type?: string
}

export type SourceGameDetailDTO = {
  id: string
  integration_id: string
  plugin_id: string
  external_id: string
  raw_title: string
  platform: string
  kind: string
  group_kind?: string
  root_path?: string
  url?: string
  status: string
  last_seen_at?: string
  created_at: string
  files: GameFileDTO[]
  resolver_matches: unknown[]
}

export type CompletionTime = {
  main_story?: number
  main_extra?: number
  completionist?: number
  source?: string
}

/** Persisted library view preferences (stored in /api/config/frontend). */
export type LibraryPrefs = {
  viewMode: 'grid' | 'list'
  sortBy: 'title' | 'release_date' | 'platform' | 'rating'
  sortDir: 'asc' | 'desc'
}

/** Full row (GET /api/games/{id}/detail and each item in GET /api/games). */
export type GameDetailResponse = {
  id: string
  title: string
  platform: string
  kind: string
  group_kind?: string
  root_path?: string
  files?: GameFileDTO[]
  external_ids?: ExternalIDDTO[]
  description?: string
  release_date?: string
  genres?: string[]
  developer?: string
  publisher?: string
  rating?: number
  max_players?: number
  completion_time?: CompletionTime
  media?: GameMediaDetailDTO[]
  is_game_pass?: boolean
  xcloud_available?: boolean
  store_product_id?: string
  xcloud_url?: string
  source_games: SourceGameDetailDTO[]
}

export type ListGamesResponse = {
  total: number
  page: number
  page_size: number
  games: GameDetailResponse[]
}

export async function listGames(params?: { page?: number; page_size?: number }): Promise<ListGamesResponse> {
  const q = new URLSearchParams()
  if (params?.page !== undefined) q.set('page', String(params.page))
  if (params?.page_size !== undefined) q.set('page_size', String(params.page_size))
  const qs = q.toString()
  return getJson<ListGamesResponse>(qs ? `/api/games?${qs}` : '/api/games')
}

/** Same JSON as getGameDetail — kept for callers that prefer the /detail URL. */
export async function getGameDetail(id: string): Promise<GameDetailResponse> {
  return getJson<GameDetailResponse>(`/api/games/${encodeURIComponent(id)}/detail`)
}

export async function getGame(id: string): Promise<GameDetailResponse> {
  return getJson<GameDetailResponse>(`/api/games/${encodeURIComponent(id)}`)
}

export type FrontendConfig = Record<string, unknown>

export async function getFrontendConfig(): Promise<FrontendConfig> {
  return getJson<FrontendConfig>('/api/config/frontend')
}

export async function setFrontendConfig(cfg: FrontendConfig): Promise<void> {
  await postJson('/api/config/frontend', cfg)
}

// ─── Admin / Settings API types ─────────────────────────────────────

export type Integration = {
  id: string
  plugin_id: string
  label: string
  config_json: string
  integration_type: string
  created_at: string
  updated_at: string
}

export type IntegrationStatusEntry = {
  integration_id: string
  plugin_id: string
  label: string
  status: 'ok' | 'error' | 'unavailable'
  message: string
}

export type PluginInfo = {
  plugin_id: string
  plugin_version: string
  provides: string[]
  capabilities: string[]
  config?: Record<string, unknown>
}

export type ScanResult = {
  status: string
  games: GameSummary[]
}

export type ScanIntegrationResult = {
  integration_id: string
  label: string
  plugin_id: string
  games_found: number
  games_added: number
  games_removed: number
  error?: string
}

export type ScanReport = {
  id: string
  started_at: string
  finished_at: string
  duration_ms: number
  metadata_only: boolean
  integration_ids: string[]
  games_added: number
  games_removed: number
  games_updated: number
  total_games: number
  integration_results: ScanIntegrationResult[]
}

export type SyncStatus = {
  configured: boolean
  has_stored_key: boolean
  last_push?: string
  last_pull?: string
}

export type PushResult = {
  status: string
  exported_at: string
  integrations: number
  settings: number
  remote_versions: number
}

export type PullResult = {
  status: string
  result: {
    integrations_added: number
    integrations_updated: number
    integrations_skipped: number
    settings_added: number
    settings_updated: number
    settings_skipped: number
    remote_exported_at: string
  }
}

export type LibraryStats = {
  canonical_game_count: number
  source_game_found_count: number
  source_game_total_count: number
  by_platform: Record<string, number>
  by_kind: Record<string, number>
  by_integration_id: Record<string, number>
  by_plugin_id: Record<string, number>
  by_metadata_plugin_id: Record<string, number>
  canonical_with_resolver_title: number
  percent_with_resolver_title: number
}

export type IntegrationGameItem = {
  id: string
  title: string
  platform: string
}

// ─── Admin / Settings API functions ─────────────────────────────────

export async function listIntegrations(): Promise<Integration[]> {
  return getJson<Integration[]>('/api/integrations')
}

export async function getIntegrationStatus(): Promise<IntegrationStatusEntry[]> {
  return getJson<IntegrationStatusEntry[]>('/api/integrations/status')
}

export async function checkIntegrationStatus(id: string): Promise<IntegrationStatusEntry> {
  return getJson<IntegrationStatusEntry>(`/api/integrations/${encodeURIComponent(id)}/status`)
}

export type OAuthRequiredResponse = {
  status: 'oauth_required'
  plugin_id: string
  authorize_url: string
  state: string
}

export type CreateIntegrationResult = Integration | OAuthRequiredResponse

export function isOAuthRequired(result: CreateIntegrationResult): result is OAuthRequiredResponse {
  return 'status' in result && result.status === 'oauth_required'
}

export async function createIntegration(body: {
  plugin_id: string
  label: string
  integration_type: string
  config?: Record<string, unknown>
}): Promise<CreateIntegrationResult> {
  const res = await fetch(`${base}/api/integrations`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  })

  if (res.status === 409) {
    // Backend returns { error, integration_id, integration } on duplicate.
    const data = await res.json().catch(() => null)
    const existingLabel = data?.integration?.label
    throw new DuplicateIntegrationError(existingLabel)
  }

  // 202 = OAuth consent required before integration can be created.
  if (res.status === 202) {
    return res.json() as Promise<OAuthRequiredResponse>
  }

  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `${res.status} ${res.statusText}`)
  }

  return res.json() as Promise<Integration>
}

/** Thrown when creating an integration that already exists with the same config. */
export class DuplicateIntegrationError extends Error {
  constructor(public existingLabel?: string) {
    super(
      existingLabel
        ? `An integration with identical configuration already exists: "${existingLabel}"`
        : 'An integration with identical configuration already exists.',
    )
    this.name = 'DuplicateIntegrationError'
  }
}

export async function updateIntegration(
  id: string,
  body: { label?: string; integration_type?: string; config?: Record<string, unknown> },
): Promise<Integration> {
  return putJson<Integration>(`/api/integrations/${encodeURIComponent(id)}`, body)
}

export async function deleteIntegration(id: string): Promise<void> {
  return deleteRequest(`/api/integrations/${encodeURIComponent(id)}`)
}

export async function getIntegrationGames(id: string): Promise<IntegrationGameItem[]> {
  return getJson<IntegrationGameItem[]>(`/api/integrations/${encodeURIComponent(id)}/games`)
}

export async function getIntegrationEnrichedGames(id: string): Promise<IntegrationGameItem[]> {
  return getJson<IntegrationGameItem[]>(`/api/integrations/${encodeURIComponent(id)}/enriched-games`)
}

export async function listPlugins(): Promise<PluginInfo[]> {
  return getJson<PluginInfo[]>('/api/plugins')
}

export async function getPlugin(id: string): Promise<PluginInfo> {
  return getJson<PluginInfo>(`/api/plugins/${encodeURIComponent(id)}`)
}

export async function triggerScan(
  integrationIds?: string[],
  opts?: { metadataOnly?: boolean },
): Promise<ScanResult> {
  const body: Record<string, unknown> = {}
  if (integrationIds) body.game_sources = integrationIds
  if (opts?.metadataOnly) body.metadata_only = true
  return postJson<ScanResult>('/api/scan', body) as Promise<ScanResult>
}

export async function getScanReports(limit = 10): Promise<ScanReport[]> {
  return getJson<ScanReport[]>(`/api/scan/reports?limit=${limit}`)
}

export async function getScanReport(id: string): Promise<ScanReport> {
  return getJson<ScanReport>(`/api/scan/reports/${encodeURIComponent(id)}`)
}

export async function getSyncStatus(): Promise<SyncStatus> {
  return getJson<SyncStatus>('/api/sync/status')
}

export async function syncPush(passphrase?: string): Promise<PushResult> {
  return postJson<PushResult>('/api/sync/push', passphrase ? { passphrase } : {}) as Promise<PushResult>
}

export async function syncPull(passphrase?: string): Promise<PullResult> {
  return postJson<PullResult>('/api/sync/pull', passphrase ? { passphrase } : {}) as Promise<PullResult>
}

export async function storeKey(passphrase: string): Promise<void> {
  await postJson('/api/sync/key', { passphrase })
}

export async function clearKey(): Promise<void> {
  return deleteRequest('/api/sync/key')
}

export async function getStats(): Promise<LibraryStats> {
  return getJson<LibraryStats>('/api/stats')
}

// ─── Plugin Browse API ──────────────────────────────────────────────

export type BrowseFolder = { name: string; path: string }
export type BrowseResponse = { folders: BrowseFolder[] }

export async function browsePlugin(pluginId: string, path: string): Promise<BrowseResponse> {
  return postJson<BrowseResponse>(
    `/api/plugins/${encodeURIComponent(pluginId)}/browse`,
    { path },
  ) as Promise<BrowseResponse>
}
