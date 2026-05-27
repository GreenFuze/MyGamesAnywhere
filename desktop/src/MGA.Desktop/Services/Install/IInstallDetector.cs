namespace MGA.Desktop.Services.Install;

/// <summary>
/// A per-plugin-type install detector.
/// Each concrete implementation handles one storefront (Steam, Epic, …)
/// or one detection method (ARP for file-backed games).
///
/// Detectors are registered in <see cref="InstallDetectionService"/> by their
/// <see cref="PluginId"/>. The service routes each source game to the correct detector.
/// </summary>
public interface IInstallDetector
{
    /// <summary>
    /// The plugin_id this detector handles, e.g. "game-source-steam".
    /// For catch-all detectors (like ARP) this is the empty string — the service
    /// falls back to those when no plugin-specific detector matches.
    /// </summary>
    string PluginId { get; }

    /// <summary>
    /// Asynchronously checks whether the game described by <paramref name="source"/>
    /// is installed on this machine.
    /// </summary>
    /// <returns>
    /// An <see cref="InstallStatus"/> describing the current install state,
    /// or <see langword="null"/> if this detector cannot make a determination
    /// for the given source (the service will try the next detector).
    /// </returns>
    Task<InstallStatus?> DetectAsync(
        SourceGameInfo        source,
        string                canonicalTitle,
        CancellationToken     ct = default);
}
