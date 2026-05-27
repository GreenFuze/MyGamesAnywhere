using Microsoft.Win32;

namespace MGA.Desktop.Services.Install;

/// <summary>
/// Detects whether a Steam game (plugin_id = "game-source-steam") is installed
/// by reading Steam's <c>libraryfolders.vdf</c> and per-app <c>appmanifest_*.acf</c>
/// KeyValues files.
///
/// Also checks whether the Steam client itself is present on this machine, so
/// the UI can differentiate "game not downloaded" from "Steam not installed".
/// </summary>
public sealed class SteamInstallDetector : IInstallDetector
{
    public const string SteamPluginId = "game-source-steam";

    public string PluginId => SteamPluginId;

    // ---------------------------------------------------------------------------
    // IInstallDetector
    // ---------------------------------------------------------------------------

    public Task<InstallStatus?> DetectAsync(
        SourceGameInfo    source,
        string            canonicalTitle,
        CancellationToken ct = default)
    {
        // external_id for Steam is the numeric AppID string.
        if (!int.TryParse(source.ExternalId, out var appId))
            return Task.FromResult<InstallStatus?>(null);

        return Task.FromResult(Detect(appId));
    }

    // ---------------------------------------------------------------------------
    // Private — synchronous detection (all I/O is file-based, tiny files)
    // ---------------------------------------------------------------------------

    private static InstallStatus? Detect(int appId)
    {
        var steamPath = FindSteamPath();

        // Steam client not installed on this machine.
        if (steamPath is null)
            return new InstallStatus
            {
                State                = InstallState.ClientMissing,
                ClientMissingMessage = "Steam is not installed on this machine.",
                ClientDownloadUrl    = "https://store.steampowered.com/about/",
            };

        // Steam is installed — check all library folders.
        var libraries = ReadLibraryPaths(steamPath);
        foreach (var lib in libraries)
        {
            var acfPath = Path.Combine(lib, "steamapps", $"appmanifest_{appId}.acf");
            if (!File.Exists(acfPath))
                continue;

            var (stateFlags, installDir) = ParseAcf(acfPath);

            // Bit 2 (value 4) set = game is fully installed.
            if ((stateFlags & 4) == 4 && !string.IsNullOrEmpty(installDir))
            {
                var fullInstallPath = Path.Combine(lib, "steamapps", "common", installDir);
                return new InstallStatus
                {
                    State       = InstallState.Installed,
                    InstallPath = fullInstallPath,
                    LaunchUri   = $"steam://rungameid/{appId}",
                    UninstallUri = $"steam://uninstall/{appId}",
                };
            }

            // ACF exists but not installed (downloading / paused / etc.).
            return new InstallStatus
            {
                State     = InstallState.NotInstalled,
                LaunchUri = $"steam://install/{appId}",
            };
        }

        // ACF not found in any library → game is not in the user's Steam library
        // OR has never been downloaded. Show "install" pointing at the store page.
        return new InstallStatus
        {
            State     = InstallState.NotInstalled,
            LaunchUri = $"steam://install/{appId}",
        };
    }

    // ---------------------------------------------------------------------------
    // Private — Steam path resolution
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Reads the Steam install path from the Windows registry.
    /// Returns null on non-Windows or when Steam is not installed.
    /// </summary>
    private static string? FindSteamPath()
    {
        if (!OperatingSystem.IsWindows())
            return null;

        // 64-bit registry hive first, then 32-bit fallback.
        return ReadRegString(@"HKCU\Software\Valve\Steam", "SteamPath")
            ?? ReadRegString(@"HKLM\SOFTWARE\Valve\Steam", "InstallPath")
            ?? ReadRegString(@"HKLM\SOFTWARE\WOW6432Node\Valve\Steam", "InstallPath");
    }

    private static string? ReadRegString(string path, string valueName)
    {
        if (!OperatingSystem.IsWindows())
            return null;

        // Split into hive + subkey.
        var sep   = path.IndexOf('\\');
        var hive  = path[..sep];
        var subKey = path[(sep + 1)..];

        using var key = hive switch
        {
            "HKCU" => Registry.CurrentUser.OpenSubKey(subKey),
            "HKLM" => Registry.LocalMachine.OpenSubKey(subKey),
            _      => null,
        };

        return key?.GetValue(valueName) as string;
    }

    // ---------------------------------------------------------------------------
    // Private — libraryfolders.vdf parsing
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns all Steam library paths (including the default one under Steam root).
    /// Reads <c>{steamPath}/config/libraryfolders.vdf</c>.
    /// </summary>
    private static List<string> ReadLibraryPaths(string steamPath)
    {
        var paths = new List<string> { steamPath };

        var vdf = Path.Combine(steamPath, "config", "libraryfolders.vdf");
        if (!File.Exists(vdf))
            return paths;

        try
        {
            foreach (var line in File.ReadLines(vdf))
            {
                // Looking for:  "path"    "D:\\SteamLibrary"
                var trimmed = line.Trim();
                if (!trimmed.StartsWith("\"path\"", StringComparison.OrdinalIgnoreCase))
                    continue;

                var value = ExtractVdfValue(trimmed);
                if (!string.IsNullOrEmpty(value) && Directory.Exists(value))
                    paths.Add(value);
            }
        }
        catch
        {
            // If the VDF is malformed, just use the default library.
        }

        return paths;
    }

    // ---------------------------------------------------------------------------
    // Private — appmanifest_*.acf parsing
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Parses a Steam <c>appmanifest_N.acf</c> file and returns
    /// (<c>StateFlags</c>, <c>installdir</c>).
    /// </summary>
    private static (int stateFlags, string installDir) ParseAcf(string acfPath)
    {
        int    stateFlags = 0;
        string installDir = string.Empty;

        try
        {
            foreach (var line in File.ReadLines(acfPath))
            {
                var trimmed = line.Trim();

                if (trimmed.StartsWith("\"StateFlags\"", StringComparison.OrdinalIgnoreCase))
                {
                    var v = ExtractVdfValue(trimmed);
                    if (int.TryParse(v, out var flags))
                        stateFlags = flags;
                }
                else if (trimmed.StartsWith("\"installdir\"", StringComparison.OrdinalIgnoreCase))
                {
                    installDir = ExtractVdfValue(trimmed) ?? string.Empty;
                }
            }
        }
        catch
        {
            // Silently return defaults on parse failure.
        }

        return (stateFlags, installDir);
    }

    // ---------------------------------------------------------------------------
    // Private — VDF value extractor
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Extracts the value from a Valve KeyValues line like:
    /// <code>"key"    "value"</code>
    /// Returns the value string, or null if parsing fails.
    /// </summary>
    private static string? ExtractVdfValue(string line)
    {
        // Find the second quoted token.
        var first = line.IndexOf('"');
        if (first < 0) return null;

        var firstEnd = line.IndexOf('"', first + 1);
        if (firstEnd < 0) return null;

        var second = line.IndexOf('"', firstEnd + 1);
        if (second < 0) return null;

        var secondEnd = line.IndexOf('"', second + 1);
        if (secondEnd < 0) return null;

        return line[(second + 1)..secondEnd];
    }
}
