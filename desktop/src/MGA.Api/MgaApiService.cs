using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Api;

/// <summary>
/// Facade over the generated MGA REST API client.
///
/// All methods throw MgaApiException on non-2xx responses — never return null
/// or swallow errors silently (fail-fast policy).
///
/// The HttpClient lifetime is owned by ServerConnectionService (RAII).
/// This class is a thin stateless wrapper — safe to re-create when switching servers.
///
/// TODO: after running scripts/generate-api-client.ps1, replace the stub methods
///       below with calls to the generated MgaApiClient.
/// </summary>
public sealed class MgaApiService
{
    private readonly HttpClient _http;

    internal static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        NumberHandling = JsonNumberHandling.AllowReadingFromString,
    };

    public MgaApiService(HttpClient http)
    {
        _http = http;
    }

    // ---------------------------------------------------------------------------
    // Health
    // ---------------------------------------------------------------------------

    /// <summary>Returns true when the server responds with HTTP 200 on /health.</summary>
    public async Task<bool> PingAsync(CancellationToken ct = default)
    {
        try
        {
            var resp = await _http.GetAsync("/health", ct).ConfigureAwait(false);
            return resp.IsSuccessStatusCode;
        }
        catch
        {
            return false;
        }
    }

    // ---------------------------------------------------------------------------
    // Internal helpers
    // ---------------------------------------------------------------------------

    private async Task<T> GetAsync<T>(string path, CancellationToken ct = default)
    {
        var resp = await _http.GetAsync(path, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<T>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, $"Server returned null for {path}");
    }

    private async Task EnsureSuccess(HttpResponseMessage resp, CancellationToken ct)
    {
        if (resp.IsSuccessStatusCode)
            return;

        // Try to extract a structured error body.
        string? errorCode = null;
        string message;

        try
        {
            var body = await resp.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            using var doc = JsonDocument.Parse(body);

            errorCode = doc.RootElement.TryGetProperty("code", out var codeEl)
                ? codeEl.GetString()
                : null;

            message = doc.RootElement.TryGetProperty("message", out var msgEl)
                ? msgEl.GetString() ?? resp.ReasonPhrase ?? "unknown error"
                : body;
        }
        catch
        {
            message = resp.ReasonPhrase ?? "unknown error";
        }

        throw new MgaApiException((int)resp.StatusCode, message, errorCode);
    }

    // ---------------------------------------------------------------------------
    // Games
    // ---------------------------------------------------------------------------

    /// <summary>Returns a paginated list of games from the server.</summary>
    public Task<ListGamesResponse> ListGamesAsync(
        int page = 0,
        int pageSize = 100,
        CancellationToken ct = default)
        => GetAsync<ListGamesResponse>($"/api/games?page={page}&page_size={pageSize}", ct);

    /// <summary>Returns the full detail for a single game.</summary>
    public Task<GameDetail> GetGameAsync(string id, CancellationToken ct = default)
        => GetAsync<GameDetail>($"/api/games/{Uri.EscapeDataString(id)}", ct);

    /// <summary>
    /// Triggers an online metadata refresh for a single game via
    /// POST /api/games/{id}/refresh-metadata.
    /// Returns the updated <see cref="GameDetail"/> on success.
    /// Throws <see cref="MgaApiException"/> with status 409 when no eligible provider
    /// exists, or 422 when metadata providers are unavailable.
    /// </summary>
    public Task<GameDetail> RefreshGameMetadataAsync(string id, CancellationToken ct = default)
        => PostAsync<GameDetail>($"/api/games/{Uri.EscapeDataString(id)}/refresh-metadata", ct);

    /// <summary>
    /// Hard-deletes a source-game record (and its files) via
    /// DELETE /api/games/{id}/sources/{sourceGameId}.
    /// </summary>
    public Task<DeleteSourceGameResponse> DeleteSourceGameAsync(
        string gameId, string sourceGameId, CancellationToken ct = default)
        => DeleteAsync<DeleteSourceGameResponse>(
            $"/api/games/{Uri.EscapeDataString(gameId)}/sources/{Uri.EscapeDataString(sourceGameId)}", ct);

    /// <summary>
    /// Removes the canonical pin that forced this source to map to a specific game via
    /// DELETE /api/games/{id}/sources/{sourceGameId}/canonical-pin.
    /// </summary>
    public Task<CanonicalGroupingResponse> ClearCanonicalPinAsync(
        string gameId, string sourceGameId, CancellationToken ct = default)
        => DeleteAsync<CanonicalGroupingResponse>(
            $"/api/games/{Uri.EscapeDataString(gameId)}/sources/{Uri.EscapeDataString(sourceGameId)}/canonical-pin", ct);

    /// <summary>
    /// Splits a source-game into its own canonical entry via
    /// POST /api/games/{id}/sources/{sourceGameId}/canonical/split.
    /// </summary>
    public Task<CanonicalGroupingResponse> SplitSourceGameAsync(
        string gameId, string sourceGameId, CancellationToken ct = default)
        => PostAsync<CanonicalGroupingResponse>(
            $"/api/games/{Uri.EscapeDataString(gameId)}/sources/{Uri.EscapeDataString(sourceGameId)}/canonical/split", ct);

    /// <summary>
    /// Merges a source game into a different canonical entry via
    /// POST /api/games/{id}/sources/{sourceGameId}/canonical/merge.
    /// The source is detached from the current canonical and re-pinned to the target.
    /// </summary>
    public async Task<CanonicalGroupingResponse> MergeSourceGameAsync(
        string gameId, string sourceGameId, string targetCanonicalGameId, CancellationToken ct = default)
    {
        var body = new { target_canonical_game_id = targetCanonicalGameId };
        var resp = await _http.PostAsJsonAsync(
            $"/api/games/{Uri.EscapeDataString(gameId)}/sources/{Uri.EscapeDataString(sourceGameId)}/canonical/merge",
            body, JsonOptions, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<CanonicalGroupingResponse>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, "Server returned null for merge source");
    }

    /// <summary>
    /// Searches canonical games by title query via GET /api/canonical-games/search.
    /// Returns up to <paramref name="limit"/> results (default 20).
    /// </summary>
    public Task<CanonicalGameSearchResponse> SearchCanonicalGamesAsync(
        string query, int limit = 20, CancellationToken ct = default)
        => GetAsync<CanonicalGameSearchResponse>(
            $"/api/canonical-games/search?q={Uri.EscapeDataString(query)}&limit={limit}", ct);

    // ---------------------------------------------------------------------------
    // Stats
    // ---------------------------------------------------------------------------

    /// <summary>Returns top-level library statistics.</summary>
    public Task<LibraryStats> GetLibraryStatsAsync(CancellationToken ct = default)
        => GetAsync<LibraryStats>("/api/stats", ct);

    /// <summary>Returns detailed library statistics from GET /api/stats/library.</summary>
    public Task<LibraryStatistics> GetLibraryStatisticsAsync(CancellationToken ct = default)
        => GetAsync<LibraryStatistics>("/api/stats/library", ct);

    /// <summary>Returns gamer-profile statistics from GET /api/stats/gamer.</summary>
    public Task<GamerStatistics> GetGamerStatisticsAsync(CancellationToken ct = default)
        => GetAsync<GamerStatistics>("/api/stats/gamer", ct);

    // ---------------------------------------------------------------------------
    // Achievements
    // ---------------------------------------------------------------------------

    /// <summary>Returns the full achievements dashboard from GET /api/achievements.</summary>
    public Task<AchievementsDashboard> GetAchievementsDashboardAsync(CancellationToken ct = default)
        => GetAsync<AchievementsDashboard>("/api/achievements", ct);

    /// <summary>
    /// Returns stored achievement sets grouped by canonical game from GET /api/achievements/explorer.
    /// Read-only; does not trigger provider fetches.
    /// </summary>
    public Task<AchievementsExplorerResponse> GetAchievementsExplorerAsync(CancellationToken ct = default)
        => GetAsync<AchievementsExplorerResponse>("/api/achievements/explorer", ct);

    /// <summary>
    /// Posts to /api/achievements/refresh to start a background refresh job (returns 202).
    /// Throws MgaApiException for any non-2xx response other than 202.
    /// </summary>
    public async Task StartAchievementsRefreshAsync(CancellationToken ct = default)
    {
        var resp = await _http.PostAsync("/api/achievements/refresh", null, ct).ConfigureAwait(false);

        // 202 Accepted is the expected success code for async jobs.
        if ((int)resp.StatusCode != 202 && !resp.IsSuccessStatusCode)
            await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Favorites
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Sets or clears the favorite flag for a game.
    /// PUT /api/games/{id}/favorite to set; DELETE to clear.
    /// </summary>
    public async Task SetFavoriteAsync(string id, bool favorite, CancellationToken ct = default)
    {
        var path = $"/api/games/{Uri.EscapeDataString(id)}/favorite";

        var resp = favorite
            ? await _http.PutAsync(path, null, ct).ConfigureAwait(false)
            : await _http.DeleteAsync(path, ct).ConfigureAwait(false);

        await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Profiles
    // ---------------------------------------------------------------------------

    /// <summary>Returns all gamer profiles configured on the server.</summary>
    public Task<List<Profile>> GetProfilesAsync(CancellationToken ct = default)
        => GetAsync<List<Profile>>("/api/profiles", ct);

    // ---------------------------------------------------------------------------
    // Scan
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Triggers a full library scan via POST /api/scan.
    /// The server starts the job and fires SSE events (scan_started, scan_complete, scan_error).
    /// </summary>
    public async Task TriggerScanAsync(CancellationToken ct = default)
    {
        // Empty body = scan all sources.
        var resp = await _http.PostAsJsonAsync("/api/scan", new { }, JsonOptions, ct)
                              .ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Integrations
    // ---------------------------------------------------------------------------

    /// <summary>Returns live status for all configured integrations.</summary>
    public Task<List<IntegrationStatusEntry>> GetIntegrationStatusAsync(CancellationToken ct = default)
        => GetAsync<List<IntegrationStatusEntry>>("/api/integrations/status", ct);

    /// <summary>Returns the full list of configured integrations from GET /api/integrations.</summary>
    public Task<List<IntegrationDto>> ListIntegrationsAsync(CancellationToken ct = default)
        => GetAsync<List<IntegrationDto>>("/api/integrations", ct);

    /// <summary>
    /// Creates a new integration via POST /api/integrations.
    /// Returns either (IntegrationDto, null) on HTTP 201 or (null, OAuthRequiredResponse) on HTTP 202.
    /// </summary>
    public async Task<(IntegrationDto? Integration, OAuthRequiredResponse? OAuth)> CreateIntegrationAsync(
        string pluginId,
        string label,
        string integrationType,
        Dictionary<string, object> config,
        CancellationToken ct = default)
    {
        var body = new { plugin_id = pluginId, label, integration_type = integrationType, config };
        var resp = await _http.PostAsJsonAsync("/api/integrations", body, JsonOptions, ct).ConfigureAwait(false);

        if ((int)resp.StatusCode == 202)
        {
            var oauth = await resp.Content.ReadFromJsonAsync<OAuthRequiredResponse>(JsonOptions, ct).ConfigureAwait(false);
            return (null, oauth ?? throw new MgaApiException(202, "Server returned null OAuth response"));
        }

        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var dto = await resp.Content.ReadFromJsonAsync<IntegrationDto>(JsonOptions, ct).ConfigureAwait(false);
        return (dto ?? throw new MgaApiException(201, "Server returned null integration"), null);
    }

    /// <summary>
    /// Updates an existing integration via PUT /api/integrations/{id}.
    /// Returns either (IntegrationDto, null) on HTTP 200 or (null, OAuthRequiredResponse) on HTTP 202.
    /// </summary>
    public async Task<(IntegrationDto? Integration, OAuthRequiredResponse? OAuth)> UpdateIntegrationAsync(
        string id,
        string label,
        string integrationType,
        Dictionary<string, object> config,
        CancellationToken ct = default)
    {
        var body = new { label, integration_type = integrationType, config };
        var resp = await _http.PutAsJsonAsync(
            $"/api/integrations/{Uri.EscapeDataString(id)}", body, JsonOptions, ct).ConfigureAwait(false);

        if ((int)resp.StatusCode == 202)
        {
            var oauth = await resp.Content.ReadFromJsonAsync<OAuthRequiredResponse>(JsonOptions, ct).ConfigureAwait(false);
            return (null, oauth ?? throw new MgaApiException(202, "Server returned null OAuth response"));
        }

        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var dto = await resp.Content.ReadFromJsonAsync<IntegrationDto>(JsonOptions, ct).ConfigureAwait(false);
        return (dto ?? throw new MgaApiException(200, "Server returned null integration"), null);
    }

    /// <summary>Deletes an integration via DELETE /api/integrations/{id} (expects 204).</summary>
    public async Task DeleteIntegrationAsync(string id, CancellationToken ct = default)
    {
        var resp = await _http.DeleteAsync(
            $"/api/integrations/{Uri.EscapeDataString(id)}", ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    /// <summary>Returns a single plugin with its config schema from GET /api/plugins/{plugin_id}.</summary>
    public Task<PluginDto> GetPluginAsync(string pluginId, CancellationToken ct = default)
        => GetAsync<PluginDto>($"/api/plugins/{Uri.EscapeDataString(pluginId)}", ct);

    /// <summary>
    /// Authorizes an integration via POST /api/integrations/{id}/authorize.
    /// Returns either (IntegrationStatusEntry, null) on HTTP 200 or (null, OAuthRequiredResponse) on HTTP 202.
    /// </summary>
    public async Task<(IntegrationStatusEntry? Status, OAuthRequiredResponse? OAuth)> AuthorizeIntegrationAsync(
        string id, CancellationToken ct = default)
    {
        var resp = await _http.PostAsync(
            $"/api/integrations/{Uri.EscapeDataString(id)}/authorize", null, ct).ConfigureAwait(false);

        if ((int)resp.StatusCode == 202)
        {
            var oauth = await resp.Content.ReadFromJsonAsync<OAuthRequiredResponse>(JsonOptions, ct).ConfigureAwait(false);
            return (null, oauth ?? throw new MgaApiException(202, "Server returned null OAuth response"));
        }

        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var status = await resp.Content.ReadFromJsonAsync<IntegrationStatusEntry>(JsonOptions, ct).ConfigureAwait(false);
        return (status ?? throw new MgaApiException(200, "Server returned null status"), null);
    }

    /// <summary>Triggers a background refresh for integration {id} via POST /api/integrations/{id}/refresh.</summary>
    public async Task RefreshIntegrationAsync(string id, CancellationToken ct = default)
    {
        var resp = await _http.PostAsync(
            $"/api/integrations/{Uri.EscapeDataString(id)}/refresh",
            null, ct).ConfigureAwait(false);

        // 202 Accepted is the normal success code for async jobs.
        if ((int)resp.StatusCode != 202 && !resp.IsSuccessStatusCode)
            await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Scan jobs
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Starts a scan job via POST /api/scan (returns HTTP 202 with ScanJobStatus).
    /// Pass integrationIds to scan specific integrations; null/empty scans all.
    /// </summary>
    public async Task<ScanJobStatus> StartScanAsync(
        IEnumerable<string>? integrationIds = null,
        bool metadataOnly = false,
        CancellationToken ct = default)
    {
        var ids = integrationIds?.ToList();
        object body = (ids is { Count: > 0 })
            ? new { game_sources = ids, metadata_only = metadataOnly }
            : new { metadata_only = metadataOnly };

        var resp = await _http.PostAsJsonAsync("/api/scan", body, JsonOptions, ct).ConfigureAwait(false);

        // 202 is the expected response for a successfully queued scan.
        if ((int)resp.StatusCode != 202 && !resp.IsSuccessStatusCode)
            await EnsureSuccess(resp, ct).ConfigureAwait(false);

        var result = await resp.Content.ReadFromJsonAsync<ScanJobStatus>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(202, "Server returned null scan job status");
    }

    /// <summary>Returns the current status of a scan job from GET /api/scan/jobs/{job_id}.</summary>
    public Task<ScanJobStatus> GetScanJobAsync(string jobId, CancellationToken ct = default)
        => GetAsync<ScanJobStatus>($"/api/scan/jobs/{Uri.EscapeDataString(jobId)}", ct);

    /// <summary>Cancels a running scan job via POST /api/scan/jobs/{job_id}/cancel.</summary>
    public async Task<ScanJobStatus> CancelScanJobAsync(string jobId, CancellationToken ct = default)
    {
        var resp = await _http.PostAsync(
            $"/api/scan/jobs/{Uri.EscapeDataString(jobId)}/cancel", null, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<ScanJobStatus>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, "Server returned null scan job status");
    }

    // ---------------------------------------------------------------------------
    // Cache
    // ---------------------------------------------------------------------------

    /// <summary>Returns all source-cache entries from GET /api/cache/entries.</summary>
    public Task<CacheEntriesResponse> GetCacheEntriesAsync(CancellationToken ct = default)
        => GetAsync<CacheEntriesResponse>("/api/cache/entries", ct);

    /// <summary>Clears all source-cache entries via POST /api/cache/clear.</summary>
    public async Task ClearCacheAsync(CancellationToken ct = default)
    {
        var resp = await _http.PostAsync("/api/cache/clear", null, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    /// <summary>Triggers cache preparation for one game via POST /api/games/{id}/cache/prepare.</summary>
    public async Task PrepareCacheAsync(string gameId, CancellationToken ct = default)
    {
        var resp = await _http.PostAsync(
            $"/api/games/{Uri.EscapeDataString(gameId)}/cache/prepare", null, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Plugins
    // ---------------------------------------------------------------------------

    /// <summary>Returns all server-side plugins from GET /api/plugins.</summary>
    public Task<List<PluginDto>> GetPluginsAsync(CancellationToken ct = default)
        => GetAsync<List<PluginDto>>("/api/plugins", ct);

    // ---------------------------------------------------------------------------
    // Duplicates
    // ---------------------------------------------------------------------------

    /// <summary>Returns duplicate-game groups from GET /api/duplicates/games.</summary>
    /// <param name="mode">"loose" (default) or "strict".</param>
    public Task<DuplicateGamesResponse> GetDuplicatesAsync(
        string mode = "loose", CancellationToken ct = default)
        => GetAsync<DuplicateGamesResponse>($"/api/duplicates/games?mode={Uri.EscapeDataString(mode)}", ct);

    // ---------------------------------------------------------------------------
    // About / Version
    // ---------------------------------------------------------------------------

    /// <summary>Returns server build metadata from GET /api/about.</summary>
    public Task<AboutInfo> GetAboutInfoAsync(CancellationToken ct = default)
        => GetAsync<AboutInfo>("/api/about", ct);

    // ---------------------------------------------------------------------------
    // Update
    // ---------------------------------------------------------------------------

    /// <summary>Returns the current update status from GET /api/update/status.</summary>
    public Task<UpdateStatus> GetUpdateStatusAsync(CancellationToken ct = default)
        => GetAsync<UpdateStatus>("/api/update/status", ct);

    /// <summary>
    /// Triggers an update check via POST /api/update/check (returns 200 with UpdateStatus).
    /// </summary>
    public Task<UpdateStatus> CheckForUpdatesAsync(CancellationToken ct = default)
        => PostAsync<UpdateStatus>("/api/update/check", ct);

    // ---------------------------------------------------------------------------
    // Internal helpers (extended)
    // ---------------------------------------------------------------------------

    private async Task<T> PostAsync<T>(string path, CancellationToken ct = default)
    {
        var resp = await _http.PostAsync(path, null, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<T>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, $"Server returned null for {path}");
    }

    private async Task<T> DeleteAsync<T>(string path, CancellationToken ct = default)
    {
        var resp = await _http.DeleteAsync(path, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<T>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, $"Server returned null for DELETE {path}");
    }

    // ---------------------------------------------------------------------------
    // Media
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Converts a relative media path (e.g. "/api/media/123") to an absolute URL
    /// using the HttpClient's base address.
    /// </summary>
    public string GetMediaUrl(string relativeUrl)
    {
        var baseAddress = _http.BaseAddress?.ToString().TrimEnd('/') ?? string.Empty;
        return baseAddress + relativeUrl;
    }

    // ---------------------------------------------------------------------------
    // Manual Review / Undetected Games
    // ---------------------------------------------------------------------------

    /// <summary>Lists manual-review candidates from GET /api/review-candidates.</summary>
    public Task<List<ReviewCandidateSummary>> ListReviewCandidatesAsync(
        string scope = "active", int? limit = null, CancellationToken ct = default)
    {
        var url = $"/api/review-candidates?scope={Uri.EscapeDataString(scope)}";
        if (limit.HasValue) url += $"&limit={limit.Value}";
        return GetAsync<List<ReviewCandidateSummary>>(url, ct);
    }

    /// <summary>Returns full detail for one review candidate from GET /api/review-candidates/{id}.</summary>
    public Task<ReviewCandidateDetail> GetReviewCandidateAsync(string id, CancellationToken ct = default)
        => GetAsync<ReviewCandidateDetail>($"/api/review-candidates/{Uri.EscapeDataString(id)}", ct);

    /// <summary>Batch re-detects all pending candidates via POST /api/review-candidates/redetect.</summary>
    public Task<ReviewRedetectBatchResult> RedetectAllCandidatesAsync(CancellationToken ct = default)
        => PostAsync<ReviewRedetectBatchResult>("/api/review-candidates/redetect", ct);

    /// <summary>Re-detects one candidate via POST /api/review-candidates/{id}/redetect.</summary>
    public Task<ReviewRedetectResponse> RedetectCandidateAsync(string id, CancellationToken ct = default)
        => PostAsync<ReviewRedetectResponse>($"/api/review-candidates/{Uri.EscapeDataString(id)}/redetect", ct);

    /// <summary>Searches metadata providers for a candidate via POST /api/review-candidates/{id}/search.</summary>
    public async Task<ReviewSearchResponse> SearchCandidateAsync(
        string id, string? query = null, CancellationToken ct = default)
    {
        var body = query is not null ? new { query } : (object)new { };
        var resp = await _http.PostAsJsonAsync(
            $"/api/review-candidates/{Uri.EscapeDataString(id)}/search", body, JsonOptions, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<ReviewSearchResponse>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, "Server returned null search response");
    }

    /// <summary>Applies a search result to a candidate via POST /api/review-candidates/{id}/apply.</summary>
    public async Task<ReviewCandidateDetail> ApplyCandidateMatchAsync(
        string id, ReviewSearchResultDto result, CancellationToken ct = default)
    {
        var body = new
        {
            provider_integration_id = result.ProviderIntegrationId,
            provider_plugin_id      = result.ProviderPluginId,
            external_id             = result.ExternalId,
            title                   = result.Title,
        };
        var resp = await _http.PostAsJsonAsync(
            $"/api/review-candidates/{Uri.EscapeDataString(id)}/apply", body, JsonOptions, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var detail = await resp.Content.ReadFromJsonAsync<ReviewCandidateDetail>(JsonOptions, ct).ConfigureAwait(false);
        return detail ?? throw new MgaApiException(200, "Server returned null candidate detail");
    }

    /// <summary>Archives a candidate as "not a game" via POST /api/review-candidates/{id}/not-a-game.</summary>
    public Task<ReviewCandidateDetail> MarkCandidateNotGameAsync(string id, CancellationToken ct = default)
        => PostAsync<ReviewCandidateDetail>($"/api/review-candidates/{Uri.EscapeDataString(id)}/not-a-game", ct);

    /// <summary>Archives a candidate as DLC via POST /api/review-candidates/{id}/dlc.</summary>
    public Task<ReviewCandidateDetail> MarkCandidateDlcAsync(string id, CancellationToken ct = default)
        => PostAsync<ReviewCandidateDetail>($"/api/review-candidates/{Uri.EscapeDataString(id)}/dlc", ct);

    /// <summary>Marks a candidate as base game (not DLC) via POST /api/review-candidates/{id}/base-game.</summary>
    public Task<ReviewCandidateDetail> MarkCandidateBaseGameAsync(string id, CancellationToken ct = default)
        => PostAsync<ReviewCandidateDetail>($"/api/review-candidates/{Uri.EscapeDataString(id)}/base-game", ct);

    /// <summary>Unarchives a candidate to active queue via POST /api/review-candidates/{id}/unarchive.</summary>
    public Task<ReviewCandidateDetail> UnarchiveCandidateAsync(string id, CancellationToken ct = default)
        => PostAsync<ReviewCandidateDetail>($"/api/review-candidates/{Uri.EscapeDataString(id)}/unarchive", ct);

    /// <summary>Deletes a candidate's backing files via DELETE /api/review-candidates/{id}/files.</summary>
    public async Task<ReviewDeleteFilesResponse> DeleteCandidateFilesAsync(string id, CancellationToken ct = default)
    {
        var resp = await _http.DeleteAsync(
            $"/api/review-candidates/{Uri.EscapeDataString(id)}/files", ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<ReviewDeleteFilesResponse>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, "Server returned null delete response");
    }

    // ---------------------------------------------------------------------------
    // Integration games
    // ---------------------------------------------------------------------------

    /// <summary>Returns games linked to a source integration from GET /api/integrations/{id}/games.</summary>
    public Task<List<GameListItem>> GetIntegrationGamesAsync(string id, CancellationToken ct = default)
        => GetAsync<List<GameListItem>>($"/api/integrations/{Uri.EscapeDataString(id)}/games", ct);

    // ---------------------------------------------------------------------------
    // Scan reports
    // ---------------------------------------------------------------------------

    /// <summary>Returns recent scan reports from GET /api/scan/reports.</summary>
    public Task<List<ScanReport>> ListScanReportsAsync(int? limit = null, CancellationToken ct = default)
    {
        var url = "/api/scan/reports";
        if (limit.HasValue) url += $"?limit={limit.Value}";
        return GetAsync<List<ScanReport>>(url, ct);
    }

    // ---------------------------------------------------------------------------
    // Plugin browse
    // ---------------------------------------------------------------------------

    /// <summary>Browses a plugin's file system via POST /api/plugins/{plugin_id}/browse.</summary>
    public async Task<PluginBrowseResponse> BrowsePluginPathAsync(
        string pluginId, string path, CancellationToken ct = default)
    {
        var body = new { path };
        var resp = await _http.PostAsJsonAsync(
            $"/api/plugins/{Uri.EscapeDataString(pluginId)}/browse", body, JsonOptions, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<PluginBrowseResponse>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, "Server returned null browse response");
    }
}
