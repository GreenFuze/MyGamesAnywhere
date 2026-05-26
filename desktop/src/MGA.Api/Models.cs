using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Api;

// ---------------------------------------------------------------------------
// Game models
// ---------------------------------------------------------------------------

/// <summary>Minimal game summary for list/grid display.</summary>
public sealed record GameSummary
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("favorite")]
    public bool Favorite { get; init; }
}

/// <summary>Media asset associated with a game.</summary>
public sealed record GameMedia
{
    [JsonPropertyName("asset_id")]
    public int AssetId { get; init; }

    [JsonPropertyName("type")]
    public string Type { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string Url { get; init; } = string.Empty;

    [JsonPropertyName("width")]
    public int Width { get; init; }

    [JsonPropertyName("height")]
    public int Height { get; init; }
}

/// <summary>An external ID link (IGDB, Steam, etc.) attached to a game.</summary>
public sealed record ExternalIdDto
{
    [JsonPropertyName("source")]
    public string Source { get; init; } = string.Empty;

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string? Url { get; init; }
}

/// <summary>A single metadata resolver match stored against a source game.</summary>
public sealed record SourceResolverMatch
{
    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string? Title { get; init; }

    [JsonPropertyName("platform")]
    public string? Platform { get; init; }

    [JsonPropertyName("kind")]
    public string? Kind { get; init; }

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string? Url { get; init; }

    [JsonPropertyName("outvoted")]
    public bool Outvoted { get; init; }

    [JsonPropertyName("manual_selection")]
    public bool ManualSelection { get; init; }
}

/// <summary>A source game summary within game detail (integration + files).</summary>
public sealed record SourceGameSummary
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("integration_id")]
    public string IntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("integration_label")]
    public string? IntegrationLabel { get; init; }

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("raw_title")]
    public string RawTitle { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("root_path")]
    public string? RootPath { get; init; }

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("files")]
    public List<GameFileDto> Files { get; init; } = [];

    [JsonPropertyName("resolver_matches")]
    public List<SourceResolverMatch> ResolverMatches { get; init; } = [];
}

/// <summary>Achievement summary statistics for a game.</summary>
public sealed record AchievementSummary
{
    [JsonPropertyName("source_count")]
    public int SourceCount { get; init; }

    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }
}

/// <summary>Full game detail object returned by GET /api/games/{id}.</summary>
public sealed record GameDetail
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("favorite")]
    public bool Favorite { get; init; }

    [JsonPropertyName("description")]
    public string? Description { get; init; }

    [JsonPropertyName("release_date")]
    public string? ReleaseDate { get; init; }

    [JsonPropertyName("genres")]
    public List<string> Genres { get; init; } = [];

    [JsonPropertyName("developer")]
    public string? Developer { get; init; }

    [JsonPropertyName("publisher")]
    public string? Publisher { get; init; }

    [JsonPropertyName("rating")]
    public double Rating { get; init; }

    [JsonPropertyName("media")]
    public List<GameMedia> Media { get; init; } = [];

    [JsonPropertyName("cover_override")]
    public GameMedia? CoverOverride { get; init; }

    [JsonPropertyName("achievement_summary")]
    public AchievementSummary? AchievementSummary { get; init; }

    [JsonPropertyName("external_ids")]
    public List<ExternalIdDto> ExternalIds { get; init; } = [];

    [JsonPropertyName("source_games")]
    public List<SourceGameSummary> SourceGames { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Game Detail mutation responses
// ---------------------------------------------------------------------------

/// <summary>
/// Response from DELETE /api/games/{id}/sources/{source_game_id}.
/// When <see cref="CanonicalExists"/> is false the canonical game was deleted with its last source.
/// </summary>
public sealed record DeleteSourceGameResponse
{
    [JsonPropertyName("deleted_source_game_id")]
    public string DeletedSourceGameId { get; init; } = string.Empty;

    [JsonPropertyName("canonical_exists")]
    public bool CanonicalExists { get; init; }

    /// <summary>Updated game detail (omitted when the canonical was also deleted).</summary>
    [JsonPropertyName("game")]
    public GameDetail? Game { get; init; }

    [JsonPropertyName("warnings")]
    public List<string> Warnings { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Batch source-game delete models
// ---------------------------------------------------------------------------

/// <summary>
/// One item in a batch hard-delete request: identifies a source record
/// by both the canonical game ID and the source-game ID.
/// </summary>
public sealed record DeleteSourceGameBatchItem
{
    [JsonPropertyName("canonical_game_id")]
    public string CanonicalGameId { get; init; } = string.Empty;

    [JsonPropertyName("source_game_id")]
    public string SourceGameId { get; init; } = string.Empty;
}

/// <summary>Response from POST /api/games/sources/delete-batch.</summary>
public sealed record DeleteSourceGamesBatchResponse
{
    [JsonPropertyName("deleted_source_game_ids")]
    public List<string> DeletedSourceGameIds { get; init; } = [];

    [JsonPropertyName("warnings")]
    public List<string> Warnings { get; init; } = [];
}

/// <summary>
/// Response for canonical-grouping operations: split, merge, clear-pin.
/// </summary>
public sealed record CanonicalGroupingResponse
{
    [JsonPropertyName("source_game_id")]
    public string SourceGameId { get; init; } = string.Empty;

    [JsonPropertyName("old_canonical_game_id")]
    public string? OldCanonicalGameId { get; init; }

    [JsonPropertyName("canonical_game_id")]
    public string CanonicalGameId { get; init; } = string.Empty;

    /// <summary>Updated game detail for the (new or existing) canonical game.</summary>
    [JsonPropertyName("game")]
    public GameDetail? Game { get; init; }
}

// ---------------------------------------------------------------------------
// Canonical game search
// ---------------------------------------------------------------------------

/// <summary>A single result from GET /api/canonical-games/search.</summary>
public sealed record CanonicalGameSearchResult
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("source_count")]
    public int SourceCount { get; init; }
}

/// <summary>Response wrapper from GET /api/canonical-games/search.</summary>
public sealed record CanonicalGameSearchResponse
{
    [JsonPropertyName("games")]
    public List<CanonicalGameSearchResult> Games { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Paginated game list
// ---------------------------------------------------------------------------

/// <summary>Paginated response from GET /api/games.</summary>
public sealed record ListGamesResponse
{
    [JsonPropertyName("total")]
    public int Total { get; init; }

    [JsonPropertyName("page")]
    public int Page { get; init; }

    [JsonPropertyName("page_size")]
    public int PageSize { get; init; }

    [JsonPropertyName("games")]
    public List<GameDetail> Games { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Library stats
// ---------------------------------------------------------------------------

/// <summary>High-level library statistics from GET /api/stats.</summary>
public sealed record LibraryStats
{
    [JsonPropertyName("canonical_game_count")]
    public int CanonicalGameCount { get; init; }

    [JsonPropertyName("by_platform")]
    public Dictionary<string, int> ByPlatform { get; init; } = [];

    /// <summary>Game count keyed by integration label (source breakdown).</summary>
    [JsonPropertyName("by_integration")]
    public Dictionary<string, int> ByIntegration { get; init; } = [];

    /// <summary>Game count keyed by game kind (game / dlc / demo / etc.).</summary>
    [JsonPropertyName("by_kind")]
    public Dictionary<string, int> ByKind { get; init; } = [];

    /// <summary>Game count keyed by plugin ID.</summary>
    [JsonPropertyName("by_plugin")]
    public Dictionary<string, int> ByPlugin { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Achievement dashboard models
// ---------------------------------------------------------------------------

/// <summary>Overall achievement totals (total vs unlocked).</summary>
public sealed record AchievementTotals
{
    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }
}

/// <summary>Per-source achievement statistics for the dashboard systems list.</summary>
public sealed record AchievementSystemStat
{
    [JsonPropertyName("source")]
    public string Source { get; init; } = string.Empty;

    [JsonPropertyName("game_count")]
    public int GameCount { get; init; }

    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }

    [JsonPropertyName("total_points")]
    public int TotalPoints { get; init; }

    [JsonPropertyName("earned_points")]
    public int EarnedPoints { get; init; }
}

/// <summary>A single game entry inside the achievements dashboard.</summary>
public sealed record AchievementGameEntry
{
    [JsonPropertyName("game")]
    public GameDetail Game { get; init; } = new();

    [JsonPropertyName("systems")]
    public List<AchievementSystemStat> Systems { get; init; } = [];
}

/// <summary>Metadata about the last achievements refresh job.</summary>
public sealed record AchievementRefreshInfo
{
    [JsonPropertyName("total")]
    public int Total { get; init; }

    [JsonPropertyName("success_count")]
    public int SuccessCount { get; init; }

    [JsonPropertyName("last_successful_at")]
    public string? LastSuccessfulAt { get; init; }
}

/// <summary>Full response from GET /api/achievements.</summary>
public sealed record AchievementsDashboard
{
    [JsonPropertyName("totals")]
    public AchievementTotals Totals { get; init; } = new();

    [JsonPropertyName("systems")]
    public List<AchievementSystemStat> Systems { get; init; } = [];

    [JsonPropertyName("games")]
    public List<AchievementGameEntry> Games { get; init; } = [];

    [JsonPropertyName("refresh")]
    public AchievementRefreshInfo Refresh { get; init; } = new();
}

// ---------------------------------------------------------------------------
// Achievement explorer models (GET /api/achievements/explorer)
// ---------------------------------------------------------------------------

/// <summary>An individual achievement row returned by the explorer endpoint.</summary>
public sealed record AchievementDto
{
    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("description")]
    public string Description { get; init; } = string.Empty;

    [JsonPropertyName("locked_icon")]
    public string? LockedIcon { get; init; }

    [JsonPropertyName("unlocked_icon")]
    public string? UnlockedIcon { get; init; }

    [JsonPropertyName("points")]
    public int Points { get; init; }

    [JsonPropertyName("rarity")]
    public double Rarity { get; init; }

    [JsonPropertyName("unlocked")]
    public bool Unlocked { get; init; }

    [JsonPropertyName("unlocked_at")]
    public string? UnlockedAt { get; init; }
}

/// <summary>One achievement set from a single source/integration within a game.</summary>
public sealed record AchievementSetDto
{
    [JsonPropertyName("source")]
    public string Source { get; init; } = string.Empty;

    [JsonPropertyName("external_game_id")]
    public string ExternalGameId { get; init; } = string.Empty;

    [JsonPropertyName("source_game_id")]
    public string? SourceGameId { get; init; }

    [JsonPropertyName("source_title")]
    public string? SourceTitle { get; init; }

    [JsonPropertyName("platform")]
    public string? Platform { get; init; }

    [JsonPropertyName("integration_id")]
    public string? IntegrationId { get; init; }

    [JsonPropertyName("integration_label")]
    public string? IntegrationLabel { get; init; }

    [JsonPropertyName("plugin_id")]
    public string? PluginId { get; init; }

    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }

    [JsonPropertyName("total_points")]
    public int TotalPoints { get; init; }

    [JsonPropertyName("earned_points")]
    public int EarnedPoints { get; init; }

    [JsonPropertyName("achievements")]
    public List<AchievementDto> Achievements { get; init; } = [];
}

/// <summary>A game entry in the explorer — canonical game info plus all achievement sets.</summary>
public sealed record AchievementGameExplorerDto
{
    [JsonPropertyName("game")]
    public GameDetail Game { get; init; } = new();

    [JsonPropertyName("systems")]
    public List<AchievementSetDto> Systems { get; init; } = [];
}

/// <summary>Full response from GET /api/achievements/explorer.</summary>
public sealed record AchievementsExplorerResponse
{
    [JsonPropertyName("games")]
    public List<AchievementGameExplorerDto> Games { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Library / Gamer stats models
// ---------------------------------------------------------------------------

/// <summary>A key/label/count triplet used for breakdown rows (platforms, genres, etc.).</summary>
public sealed record CountStat
{
    [JsonPropertyName("key")]
    public string Key { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    [JsonPropertyName("count")]
    public int Count { get; init; }
}

/// <summary>Like CountStat but also carries a percentage for coverage rows.</summary>
public sealed record CoverageStat
{
    [JsonPropertyName("key")]
    public string Key { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    [JsonPropertyName("count")]
    public int Count { get; init; }

    [JsonPropertyName("percent")]
    public double Percent { get; init; }
}

/// <summary>Top-line summary inside LibraryStatistics.</summary>
public sealed record LibraryStatsSummary
{
    [JsonPropertyName("canonical_game_count")]
    public int CanonicalGameCount { get; init; }
}

/// <summary>Full response from GET /api/stats/library.</summary>
public sealed record LibraryStatistics
{
    [JsonPropertyName("summary")]
    public LibraryStatsSummary Summary { get; init; } = new();

    [JsonPropertyName("platforms")]
    public List<CountStat> Platforms { get; init; } = [];

    [JsonPropertyName("kinds")]
    public List<CountStat> Kinds { get; init; } = [];

    [JsonPropertyName("genres")]
    public List<CountStat> Genres { get; init; } = [];

    [JsonPropertyName("decades")]
    public List<CountStat> Decades { get; init; } = [];

    [JsonPropertyName("coverage")]
    public List<CoverageStat> Coverage { get; init; } = [];
}

/// <summary>A bucket of games grouped by achievement-completion percentage.</summary>
public sealed record AchievementCompletionBucket
{
    [JsonPropertyName("key")]
    public string Key { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    [JsonPropertyName("game_count")]
    public int GameCount { get; init; }
}

/// <summary>Per-source achievement stats inside GamerStatistics.</summary>
public sealed record AchievementSystem
{
    [JsonPropertyName("source")]
    public string Source { get; init; } = string.Empty;

    [JsonPropertyName("game_count")]
    public int GameCount { get; init; }

    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }
}

/// <summary>Full response from GET /api/stats/gamer.</summary>
public sealed record GamerStatistics
{
    [JsonPropertyName("total_games")]
    public int TotalGames { get; init; }

    [JsonPropertyName("favorite_games")]
    public int FavoriteGames { get; init; }

    [JsonPropertyName("total_achievements")]
    public int TotalAchievements { get; init; }

    [JsonPropertyName("unlocked_achievements")]
    public int UnlockedAchievements { get; init; }

    [JsonPropertyName("achievement_unlock_percent")]
    public double AchievementUnlockPercent { get; init; }

    [JsonPropertyName("achievement_systems")]
    public List<AchievementSystem> AchievementSystems { get; init; } = [];

    [JsonPropertyName("achievement_completion_buckets")]
    public List<AchievementCompletionBucket> AchievementCompletionBuckets { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Integrations
// ---------------------------------------------------------------------------

/// <summary>Live status entry from GET /api/integrations/status.</summary>
public sealed record IntegrationStatusEntry
{
    [JsonPropertyName("integration_id")]
    public string IntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    /// <summary>"ok", "error", "pending", etc.</summary>
    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("message")]
    public string Message { get; init; } = string.Empty;
}

/// <summary>Full integration record from GET /api/integrations or POST /api/integrations.</summary>
public sealed record IntegrationDto
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    /// <summary>Double-encoded JSON string, e.g. "{\"root_path\":\"/games\"}".</summary>
    [JsonPropertyName("config_json")]
    public string ConfigJson { get; init; } = string.Empty;

    [JsonPropertyName("integration_type")]
    public string IntegrationType { get; init; } = string.Empty;

    [JsonPropertyName("created_at")]
    public string? CreatedAt { get; init; }

    [JsonPropertyName("updated_at")]
    public string? UpdatedAt { get; init; }
}

/// <summary>
/// Returned as HTTP 202 when an OAuth flow is required to complete
/// creating or updating an integration.
/// </summary>
public sealed record OAuthRequiredResponse
{
    [JsonPropertyName("authorize_url")]
    public string AuthorizeUrl { get; init; } = string.Empty;

    [JsonPropertyName("state")]
    public string State { get; init; } = string.Empty;
}

/// <summary>Job status returned by POST /api/scan and GET /api/scan/jobs/{job_id}.</summary>
public sealed record ScanJobStatus
{
    [JsonPropertyName("job_id")]
    public string JobId { get; init; } = string.Empty;

    /// <summary>"pending", "running", "completed", "failed", "cancelled", "cancelling".</summary>
    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("integration_count")]
    public int IntegrationCount { get; init; }

    [JsonPropertyName("integrations_completed")]
    public int IntegrationsCompleted { get; init; }

    [JsonPropertyName("current_integration_label")]
    public string? CurrentIntegrationLabel { get; init; }

    [JsonPropertyName("error")]
    public string? Error { get; init; }
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

/// <summary>A single source-cache entry from GET /api/cache/entries.</summary>
public sealed record CacheEntryDto
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("canonical_title")]
    public string CanonicalTitle { get; init; } = string.Empty;

    [JsonPropertyName("source_title")]
    public string SourceTitle { get; init; } = string.Empty;

    [JsonPropertyName("integration_label")]
    public string IntegrationLabel { get; init; } = string.Empty;

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("size")]
    public long Size { get; init; }

    [JsonPropertyName("file_count")]
    public int FileCount { get; init; }
}

/// <summary>Wrapper from GET /api/cache/entries.</summary>
public sealed record CacheEntriesResponse
{
    [JsonPropertyName("entries")]
    public List<CacheEntryDto> Entries { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

/// <summary>Gamer profile from GET /api/profiles.</summary>
public sealed record Profile
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("display_name")]
    public string DisplayName { get; init; } = string.Empty;

    [JsonPropertyName("avatar_key")]
    public string? AvatarKey { get; init; }

    [JsonPropertyName("role")]
    public string Role { get; init; } = string.Empty;

    [JsonPropertyName("created_at")]
    public string? CreatedAt { get; init; }
}

// ---------------------------------------------------------------------------
// Plugins
// ---------------------------------------------------------------------------

/// <summary>A single server-side plugin returned by GET /api/plugins or GET /api/plugins/{id}.</summary>
public sealed record PluginDto
{
    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("plugin_version")]
    public string Version { get; init; } = string.Empty;

    [JsonPropertyName("provides")]
    public List<string> Provides { get; init; } = [];

    [JsonPropertyName("capabilities")]
    public List<string> Capabilities { get; init; } = [];

    /// <summary>
    /// Config schema map returned under the "config" key.
    /// Each key is a field name; value is a JsonElement with optional sub-keys:
    /// type, required, description, x-secret, x-help-url.
    /// </summary>
    [JsonPropertyName("config")]
    public Dictionary<string, JsonElement>? ConfigSchema { get; init; }
}

// ---------------------------------------------------------------------------
// Duplicates
// ---------------------------------------------------------------------------

/// <summary>
/// Minimal source-game record nested inside a duplicate source entry.
/// Carries the source game ID (needed to call the merge endpoint) and
/// integration metadata for display.
/// </summary>
public sealed record DuplicateSourceRecord
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("integration_id")]
    public string IntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("integration_label")]
    public string? IntegrationLabel { get; init; }

    [JsonPropertyName("raw_title")]
    public string RawTitle { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;
}

/// <summary>A single source entry within a duplicate group.</summary>
public sealed record DuplicateSourceDto
{
    [JsonPropertyName("canonical_game_id")]
    public string CanonicalGameId { get; init; } = string.Empty;

    [JsonPropertyName("canonical_title")]
    public string CanonicalTitle { get; init; } = string.Empty;

    /// <summary>The nested source-game record carrying the source game ID and integration info.</summary>
    [JsonPropertyName("source")]
    public DuplicateSourceRecord Source { get; init; } = new();

    [JsonPropertyName("file_count")]
    public int FileCount { get; init; }

    [JsonPropertyName("total_size")]
    public long TotalSize { get; init; }

    [JsonPropertyName("cached")]
    public bool Cached { get; init; }
}

/// <summary>A group of duplicate games returned by GET /api/duplicates/games.</summary>
public sealed record DuplicateGroupDto
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("mode")]
    public string Mode { get; init; } = string.Empty;

    [JsonPropertyName("representative_title")]
    public string RepresentativeTitle { get; init; } = string.Empty;

    [JsonPropertyName("canonical_ids")]
    public List<string> CanonicalIds { get; init; } = [];

    [JsonPropertyName("sources")]
    public List<DuplicateSourceDto> Sources { get; init; } = [];
}

/// <summary>Full response from GET /api/duplicates/games.</summary>
public sealed record DuplicateGamesResponse
{
    [JsonPropertyName("mode")]
    public string Mode { get; init; } = string.Empty;

    [JsonPropertyName("groups")]
    public List<DuplicateGroupDto> Groups { get; init; } = [];
}

// ---------------------------------------------------------------------------
// About / Version
// ---------------------------------------------------------------------------

/// <summary>Server build metadata from GET /api/about.</summary>
public sealed record AboutInfo
{
    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;

    [JsonPropertyName("commit")]
    public string Commit { get; init; } = string.Empty;

    [JsonPropertyName("build_date")]
    public string BuildDate { get; init; } = string.Empty;

    [JsonPropertyName("author_credits")]
    public List<string> AuthorCredits { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

/// <summary>Live update status from GET /api/update/status.</summary>
public sealed record UpdateStatus
{
    [JsonPropertyName("current_version")]
    public string CurrentVersion { get; init; } = string.Empty;

    [JsonPropertyName("latest_version")]
    public string? LatestVersion { get; init; }

    [JsonPropertyName("update_available")]
    public bool UpdateAvailable { get; init; }

    [JsonPropertyName("release_notes_url")]
    public string? ReleaseNotesUrl { get; init; }

    [JsonPropertyName("install_type")]
    public string InstallType { get; init; } = string.Empty;

    [JsonPropertyName("download_in_progress")]
    public bool DownloadInProgress { get; init; }

    [JsonPropertyName("download_percent")]
    public double DownloadPercent { get; init; }

    [JsonPropertyName("message")]
    public string? Message { get; init; }
}

// ---------------------------------------------------------------------------
// Manual Review / Undetected Games
// ---------------------------------------------------------------------------

/// <summary>Lightweight manual-review candidate for the list view.</summary>
public record ReviewCandidateSummary
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("canonical_game_id")]
    public string? CanonicalGameId { get; init; }

    [JsonPropertyName("current_title")]
    public string CurrentTitle { get; init; } = string.Empty;

    [JsonPropertyName("raw_title")]
    public string RawTitle { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("integration_id")]
    public string IntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("integration_label")]
    public string? IntegrationLabel { get; init; }

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("root_path")]
    public string? RootPath { get; init; }

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("review_state")]
    public string ReviewState { get; init; } = string.Empty;

    [JsonPropertyName("file_count")]
    public int FileCount { get; init; }

    [JsonPropertyName("resolver_match_count")]
    public int ResolverMatchCount { get; init; }

    [JsonPropertyName("review_reasons")]
    public List<string> ReviewReasons { get; init; } = [];

    [JsonPropertyName("created_at")]
    public string CreatedAt { get; init; } = string.Empty;
}

/// <summary>A single file attached to a review candidate.</summary>
public sealed record GameFileDto
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("path")]
    public string Path { get; init; } = string.Empty;

    [JsonPropertyName("role")]
    public string Role { get; init; } = string.Empty;

    [JsonPropertyName("file_kind")]
    public string? FileKind { get; init; }

    [JsonPropertyName("size")]
    public long Size { get; init; }
}

/// <summary>A single metadata resolver match for a review candidate.</summary>
public sealed record ResolverMatchDto
{
    [JsonPropertyName("provider")]
    public string Provider { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string? Url { get; init; }

    [JsonPropertyName("image_url")]
    public string? ImageUrl { get; init; }
}

/// <summary>Full review candidate detail including files and resolver matches.</summary>
public sealed record ReviewCandidateDetail : ReviewCandidateSummary
{
    [JsonPropertyName("url")]
    public string? Url { get; init; }

    [JsonPropertyName("files")]
    public List<GameFileDto> Files { get; init; } = [];

    [JsonPropertyName("resolver_matches")]
    public List<ResolverMatchDto> ResolverMatches { get; init; } = [];
}

/// <summary>A single metadata search result for a review candidate.</summary>
public sealed record ReviewSearchResultDto
{
    [JsonPropertyName("provider_integration_id")]
    public string ProviderIntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("provider_label")]
    public string? ProviderLabel { get; init; }

    [JsonPropertyName("provider_plugin_id")]
    public string ProviderPluginId { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string? Platform { get; init; }

    [JsonPropertyName("kind")]
    public string? Kind { get; init; }

    [JsonPropertyName("external_id")]
    public string ExternalId { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string? Url { get; init; }

    [JsonPropertyName("description")]
    public string? Description { get; init; }

    [JsonPropertyName("release_date")]
    public string? ReleaseDate { get; init; }

    [JsonPropertyName("image_url")]
    public string? ImageUrl { get; init; }
}

/// <summary>Response from POST /api/review-candidates/{id}/search.</summary>
public sealed record ReviewSearchResponse
{
    [JsonPropertyName("candidate_id")]
    public string CandidateId { get; init; } = string.Empty;

    [JsonPropertyName("query")]
    public string Query { get; init; } = string.Empty;

    [JsonPropertyName("results")]
    public List<ReviewSearchResultDto> Results { get; init; } = [];
}

/// <summary>Response from POST /api/review-candidates/{id}/redetect.</summary>
public sealed record ReviewRedetectResponse
{
    [JsonPropertyName("result")]
    public ReviewRedetectResult Result { get; init; } = new();

    [JsonPropertyName("candidate")]
    public ReviewCandidateDetail? Candidate { get; init; }
}

/// <summary>Single result entry within a redetect response.</summary>
public sealed record ReviewRedetectResult
{
    [JsonPropertyName("candidate_id")]
    public string CandidateId { get; init; } = string.Empty;

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("match_count")]
    public int MatchCount { get; init; }

    [JsonPropertyName("provider_count")]
    public int ProviderCount { get; init; }
}

/// <summary>Response from POST /api/review-candidates/redetect (batch).</summary>
public sealed record ReviewRedetectBatchResult
{
    [JsonPropertyName("attempted")]
    public int Attempted { get; init; }

    [JsonPropertyName("matched")]
    public int Matched { get; init; }

    [JsonPropertyName("unidentified")]
    public int Unidentified { get; init; }

    [JsonPropertyName("error")]
    public string? Error { get; init; }
}

/// <summary>Response from DELETE /api/review-candidates/{id}/files.</summary>
public sealed record ReviewDeleteFilesResponse
{
    [JsonPropertyName("deleted_candidate_id")]
    public string DeletedCandidateId { get; init; } = string.Empty;

    [JsonPropertyName("canonical_exists")]
    public bool CanonicalExists { get; init; }
}

// ---------------------------------------------------------------------------
// Integration games
// ---------------------------------------------------------------------------

/// <summary>Lightweight game reference from GET /api/integrations/{id}/games.</summary>
public sealed record GameListItem
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;
}

// ---------------------------------------------------------------------------
// Plugin browse
// ---------------------------------------------------------------------------

/// <summary>A single entry in a plugin file-browse response.</summary>
public sealed record BrowseEntry
{
    [JsonPropertyName("name")]
    public string Name { get; init; } = string.Empty;

    [JsonPropertyName("path")]
    public string Path { get; init; } = string.Empty;

    [JsonPropertyName("is_dir")]
    public bool IsDir { get; init; }

    [JsonPropertyName("size")]
    public long Size { get; init; }
}

/// <summary>Response from POST /api/plugins/{plugin_id}/browse.</summary>
public sealed record PluginBrowseResponse
{
    [JsonPropertyName("path")]
    public string Path { get; init; } = string.Empty;

    [JsonPropertyName("entries")]
    public List<BrowseEntry> Entries { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Scan reports
// ---------------------------------------------------------------------------

/// <summary>Per-integration breakdown within a scan report.</summary>
public sealed record ScanIntegrationResult
{
    [JsonPropertyName("integration_id")]
    public string IntegrationId { get; init; } = string.Empty;

    [JsonPropertyName("label")]
    public string Label { get; init; } = string.Empty;

    [JsonPropertyName("plugin_id")]
    public string PluginId { get; init; } = string.Empty;

    [JsonPropertyName("games_found")]
    public int GamesFound { get; init; }

    [JsonPropertyName("games_added")]
    public int GamesAdded { get; init; }

    [JsonPropertyName("games_removed")]
    public int GamesRemoved { get; init; }

    [JsonPropertyName("error")]
    public string? Error { get; init; }
}

/// <summary>A completed scan report from GET /api/scan/reports.</summary>
public sealed record ScanReport
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("started_at")]
    public string StartedAt { get; init; } = string.Empty;

    [JsonPropertyName("finished_at")]
    public string FinishedAt { get; init; } = string.Empty;

    [JsonPropertyName("duration_ms")]
    public long DurationMs { get; init; }

    [JsonPropertyName("metadata_only")]
    public bool MetadataOnly { get; init; }

    [JsonPropertyName("games_added")]
    public int GamesAdded { get; init; }

    [JsonPropertyName("games_removed")]
    public int GamesRemoved { get; init; }

    [JsonPropertyName("games_updated")]
    public int GamesUpdated { get; init; }

    [JsonPropertyName("total_games")]
    public int TotalGames { get; init; }

    [JsonPropertyName("integration_results")]
    public List<ScanIntegrationResult> IntegrationResults { get; init; } = [];
}
