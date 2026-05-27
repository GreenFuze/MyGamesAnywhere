// CA1416: All Windows-only registry calls are guarded by OperatingSystem.IsWindows() in DetectAsync.
#pragma warning disable CA1416

using Microsoft.Win32;

namespace MGA.Desktop.Services.Install;

/// <summary>
/// Fallback install detector that scans the Windows Add/Remove Programs (ARP)
/// registry hives and fuzzy-matches <c>DisplayName</c> against the canonical game title.
///
/// Used when no storefront-specific detector handled a game (i.e. games installed
/// outside Steam/Epic — setup.exe installers, GOG galaxy-less installs, etc.).
///
/// Only entries with fuzzy similarity ≥ <see cref="ConfidenceThreshold"/> are returned;
/// lower-confidence matches are silently skipped so the caller can fall through.
/// </summary>
public sealed class ArpInstallDetector : IInstallDetector
{
    /// <summary>
    /// Minimum bigram-similarity score to accept an ARP entry as a match.
    /// Below this value the match is too uncertain to auto-launch.
    /// </summary>
    public const double ConfidenceThreshold = 0.85;

    // Empty PluginId means this is the "no specific plugin matched" fallback.
    public string PluginId => string.Empty;

    // ---------------------------------------------------------------------------
    // IInstallDetector
    // ---------------------------------------------------------------------------

    public Task<InstallStatus?> DetectAsync(
        SourceGameInfo    source,
        string            canonicalTitle,
        CancellationToken ct = default)
    {
        if (!OperatingSystem.IsWindows() || string.IsNullOrWhiteSpace(canonicalTitle))
            return Task.FromResult<InstallStatus?>(null);

        return Task.FromResult(Detect(canonicalTitle));
    }

    // ---------------------------------------------------------------------------
    // Private — synchronous (all I/O is registry reads; very fast)
    // ---------------------------------------------------------------------------

    [System.Runtime.Versioning.SupportedOSPlatform("windows")]
    private static InstallStatus? Detect(string canonicalTitle)
    {
        var bestEntry = default(ArpEntry);
        var bestScore = 0.0;

        // Scan all three ARP hives and keep the highest-scoring entry.
        foreach (var entry in EnumerateArpEntries())
        {
            var score = ExeResolver.FuzzyRatio(entry.DisplayName, canonicalTitle);
            if (score > bestScore)
            {
                bestScore = score;
                bestEntry = entry;
            }
        }

        if (bestScore < ConfidenceThreshold || bestEntry is null)
            return null;

        // High-confidence match — attempt to resolve the main executable.
        var exePath = string.Empty;

        if (!string.IsNullOrEmpty(bestEntry.InstallLocation) &&
            Directory.Exists(bestEntry.InstallLocation))
        {
            // Prefer the explicit InstallLocation; use DisplayIcon as hint.
            var resolved = ExeResolver.Resolve(
                bestEntry.InstallLocation, canonicalTitle, bestEntry.DisplayIcon);
            exePath = resolved ?? string.Empty;
        }
        else if (!string.IsNullOrEmpty(bestEntry.DisplayIcon))
        {
            // No InstallLocation — derive the path from the DisplayIcon entry.
            var iconPath = StripIconIndex(bestEntry.DisplayIcon);
            if (iconPath.EndsWith(".exe", StringComparison.OrdinalIgnoreCase) &&
                File.Exists(iconPath))
                exePath = iconPath;
        }

        // If no exe was found, the user must confirm it manually.
        var state = string.IsNullOrEmpty(exePath)
            ? InstallState.ManualBindNeeded
            : InstallState.Installed;

        return new InstallStatus
        {
            State        = state,
            InstallPath  = bestEntry.InstallLocation,
            ExePath      = string.IsNullOrEmpty(exePath) ? null : exePath,
            LaunchUri    = string.IsNullOrEmpty(exePath) ? null : exePath,
            UninstallUri = bestEntry.UninstallString,
            Confidence   = bestScore,
        };
    }

    // ---------------------------------------------------------------------------
    // Private — ARP registry enumeration
    // ---------------------------------------------------------------------------

    private sealed record ArpEntry(
        string  DisplayName,
        string  InstallLocation,
        string? DisplayIcon,
        string? UninstallString);

    [System.Runtime.Versioning.SupportedOSPlatform("windows")]
    private static IEnumerable<ArpEntry> EnumerateArpEntries()
    {
        // 64-bit programs in HKLM
        foreach (var e in ScanHive(Registry.LocalMachine,
            @"SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall"))
            yield return e;

        // 32-bit programs in HKLM (WOW node)
        foreach (var e in ScanHive(Registry.LocalMachine,
            @"SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall"))
            yield return e;

        // Per-user installs in HKCU
        foreach (var e in ScanHive(Registry.CurrentUser,
            @"SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall"))
            yield return e;
    }

    [System.Runtime.Versioning.SupportedOSPlatform("windows")]
    private static IEnumerable<ArpEntry> ScanHive(RegistryKey hive, string subkeyPath)
    {
        using var root = hive.OpenSubKey(subkeyPath);
        if (root is null) yield break;

        foreach (var name in root.GetSubKeyNames())
        {
            using var sub = root.OpenSubKey(name);
            if (sub is null) continue;

            var displayName = sub.GetValue("DisplayName") as string;
            if (string.IsNullOrWhiteSpace(displayName)) continue;

            // Skip Windows system components (hotfixes, runtime packages, etc.)
            if (sub.GetValue("SystemComponent") is int sc && sc == 1) continue;

            var installLocation = (sub.GetValue("InstallLocation") as string ?? string.Empty).Trim();
            var displayIcon     = sub.GetValue("DisplayIcon") as string;
            var uninstallString = sub.GetValue("UninstallString") as string;

            yield return new ArpEntry(displayName, installLocation, displayIcon, uninstallString);
        }
    }

    // ---------------------------------------------------------------------------
    // Helpers
    // ---------------------------------------------------------------------------

    /// <summary>Strips a trailing ",N" icon-index suffix from a DisplayIcon value.</summary>
    private static string StripIconIndex(string path)
    {
        var comma = path.LastIndexOf(',');
        return comma > 0 && int.TryParse(path[(comma + 1)..], out _)
            ? path[..comma].Trim()
            : path;
    }
}
