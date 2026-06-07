using System.Security.Cryptography;
using System.Text;

namespace MGA.Desktop.Services;

/// <summary>
/// Caches remote media URLs (cover art, screenshots) to local disk so the UI
/// can serve images instantly from the file system on subsequent launches.
///
/// Strategy:
///   - <see cref="GetLocalOrRemoteUrl"/> returns the local path if the image is
///     already cached; otherwise it returns the remote URL for lazy loading and
///     schedules a background download.
///   - <see cref="WarmAsync"/> proactively downloads a batch of URLs so future
///     calls to <see cref="GetLocalOrRemoteUrl"/> always hit the local cache.
///
/// All downloads are throttled to <see cref="MaxConcurrent"/> simultaneous
/// requests so startup warmup never saturates the connection.
/// </summary>
public sealed class MediaCacheService : IDisposable
{
    private const int MaxConcurrent = 8;

    private readonly string           _cacheDir;
    private readonly HttpClient       _http;
    private readonly SemaphoreSlim    _throttle = new(MaxConcurrent, MaxConcurrent);
    private readonly HashSet<string>  _inFlight  = [];
    private readonly object           _lock      = new();

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public MediaCacheService()
    {
        _cacheDir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
            "MGA", "media-cache");

        Directory.CreateDirectory(_cacheDir);

        _http = new HttpClient
        {
            Timeout = TimeSpan.FromSeconds(30),
        };
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns the local <c>file:///</c> URI if the image is already cached on
    /// disk, or the original remote URL when the cache is cold (and schedules a
    /// background download for next time).
    /// </summary>
    public string GetLocalOrRemoteUrl(string? remoteUrl)
    {
        if (string.IsNullOrEmpty(remoteUrl))
            return string.Empty;

        var localPath = GetLocalPath(remoteUrl);

        if (File.Exists(localPath))
            return ToFileUri(localPath);

        // Not cached — trigger background download and serve the remote URL now.
        _ = DownloadAsync(remoteUrl, localPath);

        return remoteUrl;
    }

    /// <summary>
    /// Eagerly downloads a batch of remote URLs in parallel (throttled).
    /// Call this at startup after the game list is known so subsequent
    /// <see cref="GetLocalOrRemoteUrl"/> calls always resolve locally.
    /// </summary>
    public async Task WarmAsync(IEnumerable<string> remoteUrls)
    {
        var tasks = remoteUrls
            .Where(u => !string.IsNullOrEmpty(u))
            .Distinct(StringComparer.Ordinal)
            .Select(url => DownloadAsync(url, GetLocalPath(url)));

        await Task.WhenAll(tasks).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // IDisposable
    // ---------------------------------------------------------------------------

    public void Dispose()
    {
        _http.Dispose();
        _throttle.Dispose();
    }

    // ---------------------------------------------------------------------------
    // Private — path helpers
    // ---------------------------------------------------------------------------

    private string GetLocalPath(string remoteUrl)
    {
        // Stable hash of the URL → short hex prefix as filename.
        var hash = Convert.ToHexString(
            SHA256.HashData(Encoding.UTF8.GetBytes(remoteUrl)))[..16];

        // Preserve the original extension when available.
        var ext = TryGetExtension(remoteUrl);

        return Path.Combine(_cacheDir, $"{hash}{ext}");
    }

    private static string TryGetExtension(string url)
    {
        try
        {
            var path = new Uri(url).AbsolutePath;
            var ext  = Path.GetExtension(path);
            return string.IsNullOrEmpty(ext) ? ".jpg" : ext;
        }
        catch
        {
            return ".jpg";
        }
    }

    private static string ToFileUri(string localPath)
        => "file:///" + localPath.Replace('\\', '/');

    // ---------------------------------------------------------------------------
    // Private — download
    // ---------------------------------------------------------------------------

    private async Task DownloadAsync(string remoteUrl, string localPath)
    {
        // Skip if file already exists (race-condition guard).
        if (File.Exists(localPath))
            return;

        // Skip if a download for this URL is already in-flight.
        lock (_lock)
        {
            if (_inFlight.Contains(remoteUrl))
                return;
            _inFlight.Add(remoteUrl);
        }

        await _throttle.WaitAsync().ConfigureAwait(false);
        try
        {
            if (File.Exists(localPath))
                return;

            var bytes = await _http.GetByteArrayAsync(remoteUrl).ConfigureAwait(false);

            // Atomic write: write to .tmp then rename.
            var tmp = localPath + ".tmp";
            await File.WriteAllBytesAsync(tmp, bytes).ConfigureAwait(false);
            File.Move(tmp, localPath, overwrite: true);
        }
        catch
        {
            // Non-fatal: the UI has already fallen back to the remote URL.
        }
        finally
        {
            _throttle.Release();
            lock (_lock) { _inFlight.Remove(remoteUrl); }
        }
    }
}
