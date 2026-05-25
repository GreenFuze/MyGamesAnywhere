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

    // ---------------------------------------------------------------------------
    // Plugins
    // ---------------------------------------------------------------------------

    /// <summary>Returns all server-side plugins from GET /api/plugins.</summary>
    public Task<List<PluginDto>> GetPluginsAsync(CancellationToken ct = default)
        => GetAsync<List<PluginDto>>("/api/plugins", ct);

    // ---------------------------------------------------------------------------
    // Duplicates
    // ---------------------------------------------------------------------------

    /// <summary>Returns all duplicate-game groups from GET /api/duplicates/games.</summary>
    public Task<DuplicateGamesResponse> GetDuplicatesAsync(CancellationToken ct = default)
        => GetAsync<DuplicateGamesResponse>("/api/duplicates/games", ct);

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
}
