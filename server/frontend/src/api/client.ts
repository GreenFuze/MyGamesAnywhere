import type {
  FrontendConfig,
  GamePlayDTO,
  GameFileDTO,
  IntegrationGameItem,
  IntegrationStatusEntry,
  ManualReviewCandidateDetail,
  ManualReviewCandidateSummary,
  ManualReviewRedetectBatchResult,
  ManualReviewRedetectResponse,
  ManualReviewScope,
  ManualReviewSearchResponse,
  ManualReviewSearchResult,
  ResolverMatchDTO,
  SourceCacheEntry,
  SourceCacheJobStatus,
  SourceDeliveryDTO,
  SaveSyncMigrationRequest,
  SaveSyncMigrationStatus,
  SaveSyncPrefetchStatus,
  SaveSyncPutResult,
  SaveSyncSlotSummary,
  SaveSyncSnapshot,
  ScanJobStatus,
  ScanReport,
  SourceGamePlayDTO,
} from "@/api/generated/contracts";

export type {
  CollectionSectionConfig,
  CollectionSectionField,
  CollectionViewMode,
  DateFormat,
  DateTimePrefs,
  FrontendConfig,
  GameLaunchCandidateDTO,
  GameLaunchOptionDTO,
  GameLaunchSourceDTO,
  GamePlayDTO,
  GameFileDTO,
  IntegrationGameItem,
  IntegrationStatusEntry,
  LibraryPrefs,
  ManualReviewCandidateDetail,
  ManualReviewCandidateSummary,
  ManualReviewRedetectBatchResult,
  ManualReviewRedetectResponse,
  ManualReviewRedetectResult,
  ManualReviewRedetectStatus,
  ManualReviewScope,
  ManualReviewSearchProviderStatus,
  ManualReviewSearchResponse,
  ManualReviewSearchResult,
  RecentPlayedEntry,
  ResolverMatchDTO,
  SourceCacheEntry,
  SourceCacheJobStatus,
  SourceDeliveryDTO,
  SaveSyncConflict,
  SaveSyncMigrationRequest,
  SaveSyncMigrationScope,
  SaveSyncMigrationStatus,
  SaveSyncPrefetchStatus,
  SaveSyncPutResult,
  SaveSyncSlotSummary,
  SaveSyncSnapshot,
  SaveSyncSnapshotFile,
  ScanIntegrationResult,
  ScanJobIntegrationStatus,
  ScanJobMetadataProviderStatus,
  ScanJobProgress,
  ScanJobRecentEvent,
  ScanJobStatus,
  ScanReport,
  SourceGamePlayDTO,
  TimeFormat,
} from "@/api/generated/contracts";

/** Same-origin in prod (SPA behind Go); Vite proxy in dev. */
const base = "";
export const SELECTED_PROFILE_STORAGE_KEY = "mga.selectedProfileId";

function selectedProfileId(): string {
  try {
    return localStorage.getItem(SELECTED_PROFILE_STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

function withProfileHeaders(headers?: HeadersInit): Headers {
  const out = new Headers(headers);
  const profileId = selectedProfileId();
  if (profileId && !out.has("X-MGA-Profile-ID")) {
    out.set("X-MGA-Profile-ID", profileId);
  }
  return out;
}

function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const url = typeof input === "string" ? input : input.toString();
  const headers = url.includes("/api/") ? withProfileHeaders(init.headers) : new Headers(init.headers);
  return fetch(input, { ...init, headers });
}

export class ApiError extends Error {
  constructor(
    public path: string,
    public status: number,
    public statusText: string,
    public responseText?: string,
  ) {
    super(`${path}: ${status} ${statusText}`);
    this.name = "ApiError";
  }
}

async function buildApiError(path: string, res: Response): Promise<ApiError> {
  let responseText: string | undefined;
  try {
    responseText = await res.text();
  } catch {
    responseText = undefined;
  }
  return new ApiError(path, res.status, res.statusText, responseText);
}

export async function getJson<T>(path: string): Promise<T> {
  const res = await apiFetch(`${base}${path}`, {
    headers: withProfileHeaders({ Accept: "application/json" }),
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  return res.json() as Promise<T>;
}

export async function postJson<T>(
  path: string,
  body: unknown,
): Promise<T | void> {
  const res = await apiFetch(`${base}${path}`, {
    method: "POST",
    headers: withProfileHeaders({ "Content-Type": "application/json", Accept: "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return;
  }
  const text = await res.text();
  if (!text) return;
  return JSON.parse(text) as T;
}

export async function putJson<T>(path: string, body: unknown): Promise<T> {
  const res = await apiFetch(`${base}${path}`, {
    method: "PUT",
    headers: withProfileHeaders({ "Content-Type": "application/json", Accept: "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  return res.json() as Promise<T>;
}

export async function deleteRequest(path: string): Promise<void> {
  const res = await apiFetch(`${base}${path}`, { method: "DELETE" });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
}

export async function getHealth(): Promise<string> {
  const res = await fetch(`${base}/health`);
  if (!res.ok) throw new Error(`health: ${res.status}`);
  return res.text();
}

export type ProfileRole = "admin_player" | "player";

export type Profile = {
  id: string;
  display_name: string;
  avatar_key: string;
  role: ProfileRole;
  created_at: string;
  updated_at: string;
};

export type SetupStatus = {
  setup_required: boolean;
  profiles: Profile[];
};

export async function getSetupStatus(): Promise<SetupStatus> {
  return getJson<SetupStatus>("/api/setup/status");
}

export async function listProfiles(): Promise<Profile[]> {
  return getJson<Profile[]>("/api/profiles");
}

export async function startFreshSetup(body: { display_name: string; avatar_key?: string }): Promise<Profile> {
  return postJson<Profile>("/api/setup/start-fresh", body) as Promise<Profile>;
}

export type RestoreSyncSetupBody = {
  plugin_id?: string;
  label?: string;
  integration_type?: string;
  config?: Record<string, unknown>;
  passphrase: string;
  store_key?: boolean;
};

export type RestoreSyncSetupResult = {
  status: "ok";
  profile_id: string;
  integration_id?: string;
  scan_job?: ScanJobStatus;
  result?: {
    integrations_added: number;
    integrations_updated: number;
    integrations_skipped: number;
    settings_added: number;
    settings_updated: number;
    settings_skipped: number;
    remote_exported_at: string;
  };
};

export type RestoreSyncSetupOAuthRequired = {
  status: "oauth_required";
  plugin_id: string;
  authorize_url: string;
  state: string;
};

export type RestoreSyncSetupResponse = RestoreSyncSetupResult | RestoreSyncSetupOAuthRequired;

export function isRestoreSyncOAuthRequired(
  result: RestoreSyncSetupResponse,
): result is RestoreSyncSetupOAuthRequired {
  return result.status === "oauth_required";
}

export async function restoreSyncSetup(body: RestoreSyncSetupBody): Promise<RestoreSyncSetupResponse> {
  return postJson<RestoreSyncSetupResponse>("/api/setup/restore-sync", body) as Promise<RestoreSyncSetupResponse>;
}

export async function checkRestoreSyncSetup(body: RestoreSyncSetupBody): Promise<RestoreSyncSetupResponse> {
  return postJson<RestoreSyncSetupResponse>("/api/setup/restore-sync/check", body) as Promise<RestoreSyncSetupResponse>;
}

export async function browseRestoreSyncSetup(pluginId: string, path: string): Promise<BrowseResponse> {
  return postJson<BrowseResponse>("/api/setup/restore-sync/browse", {
    plugin_id: pluginId,
    path,
  }) as Promise<BrowseResponse>;
}

export async function createProfile(body: { display_name: string; avatar_key?: string; role: ProfileRole }): Promise<Profile> {
  return postJson<Profile>("/api/profiles", body) as Promise<Profile>;
}

export async function updateProfile(
  id: string,
  body: { display_name: string; avatar_key?: string; role: ProfileRole },
): Promise<Profile> {
  return putJson<Profile>(`/api/profiles/${encodeURIComponent(id)}`, body);
}

export async function deleteProfile(id: string): Promise<void> {
  return deleteRequest(`/api/profiles/${encodeURIComponent(id)}`);
}

/** Lightweight row used in scan/report contexts (not used for GET /api/games/{id}). */
export type GameSummary = {
  id: string;
  title: string;
  platform: string;
  kind: string;
  parent_game_id?: string;
  group_kind?: string;
  root_path?: string;
  files?: GameFileDTO[];
  external_ids?: ExternalIDDTO[];
  is_game_pass?: boolean;
  xcloud_available?: boolean;
  store_product_id?: string;
  xcloud_url?: string;
};

export type ExternalIDDTO = {
  source: string;
  external_id: string;
  url?: string;
};

export type GameMediaDetailDTO = {
  asset_id: number;
  type: string;
  url: string;
  source?: string;
  width?: number;
  height?: number;
  local_path?: string;
  hash?: string;
  mime_type?: string;
};

export type SourceGameDetailDTO = {
  id: string;
  integration_id: string;
  integration_label?: string;
  plugin_id: string;
  external_id: string;
  raw_title: string;
  platform: string;
  kind: string;
  group_kind?: string;
  root_path?: string;
  url?: string;
  status: string;
  last_seen_at?: string;
  created_at: string;
  files: GameFileDTO[];
  delivery?: SourceDeliveryDTO;
  play?: SourceGamePlayDTO;
  hard_delete?: {
    eligible: boolean;
    reason?: string;
  };
  resolver_matches: ResolverMatchDTO[];
};

export type CompletionTime = {
  main_story?: number;
  main_extra?: number;
  completionist?: number;
  source?: string;
};

/** Full row (GET /api/games/{id}/detail and each item in GET /api/games). */
export type GameDetailResponse = {
  id: string;
  title: string;
  favorite: boolean;
  platform: string;
  kind: string;
  group_kind?: string;
  root_path?: string;
  files?: GameFileDTO[];
  external_ids?: ExternalIDDTO[];
  description?: string;
  release_date?: string;
  genres?: string[];
  developer?: string;
  publisher?: string;
  rating?: number;
  max_players?: number;
  completion_time?: CompletionTime;
  media?: GameMediaDetailDTO[];
  cover_override?: GameMediaDetailDTO;
  hover_override?: GameMediaDetailDTO;
  background_override?: GameMediaDetailDTO;
  is_game_pass?: boolean;
  xcloud_available?: boolean;
  store_product_id?: string;
  xcloud_url?: string;
  play?: GamePlayDTO;
  achievement_summary?: AchievementSummaryDTO;
  source_games: SourceGameDetailDTO[];
};

export type AchievementDTO = {
  external_id: string;
  title: string;
  description: string;
  locked_icon?: string;
  unlocked_icon?: string;
  points?: number;
  rarity?: number;
  unlocked: boolean;
  unlocked_at?: string;
};

export type AchievementSetDTO = {
  source: string;
  external_game_id: string;
  source_game_id?: string;
  source_title?: string;
  platform?: string;
  integration_id?: string;
  integration_label?: string;
  plugin_id?: string;
  total_count: number;
  unlocked_count: number;
  total_points?: number;
  earned_points?: number;
  achievements: AchievementDTO[];
};

export type AchievementSummaryDTO = {
  source_count: number;
  total_count: number;
  unlocked_count: number;
  total_points?: number;
  earned_points?: number;
};

export type AchievementSystemSummaryDTO = {
  source: string;
  game_count: number;
  total_count: number;
  unlocked_count: number;
  total_points?: number;
  earned_points?: number;
};

export type AchievementGameSummaryDTO = {
  game: GameDetailResponse;
  systems: AchievementSystemSummaryDTO[];
};

export type AchievementsDashboardResponse = {
  totals: AchievementSummaryDTO;
  systems: AchievementSystemSummaryDTO[];
  games: AchievementGameSummaryDTO[];
};

export type AchievementExplorerGameDTO = {
  game: GameDetailResponse;
  systems: AchievementSetDTO[];
};

export type AchievementsExplorerResponse = {
  games: AchievementExplorerGameDTO[];
};

export type DeleteSourceGameResponse = {
  deleted_source_game_id: string;
  canonical_exists: boolean;
  game?: GameDetailResponse;
};

export type DeleteSourceGamePreviewItem = {
  path: string;
  object_id?: string;
  is_dir?: boolean;
  size?: number;
  action: string;
};

export type DeleteSourceGamePreview = {
  source_game_id: string;
  plugin_id: string;
  action: string;
  summary: string;
  items: DeleteSourceGamePreviewItem[];
  warnings?: string[];
};

export type ListGamesResponse = {
  total: number;
  page: number;
  page_size: number;
  games: GameDetailResponse[];
};

export async function listGames(params?: {
  page?: number;
  page_size?: number;
}): Promise<ListGamesResponse> {
  const q = new URLSearchParams();
  if (params?.page !== undefined) q.set("page", String(params.page));
  if (params?.page_size !== undefined)
    q.set("page_size", String(params.page_size));
  const qs = q.toString();
  return getJson<ListGamesResponse>(qs ? `/api/games?${qs}` : "/api/games");
}

/** Same JSON as getGameDetail — kept for callers that prefer the /detail URL. */
export async function getGameDetail(id: string): Promise<GameDetailResponse> {
  return getJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/detail`,
  );
}

export async function getGame(id: string): Promise<GameDetailResponse> {
  return getJson<GameDetailResponse>(`/api/games/${encodeURIComponent(id)}`);
}

export async function getGameAchievements(
  id: string,
): Promise<AchievementSetDTO[]> {
  return getJson<AchievementSetDTO[]>(
    `/api/games/${encodeURIComponent(id)}/achievements`,
  );
}

export async function getAchievementsDashboard(): Promise<AchievementsDashboardResponse> {
  return getJson<AchievementsDashboardResponse>("/api/achievements");
}

export async function getAchievementsExplorer(): Promise<AchievementsExplorerResponse> {
  return getJson<AchievementsExplorerResponse>("/api/achievements/explorer");
}

export async function setGameCoverOverride(
  id: string,
  mediaAssetId: number,
): Promise<GameDetailResponse> {
  return putJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/cover-override`,
    { media_asset_id: mediaAssetId },
  );
}

export async function clearGameCoverOverride(id: string): Promise<GameDetailResponse> {
  const path = `/api/games/${encodeURIComponent(id)}/cover-override`;
  const res = await apiFetch(`${base}${path}`, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  return res.json() as Promise<GameDetailResponse>;
}

export async function setGameHoverOverride(
  id: string,
  mediaAssetId: number,
): Promise<GameDetailResponse> {
  return putJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/hover-override`,
    { media_asset_id: mediaAssetId },
  );
}

export async function setGameBackgroundOverride(
  id: string,
  mediaAssetId: number,
): Promise<GameDetailResponse> {
  return putJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/background-override`,
    { media_asset_id: mediaAssetId },
  );
}

export async function setGameFavorite(id: string): Promise<GameDetailResponse> {
  return putJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/favorite`,
    {},
  );
}

export async function clearGameFavorite(id: string): Promise<GameDetailResponse> {
  const path = `/api/games/${encodeURIComponent(id)}/favorite`;
  const res = await apiFetch(`${base}${path}`, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  return res.json() as Promise<GameDetailResponse>;
}

export async function updateMediaAssetMetadata(
  assetId: number,
  body: { width: number; height: number; mime_type?: string },
): Promise<void> {
  const path = `/api/media/${encodeURIComponent(String(assetId))}/metadata`;
  const res = await apiFetch(`${base}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
}

export async function refreshGameMetadata(
  id: string,
): Promise<GameDetailResponse> {
  const response = await postJson<GameDetailResponse>(
    `/api/games/${encodeURIComponent(id)}/refresh-metadata`,
    {},
  );
  if (!response) {
    throw new Error("Refresh metadata request returned no response body.");
  }
  return response;
}

export async function deleteSourceGame(
  gameId: string,
  sourceGameId: string,
): Promise<DeleteSourceGameResponse> {
  const res = await apiFetch(
    `${base}/api/games/${encodeURIComponent(gameId)}/sources/${encodeURIComponent(sourceGameId)}`,
    {
      method: "DELETE",
      headers: { Accept: "application/json" },
    },
  );
  if (!res.ok) {
    throw await buildApiError(
      `/api/games/${encodeURIComponent(gameId)}/sources/${encodeURIComponent(sourceGameId)}`,
      res,
    );
  }
  return res.json() as Promise<DeleteSourceGameResponse>;
}

export async function previewDeleteSourceGame(
  gameId: string,
  sourceGameId: string,
): Promise<DeleteSourceGamePreview> {
  return postJson<DeleteSourceGamePreview>(
    `/api/games/${encodeURIComponent(gameId)}/sources/${encodeURIComponent(sourceGameId)}/delete-preview`,
    {},
  ) as Promise<DeleteSourceGamePreview>;
}

export async function getFrontendConfig(): Promise<FrontendConfig> {
  return getJson<FrontendConfig>("/api/config/frontend");
}

export async function setFrontendConfig(cfg: FrontendConfig): Promise<void> {
  await postJson("/api/config/frontend", cfg);
}

// ─── Admin / Settings API types ─────────────────────────────────────

export type Integration = {
  id: string;
  plugin_id: string;
  label: string;
  config_json: string;
  integration_type: string;
  created_at: string;
  updated_at: string;
};

export type PluginInfo = {
  plugin_id: string;
  plugin_version: string;
  provides: string[];
  capabilities: string[];
  config?: Record<string, unknown>;
};

export type ScanResult = {
  status: string;
  games: GameSummary[];
};

export type TriggerScanResult = {
  accepted: boolean;
  job: ScanJobStatus;
};

export type CancelScanResult = {
  accepted: boolean;
  job: ScanJobStatus;
};

export type IntegrationRefreshJobStatus = {
  job_id: string;
  integration_id: string;
  plugin_id: string;
  label: string;
  status: string;
  phase?: string;
  started_at?: string;
  finished_at?: string;
  items_total: number;
  items_completed: number;
  warning_count: number;
  error_count: number;
  current_item?: string;
  error?: string;
  warnings?: string[];
};

export type TriggerIntegrationRefreshResult = {
  accepted: boolean;
  job: IntegrationRefreshJobStatus;
};

export type SyncStatus = {
  configured: boolean;
  has_stored_key: boolean;
  last_push?: string;
  last_pull?: string;
};

export type PushResult = {
  status: string;
  exported_at: string;
  integrations: number;
  settings: number;
  remote_versions: number;
};

export type PullResult = {
  status: string;
  result: {
    integrations_added: number;
    integrations_updated: number;
    integrations_skipped: number;
    settings_added: number;
    settings_updated: number;
    settings_skipped: number;
    remote_exported_at: string;
  };
};

export type LibraryStats = {
  canonical_game_count: number;
  source_game_found_count: number;
  source_game_total_count: number;
  by_platform: Record<string, number>;
  by_decade: Record<string, number>;
  by_kind: Record<string, number>;
  top_genres: Record<string, number>;
  by_integration_id: Record<string, number>;
  by_plugin_id: Record<string, number>;
  by_metadata_plugin_id: Record<string, number>;
  canonical_with_resolver_title: number;
  percent_with_resolver_title: number;
  games_with_description: number;
  percent_with_description: number;
  games_with_media: number;
  games_with_achievements: number;
  percent_with_media: number;
  percent_with_achievements: number;
};

export type AboutInfo = {
  version: string;
  commit: string;
  build_date: string;
  author_credits: string[];
};

export type UpdateAsset = {
  os: string;
  arch: string;
  type: string;
  url: string;
  sha256: string;
  size: number;
  name?: string;
  version?: string;
};

export type UpdateStatus = {
  current_version: string;
  latest_version?: string;
  update_available: boolean;
  manifest_url?: string;
  release_notes_url?: string;
  install_type: string;
  downloaded_path?: string;
  downloaded_sha256?: string;
  selected_asset?: UpdateAsset;
  message?: string;
};

export type UpdateDownloadResult = {
  status: UpdateStatus;
  path: string;
  sha256: string;
  size: number;
};

export type UpdateApplyResult = {
  applied: boolean;
  message: string;
  path?: string;
};

// ─── Admin / Settings API functions ─────────────────────────────────

export async function listIntegrations(): Promise<Integration[]> {
  return getJson<Integration[]>("/api/integrations");
}

export async function getIntegrationStatus(): Promise<
  IntegrationStatusEntry[]
> {
  return getJson<IntegrationStatusEntry[]>("/api/integrations/status");
}

export async function checkIntegrationStatus(
  id: string,
): Promise<IntegrationStatusEntry> {
  return getJson<IntegrationStatusEntry>(
    `/api/integrations/${encodeURIComponent(id)}/status`,
  );
}

export type OAuthRequiredResponse = {
  status: "oauth_required";
  plugin_id: string;
  authorize_url: string;
  state: string;
};

export type CreateIntegrationResult = Integration | OAuthRequiredResponse;
export type UpdateIntegrationResult = Integration | OAuthRequiredResponse;
export type StartIntegrationAuthResult =
  | IntegrationStatusEntry
  | OAuthRequiredResponse;

export function isOAuthRequired(
  result:
    | CreateIntegrationResult
    | UpdateIntegrationResult
    | StartIntegrationAuthResult,
): result is OAuthRequiredResponse {
  return "status" in result && result.status === "oauth_required";
}

export async function createIntegration(body: {
  plugin_id: string;
  label: string;
  integration_type: string;
  config?: Record<string, unknown>;
}): Promise<CreateIntegrationResult> {
  const res = await apiFetch(`${base}/api/integrations`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });

  if (res.status === 409) {
    // Backend returns { error, message, integration_id, integration } on duplicate.
    const data = await res.json().catch(() => null);
    const existingLabel = data?.integration?.label;
    throw new DuplicateIntegrationError(existingLabel, data?.message);
  }

  // 202 = OAuth consent required before integration can be created.
  if (res.status === 202) {
    return res.json() as Promise<OAuthRequiredResponse>;
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `${res.status} ${res.statusText}`);
  }

  return res.json() as Promise<Integration>;
}

/** Thrown when creating an integration that already exists with the same config. */
export class DuplicateIntegrationError extends Error {
  constructor(public existingLabel?: string, message?: string) {
    super(
      message ?? (
      existingLabel
        ? `An integration with identical configuration already exists: "${existingLabel}"`
        : "An integration with identical configuration already exists."
      ),
    );
    this.name = "DuplicateIntegrationError";
  }
}

export async function updateIntegration(
  id: string,
  body: {
    label?: string;
    integration_type?: string;
    config?: Record<string, unknown>;
  },
): Promise<UpdateIntegrationResult> {
  const path = `/api/integrations/${encodeURIComponent(id)}`;
  const res = await apiFetch(`${base}${path}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });

  if (res.status === 202) {
    return res.json() as Promise<OAuthRequiredResponse>;
  }

  if (res.status === 409) {
    const data = await res.json().catch(() => null);
    const existingLabel = data?.integration?.label;
    throw new DuplicateIntegrationError(existingLabel, data?.message);
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `${res.status} ${res.statusText}`);
  }

  return res.json() as Promise<Integration>;
}

export async function startIntegrationAuth(
  id: string,
): Promise<StartIntegrationAuthResult> {
  const path = `/api/integrations/${encodeURIComponent(id)}/authorize`;
  const res = await apiFetch(`${base}${path}`, {
    method: "POST",
    headers: { Accept: "application/json" },
  });

  if (res.status === 202) {
    return res.json() as Promise<OAuthRequiredResponse>;
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `${res.status} ${res.statusText}`);
  }

  return res.json() as Promise<IntegrationStatusEntry>;
}

export async function deleteIntegration(id: string): Promise<void> {
  return deleteRequest(`/api/integrations/${encodeURIComponent(id)}`);
}

export async function getIntegrationGames(
  id: string,
): Promise<IntegrationGameItem[]> {
  return getJson<IntegrationGameItem[]>(
    `/api/integrations/${encodeURIComponent(id)}/games`,
  );
}

export async function getIntegrationEnrichedGames(
  id: string,
): Promise<IntegrationGameItem[]> {
  return getJson<IntegrationGameItem[]>(
    `/api/integrations/${encodeURIComponent(id)}/enriched-games`,
  );
}

export async function startIntegrationRefresh(
  id: string,
): Promise<TriggerIntegrationRefreshResult> {
  const path = `/api/integrations/${encodeURIComponent(id)}/refresh`;
  const res = await apiFetch(`${base}${path}`, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  if (res.status !== 202 && res.status !== 409) {
    throw await buildApiError(path, res);
  }
  return {
    accepted: res.status === 202,
    job: (await res.json()) as IntegrationRefreshJobStatus,
  };
}

export async function getIntegrationRefreshJob(
  jobId: string,
): Promise<IntegrationRefreshJobStatus> {
  return getJson<IntegrationRefreshJobStatus>(
    `/api/integration-refresh/jobs/${encodeURIComponent(jobId)}`,
  );
}

export async function listManualReviewCandidates(
  scope: ManualReviewScope,
  limit = 200,
): Promise<ManualReviewCandidateSummary[]> {
  return getJson<ManualReviewCandidateSummary[]>(
    `/api/review-candidates?scope=${encodeURIComponent(scope)}&limit=${limit}`,
  );
}

export async function getManualReviewCandidate(
  id: string,
): Promise<ManualReviewCandidateDetail> {
  return getJson<ManualReviewCandidateDetail>(
    `/api/review-candidates/${encodeURIComponent(id)}`,
  );
}

export async function searchManualReviewCandidate(
  id: string,
  query?: string,
): Promise<ManualReviewSearchResponse> {
  return postJson<ManualReviewSearchResponse>(
    `/api/review-candidates/${encodeURIComponent(id)}/search`,
    query !== undefined ? { query } : {},
  ) as Promise<ManualReviewSearchResponse>;
}

export async function applyManualReviewCandidate(
  id: string,
  body: ManualReviewSearchResult,
): Promise<ManualReviewCandidateDetail> {
  return postJson<ManualReviewCandidateDetail>(
    `/api/review-candidates/${encodeURIComponent(id)}/apply`,
    body,
  ) as Promise<ManualReviewCandidateDetail>;
}

export async function redetectManualReviewCandidate(
  id: string,
): Promise<ManualReviewRedetectResponse> {
  return postJson<ManualReviewRedetectResponse>(
    `/api/review-candidates/${encodeURIComponent(id)}/redetect`,
    {},
  ) as Promise<ManualReviewRedetectResponse>;
}

export async function redetectActiveManualReviewCandidates(): Promise<ManualReviewRedetectBatchResult> {
  return postJson<ManualReviewRedetectBatchResult>(
    "/api/review-candidates/redetect",
    {},
  ) as Promise<ManualReviewRedetectBatchResult>;
}

export async function markManualReviewCandidateNotAGame(
  id: string,
): Promise<ManualReviewCandidateDetail> {
  return postJson<ManualReviewCandidateDetail>(
    `/api/review-candidates/${encodeURIComponent(id)}/not-a-game`,
    {},
  ) as Promise<ManualReviewCandidateDetail>;
}

export async function unarchiveManualReviewCandidate(
  id: string,
): Promise<ManualReviewCandidateDetail> {
  return postJson<ManualReviewCandidateDetail>(
    `/api/review-candidates/${encodeURIComponent(id)}/unarchive`,
    {},
  ) as Promise<ManualReviewCandidateDetail>;
}

export type ManualReviewDeleteCandidateFilesResponse = {
  deleted_candidate_id: string;
  canonical_exists: boolean;
};

export async function deleteManualReviewCandidateFiles(
  id: string,
): Promise<ManualReviewDeleteCandidateFilesResponse> {
  const path = `/api/review-candidates/${encodeURIComponent(id)}/files`;
  const res = await apiFetch(path, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw await buildApiError(path, res);
  }
  return res.json() as Promise<ManualReviewDeleteCandidateFilesResponse>;
}

export async function previewDeleteManualReviewCandidateFiles(
  id: string,
): Promise<DeleteSourceGamePreview> {
  return postJson<DeleteSourceGamePreview>(
    `/api/review-candidates/${encodeURIComponent(id)}/files/delete-preview`,
    {},
  ) as Promise<DeleteSourceGamePreview>;
}

export async function listPlugins(): Promise<PluginInfo[]> {
  return getJson<PluginInfo[]>("/api/plugins");
}

export async function getPlugin(id: string): Promise<PluginInfo> {
  return getJson<PluginInfo>(`/api/plugins/${encodeURIComponent(id)}`);
}

export async function triggerScan(
  integrationIds?: string[],
  opts?: { metadataOnly?: boolean },
): Promise<TriggerScanResult> {
  const body: Record<string, unknown> = {};
  if (integrationIds) body.game_sources = integrationIds;
  if (opts?.metadataOnly) body.metadata_only = true;
  const res = await apiFetch(`${base}/api/scan`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });
  if (res.status !== 202 && res.status !== 409) {
    throw await buildApiError("/api/scan", res);
  }
  return {
    accepted: res.status === 202,
    job: (await res.json()) as ScanJobStatus,
  };
}

export async function getScanJob(jobId: string): Promise<ScanJobStatus> {
  return getJson<ScanJobStatus>(`/api/scan/jobs/${encodeURIComponent(jobId)}`);
}

export async function cancelScanJob(jobId: string): Promise<CancelScanResult> {
  const res = await apiFetch(
    `${base}/api/scan/jobs/${encodeURIComponent(jobId)}/cancel`,
    {
      method: "POST",
      headers: { Accept: "application/json" },
    },
  );
  if (res.status !== 200 && res.status !== 202 && res.status !== 409) {
    throw await buildApiError(
      `/api/scan/jobs/${encodeURIComponent(jobId)}/cancel`,
      res,
    );
  }
  return {
    accepted: res.status === 202,
    job: (await res.json()) as ScanJobStatus,
  };
}

export async function getScanReports(limit = 10): Promise<ScanReport[]> {
  return getJson<ScanReport[]>(`/api/scan/reports?limit=${limit}`);
}

export async function getScanReport(id: string): Promise<ScanReport> {
  return getJson<ScanReport>(`/api/scan/reports/${encodeURIComponent(id)}`);
}

export async function getSyncStatus(): Promise<SyncStatus> {
  return getJson<SyncStatus>("/api/sync/status");
}

export async function syncPush(passphrase?: string): Promise<PushResult> {
  return postJson<PushResult>(
    "/api/sync/push",
    passphrase ? { passphrase } : {},
  ) as Promise<PushResult>;
}

export async function syncPull(passphrase?: string): Promise<PullResult> {
  return postJson<PullResult>(
    "/api/sync/pull",
    passphrase ? { passphrase } : {},
  ) as Promise<PullResult>;
}

export async function storeKey(passphrase: string, currentPassphrase?: string): Promise<void> {
  await postJson("/api/sync/key", {
    passphrase,
    ...(currentPassphrase ? { current_passphrase: currentPassphrase } : {}),
  });
}

export async function clearKey(): Promise<void> {
  return deleteRequest("/api/sync/key");
}

export async function getStats(): Promise<LibraryStats> {
  return getJson<LibraryStats>("/api/stats");
}

export async function getAboutInfo(): Promise<AboutInfo> {
  return getJson<AboutInfo>("/api/about");
}

export async function getUpdateStatus(): Promise<UpdateStatus> {
  return getJson<UpdateStatus>("/api/update/status");
}

export async function checkForUpdates(): Promise<UpdateStatus> {
  return postJson<UpdateStatus>("/api/update/check", {}) as Promise<UpdateStatus>;
}

export async function downloadUpdate(): Promise<UpdateDownloadResult> {
  return postJson<UpdateDownloadResult>("/api/update/download", {}) as Promise<UpdateDownloadResult>;
}

export async function applyUpdate(): Promise<UpdateApplyResult> {
  return postJson<UpdateApplyResult>("/api/update/apply", {}) as Promise<UpdateApplyResult>;
}

// ─── Plugin Browse API ──────────────────────────────────────────────

export type BrowseFolder = { name: string; path: string };
export type BrowseResponse = { folders: BrowseFolder[] };

export async function browsePlugin(
  pluginId: string,
  path: string,
): Promise<BrowseResponse> {
  return postJson<BrowseResponse>(
    `/api/plugins/${encodeURIComponent(pluginId)}/browse`,
    { path },
  ) as Promise<BrowseResponse>;
}

export async function listGameSaveSyncSlots(params: {
  gameId: string;
  integrationId: string;
  sourceGameId: string;
  runtime: string;
}): Promise<SaveSyncSlotSummary[]> {
  const q = new URLSearchParams({
    integration_id: params.integrationId,
    source_game_id: params.sourceGameId,
    runtime: params.runtime,
  });
  const result = await getJson<{ slots: SaveSyncSlotSummary[] }>(
    `/api/games/${encodeURIComponent(params.gameId)}/save-sync/slots?${q.toString()}`,
  );
  return result.slots;
}

export async function getGameSaveSyncSlot(params: {
  gameId: string;
  integrationId: string;
  sourceGameId: string;
  runtime: string;
  slotId: string;
}): Promise<SaveSyncSnapshot> {
  const q = new URLSearchParams({
    integration_id: params.integrationId,
    source_game_id: params.sourceGameId,
    runtime: params.runtime,
  });
  return getJson<SaveSyncSnapshot>(
    `/api/games/${encodeURIComponent(params.gameId)}/save-sync/slots/${encodeURIComponent(params.slotId)}?${q.toString()}`,
  );
}

export async function putGameSaveSyncSlot(params: {
  gameId: string;
  slotId: string;
  integrationId: string;
  sourceGameId: string;
  runtime: string;
  baseManifestHash?: string;
  force?: boolean;
  snapshot: SaveSyncSnapshot;
}): Promise<SaveSyncPutResult> {
  const res = await apiFetch(
    `${base}/api/games/${encodeURIComponent(params.gameId)}/save-sync/slots/${encodeURIComponent(params.slotId)}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({
        integration_id: params.integrationId,
        source_game_id: params.sourceGameId,
        runtime: params.runtime,
        base_manifest_hash: params.baseManifestHash,
        force: params.force ?? false,
        snapshot: params.snapshot,
      }),
    },
  );

  if (res.status === 409) {
    return res.json() as Promise<SaveSyncPutResult>;
  }
  if (!res.ok) {
    throw await buildApiError(
      `/api/games/${params.gameId}/save-sync/slots/${params.slotId}`,
      res,
    );
  }
  return res.json() as Promise<SaveSyncPutResult>;
}

export async function startGameSaveSyncPrefetch(params: {
  gameId: string;
  integrationId: string;
  sourceGameId: string;
  runtime: string;
}): Promise<SaveSyncPrefetchStatus> {
  return postJson<SaveSyncPrefetchStatus>(
    `/api/games/${encodeURIComponent(params.gameId)}/save-sync/prefetch`,
    {
      integration_id: params.integrationId,
      source_game_id: params.sourceGameId,
      runtime: params.runtime,
    },
  ) as Promise<SaveSyncPrefetchStatus>;
}

export async function getSaveSyncPrefetchStatus(
  jobId: string,
): Promise<SaveSyncPrefetchStatus> {
  return getJson<SaveSyncPrefetchStatus>(
    `/api/save-sync/prefetch/${encodeURIComponent(jobId)}`,
  );
}

export async function startSaveSyncMigration(
  body: SaveSyncMigrationRequest,
): Promise<SaveSyncMigrationStatus> {
  return postJson<SaveSyncMigrationStatus>(
    "/api/save-sync/migrations",
    body,
  ) as Promise<SaveSyncMigrationStatus>;
}

export async function getSaveSyncMigrationStatus(
  jobId: string,
): Promise<SaveSyncMigrationStatus> {
  return getJson<SaveSyncMigrationStatus>(
    `/api/save-sync/migrations/${encodeURIComponent(jobId)}`,
  );
}

export async function prepareGameCache(params: {
  gameId: string;
  sourceGameId: string;
  profile: string;
}): Promise<{ accepted: boolean; immediate: boolean; job?: SourceCacheJobStatus }> {
  const res = await apiFetch(`${base}/api/games/${encodeURIComponent(params.gameId)}/cache/prepare`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({
      source_game_id: params.sourceGameId,
      profile: params.profile,
    }),
  });
  if (res.status !== 200 && res.status !== 202) {
    throw await buildApiError(`/api/games/${params.gameId}/cache/prepare`, res);
  }
  return res.json() as Promise<{ accepted: boolean; immediate: boolean; job?: SourceCacheJobStatus }>;
}

export async function getCacheJob(jobId: string): Promise<SourceCacheJobStatus> {
  return getJson<SourceCacheJobStatus>(`/api/cache/jobs/${encodeURIComponent(jobId)}`);
}

export async function listCacheJobs(limit = 25): Promise<SourceCacheJobStatus[]> {
  const result = await getJson<{ jobs: SourceCacheJobStatus[] }>(`/api/cache/jobs?limit=${limit}`);
  return result.jobs;
}

export async function listCacheEntries(): Promise<SourceCacheEntry[]> {
  const result = await getJson<{ entries: SourceCacheEntry[] }>(`/api/cache/entries`);
  return result.entries;
}

export async function deleteCacheEntry(entryId: string): Promise<void> {
  await deleteRequest(`/api/cache/entries/${encodeURIComponent(entryId)}`);
}

export async function clearCacheEntries(): Promise<void> {
  await postJson(`/api/cache/clear`, {});
}
