using System.Text.Json;
using MGA.Api;

namespace MGA.Desktop.Services;

/// <summary>
/// Two-tier cache for the full game list returned by <c>ListGamesAsync</c>.
///
/// Tier 1 — In-memory: serves the current session. Stale after 3 minutes; a
///   silent background refresh keeps it current while the user navigates.
///
/// Tier 2 — Disk: persists across app restarts so Play/Library are instant on
///   every launch, not just after the first navigation of a session.
///   Cache file: <c>%APPDATA%\MGA\game-cache.json</c>.  Stale after 1 hour.
///
/// Thread-safety: <see cref="TryGet"/> and <see cref="Update"/> must be called
/// from the UI thread.  The disk I/O in <see cref="Update"/> is fire-and-forget
/// on a background thread — safe because we only write, never read, from there.
/// </summary>
public sealed class GameCacheService
{
    // -------------------------------------------------------------------------
    // Staleness windows
    // -------------------------------------------------------------------------

    private static readonly TimeSpan MemoryStaleness = TimeSpan.FromMinutes(3);

    // Disk cache survives for 24 h — games rarely change; a background refresh
    // always runs after the disk hit to pull the latest data silently.
    private static readonly TimeSpan DiskStaleness = TimeSpan.FromHours(24);

    // -------------------------------------------------------------------------
    // Memory tier
    // -------------------------------------------------------------------------

    private List<GameDetail>? _cached;
    private string?           _serverUrl;
    private DateTime          _fetchedAt = DateTime.MinValue;

    // -------------------------------------------------------------------------
    // Disk tier
    // -------------------------------------------------------------------------

    private static string CacheFilePath =>
        Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
            "MGA", "game-cache.json");

    private static readonly JsonSerializerOptions JsonOpts =
        new() { WriteIndented = false };

    // -------------------------------------------------------------------------
    // Constructor — eagerly warm memory tier from disk
    // -------------------------------------------------------------------------

    public GameCacheService()
    {
        // Load from disk synchronously in the constructor.
        // The file is local so this is typically <5 ms even for 500+ games.
        LoadFromDisk();
    }

    // -------------------------------------------------------------------------
    // Public API (UI-thread only)
    // -------------------------------------------------------------------------

    /// <summary>
    /// Returns true and the cached game list when a fresh entry exists for
    /// <paramref name="serverUrl"/>.  Returns false when the cache is empty,
    /// stale, or belongs to a different server.
    /// </summary>
    public bool TryGet(string serverUrl, out IReadOnlyList<GameDetail> games)
    {
        if (_cached is not null
            && _serverUrl == serverUrl
            && DateTime.UtcNow - _fetchedAt < MemoryStaleness)
        {
            games = _cached;
            return true;
        }

        games = [];
        return false;
    }

    /// <summary>
    /// Replaces the in-memory cache and asynchronously persists it to disk.
    /// Must be called from the UI thread.
    /// </summary>
    public void Update(string serverUrl, List<GameDetail> games)
    {
        _cached    = games;
        _serverUrl = serverUrl;
        _fetchedAt = DateTime.UtcNow;

        // Persist to disk on a pool thread — fire-and-forget.
        _ = SaveToDiskAsync(serverUrl, _fetchedAt, games);
    }

    /// <summary>
    /// Invalidates the in-memory cache so the next navigation triggers a full
    /// re-fetch.  Does NOT touch the disk cache (Background refresh will
    /// overwrite it once the new data arrives).
    /// </summary>
    public void Invalidate() => _cached = null;

    // -------------------------------------------------------------------------
    // Disk helpers
    // -------------------------------------------------------------------------

    private void LoadFromDisk()
    {
        try
        {
            var path = CacheFilePath;
            if (!File.Exists(path))
                return;

            var json    = File.ReadAllText(path);
            var payload = JsonSerializer.Deserialize<DiskPayload>(json, JsonOpts);
            if (payload is null)
                return;

            // Reject stale disk cache — a background refresh will replace it.
            if (DateTime.UtcNow - payload.CachedAt > DiskStaleness)
                return;

            _serverUrl = payload.ServerUrl;
            _cached    = payload.Games;
            // Stamp as "just loaded" so the 3-minute memory window starts fresh.
            // The disk's CachedAt is only used for the disk-staleness check above.
            _fetchedAt = DateTime.UtcNow;
        }
        catch
        {
            // Corrupt or unreadable cache — silently discard.
            // The app will cold-load from the server on first navigation.
        }
    }

    private static async Task SaveToDiskAsync(
        string serverUrl, DateTime cachedAt, List<GameDetail> games)
    {
        try
        {
            var path = CacheFilePath;
            var dir  = Path.GetDirectoryName(path)!;
            Directory.CreateDirectory(dir);

            var payload = new DiskPayload(serverUrl, cachedAt, games);
            var json    = JsonSerializer.Serialize(payload, JsonOpts);

            // Write to a temp file then atomic-rename to avoid corrupt reads
            // if the app is killed mid-write.
            var tmp = path + ".tmp";
            await File.WriteAllTextAsync(tmp, json).ConfigureAwait(false);
            File.Move(tmp, path, overwrite: true);
        }
        catch
        {
            // Disk write failed (permissions, full disk, etc.) — silently ignore.
            // The app works fine without the persistent cache.
        }
    }

    // -------------------------------------------------------------------------
    // Serialization shape
    // -------------------------------------------------------------------------

    /// <summary>Wire format stored in <c>game-cache.json</c>.</summary>
    private sealed record DiskPayload(
        string          ServerUrl,
        DateTime        CachedAt,
        List<GameDetail> Games);
}
