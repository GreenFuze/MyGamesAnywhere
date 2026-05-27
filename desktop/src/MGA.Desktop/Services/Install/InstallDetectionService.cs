using System.Reactive.Subjects;

namespace MGA.Desktop.Services.Install;

/// <summary>
/// Orchestrates install detection for the entire game library.
///
/// For each game it tries, in order:
/// <list type="number">
///   <item>Manual binding — user-confirmed exe path (always wins).</item>
///   <item>Per-source storefront detectors (Steam, Epic, …) matched by PluginId.</item>
///   <item>ARP fallback detector (fuzzy match against canonical title).</item>
/// </list>
///
/// Results are cached in memory (session-only; the server is never contacted).
/// The <see cref="StatusUpdated"/> observable notifies subscribers of changes
/// so the UI can update game-card badges reactively.
/// </summary>
public sealed class InstallDetectionService : IDisposable
{
    private readonly Dictionary<string, IInstallDetector> _byPluginId;
    private readonly IInstallDetector? _fallback; // detector with empty PluginId
    private readonly InstallBindingService _bindings;

    // Session cache: canonical game ID → latest detected status.
    private readonly Dictionary<string, InstallStatus> _cache = [];

    // Reactive stream; fires on every cache write.
    private readonly Subject<(string GameId, InstallStatus Status)> _updates = new();

    /// <summary>
    /// Fires whenever a game's install status is set or refreshed.
    /// The event carries <c>(gameId, newStatus)</c> on the calling thread.
    /// </summary>
    public IObservable<(string GameId, InstallStatus Status)> StatusUpdated => _updates;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public InstallDetectionService(
        IEnumerable<IInstallDetector> detectors,
        InstallBindingService         bindings)
    {
        _bindings = bindings;

        // Partition: non-empty PluginId → exact-match dict; empty → fallback.
        var named = new Dictionary<string, IInstallDetector>(StringComparer.OrdinalIgnoreCase);
        IInstallDetector? fallback = null;

        foreach (var d in detectors)
        {
            if (string.IsNullOrEmpty(d.PluginId))
                fallback = d;
            else
                named[d.PluginId] = d;
        }

        _byPluginId = named;
        _fallback   = fallback;
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns the last cached status for <paramref name="gameId"/>,
    /// or <see langword="null"/> when detection has not run yet.
    /// </summary>
    public InstallStatus? GetCachedStatus(string gameId) =>
        _cache.TryGetValue(gameId, out var s) ? s : null;

    /// <summary>
    /// Detects the install status for a single game.
    ///
    /// Checks manual bindings, then each source's storefront detector, then ARP.
    /// The result is stored in the memory cache and published to <see cref="StatusUpdated"/>.
    /// </summary>
    public async Task<InstallStatus> DetectGameAsync(
        string                       gameId,
        string                       canonicalTitle,
        IReadOnlyList<SourceGameInfo> sources,
        CancellationToken            ct = default)
    {
        // 1. Manual binding beats everything — if it exists and the file is still there.
        var boundExe = _bindings.GetBinding(gameId);
        if (boundExe is not null && File.Exists(boundExe))
        {
            var bound = new InstallStatus
            {
                State      = InstallState.Installed,
                ExePath    = boundExe,
                LaunchUri  = boundExe,
                Confidence = 1.0,
            };
            PushCache(gameId, bound);
            return bound;
        }

        // 2. Try each source against its registered plugin detector.
        foreach (var source in sources)
        {
            ct.ThrowIfCancellationRequested();

            if (!_byPluginId.TryGetValue(source.PluginId, out var detector))
                continue;

            try
            {
                var result = await detector.DetectAsync(source, canonicalTitle, ct)
                    .ConfigureAwait(false);

                if (result is not null)
                {
                    PushCache(gameId, result);
                    return result;
                }
            }
            catch (OperationCanceledException) { throw; }
            catch { /* detector threw — try next */ }
        }

        // 3. ARP fallback: try matching the canonical title in installed programs.
        if (_fallback is not null)
        {
            try
            {
                var dummySource = new SourceGameInfo();
                var result = await _fallback.DetectAsync(dummySource, canonicalTitle, ct)
                    .ConfigureAwait(false);

                if (result is not null)
                {
                    PushCache(gameId, result);
                    return result;
                }
            }
            catch (OperationCanceledException) { throw; }
            catch { /* ARP scan failed */ }
        }

        // 4. Could not determine; mark as Unknown (will show "Not detected" in UI).
        var unknown = InstallStatus.NotChecked;
        PushCache(gameId, unknown);
        return unknown;
    }

    /// <summary>
    /// Runs detection for each (gameId, title, sources) tuple concurrently,
    /// bounded to 4 tasks to avoid registry / file-system contention.
    ///
    /// Each result is published through <see cref="StatusUpdated"/> as it arrives.
    /// Callers subscribe to that observable and marshal UI updates themselves.
    /// </summary>
    public async Task DetectAllAsync(
        IEnumerable<(string GameId, string Title, IReadOnlyList<SourceGameInfo> Sources)> games,
        CancellationToken ct = default)
    {
        using var semaphore = new SemaphoreSlim(initialCount: 4, maxCount: 4);

        var tasks = games.Select(async item =>
        {
            await semaphore.WaitAsync(ct).ConfigureAwait(false);
            try
            {
                await DetectGameAsync(item.GameId, item.Title, item.Sources, ct)
                    .ConfigureAwait(false);
            }
            catch (OperationCanceledException) { /* stop gracefully */ }
            catch { /* per-game failure is non-fatal */ }
            finally
            {
                semaphore.Release();
            }
        });

        await Task.WhenAll(tasks).ConfigureAwait(false);
    }

    /// <summary>
    /// Records the user-confirmed exe path for <paramref name="gameId"/>,
    /// persists it via <see cref="InstallBindingService"/>, and fires
    /// <see cref="StatusUpdated"/> with an updated Installed status.
    /// </summary>
    public void SetManualBinding(string gameId, string exePath)
    {
        _bindings.SetBinding(gameId, exePath);

        var status = new InstallStatus
        {
            State      = InstallState.Installed,
            ExePath    = exePath,
            LaunchUri  = exePath,
            Confidence = 1.0,
        };
        PushCache(gameId, status);
    }

    /// <summary>
    /// Clears the manual binding for <paramref name="gameId"/> and re-runs detection.
    /// </summary>
    public async Task ClearManualBindingAsync(
        string                       gameId,
        string                       canonicalTitle,
        IReadOnlyList<SourceGameInfo> sources,
        CancellationToken            ct = default)
    {
        _bindings.RemoveBinding(gameId);
        await DetectGameAsync(gameId, canonicalTitle, sources, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Private
    // ---------------------------------------------------------------------------

    private void PushCache(string gameId, InstallStatus status)
    {
        _cache[gameId] = status;
        _updates.OnNext((gameId, status));
    }

    public void Dispose() => _updates.Dispose();
}
