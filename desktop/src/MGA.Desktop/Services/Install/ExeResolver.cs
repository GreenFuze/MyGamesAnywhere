namespace MGA.Desktop.Services.Install;

/// <summary>
/// Heuristic executable resolver.
/// Given an install directory, finds the most likely "main game" executable
/// so the desktop client can launch it directly.
///
/// Priority order:
///   1. ARP <c>DisplayIcon</c> registry value (caller passes this in)
///   2. Manually bound path stored in <see cref="InstallBindingService"/>
///   3. Largest .exe in the root (ignoring known non-game executables)
///   4. .exe with highest fuzzy similarity to the canonical game title
/// </summary>
public static class ExeResolver
{
    // ---------------------------------------------------------------------------
    // Blocklist — filenames (lower-case) known NOT to be game executables
    // ---------------------------------------------------------------------------

    private static readonly HashSet<string> s_blocklist =
        new(StringComparer.OrdinalIgnoreCase)
        {
            // Installers / uninstallers
            "unins000.exe", "unins001.exe", "uninstall.exe",
            "setup.exe", "install.exe", "autorun.exe",
            // Redistributables
            "vcredist_x64.exe", "vcredist_x86.exe", "vcredist.exe",
            "vcredist2005_x64.exe", "vcredist2005_x86.exe",
            "dxsetup.exe", "directx.exe",
            "dotnetfx.exe", "windowsdesktop-runtime.exe",
            "vc_redist.x64.exe", "vc_redist.x86.exe",
            // Unity / engine helpers
            "unitycrashandler64.exe", "unitycrashandler32.exe",
            "crashreportclient.exe", "crashpad_handler.exe",
            "cefsubprocess.exe",
            // Launchers / overlays that are not the game itself
            "uplay.exe", "epicgameslauncher.exe", "steam.exe",
            "easyanticheat.exe", "battleyeservice.exe",
        };

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Attempts to resolve the main game executable in <paramref name="installDir"/>.
    /// </summary>
    /// <param name="installDir">Root install directory.</param>
    /// <param name="canonicalTitle">Canonical game title — used for name-similarity scoring.</param>
    /// <param name="displayIconPath">
    /// Value of the ARP <c>DisplayIcon</c> registry key, if available.
    /// May contain a trailing <c>,N</c> icon-index suffix.
    /// </param>
    /// <returns>
    /// The most likely game executable path, or <see langword="null"/> if
    /// the directory contains no suitable candidates.
    /// </returns>
    public static string? Resolve(
        string  installDir,
        string  canonicalTitle,
        string? displayIconPath = null)
    {
        if (!Directory.Exists(installDir))
            return null;

        // 1. DisplayIcon — often the most reliable hint.
        if (!string.IsNullOrEmpty(displayIconPath))
        {
            var iconPath = StripIconIndex(displayIconPath);
            if (iconPath.EndsWith(".exe", StringComparison.OrdinalIgnoreCase) &&
                File.Exists(iconPath) &&
                IsCandidate(Path.GetFileName(iconPath)))
                return iconPath;
        }

        // 2. Gather all .exe candidates in root + immediate subdirectories.
        var candidates = EnumerateCandidates(installDir);
        if (candidates.Count == 0)
            return null;

        // 3. Score each candidate by a combination of file size and title similarity.
        var best = candidates
            .Select(path => (path, score: Score(path, canonicalTitle)))
            .OrderByDescending(t => t.score)
            .FirstOrDefault();

        return best.path;
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Strips a trailing <c>,N</c> icon-index suffix from a DisplayIcon path.
    /// </summary>
    private static string StripIconIndex(string path)
    {
        var comma = path.LastIndexOf(',');
        if (comma > 0 && int.TryParse(path[(comma + 1)..], out _))
            return path[..comma].Trim();
        return path;
    }

    /// <summary>
    /// Collects .exe files from the root directory and one level of subdirectories,
    /// excluding blocklisted names and known non-game subdirectory names.
    /// </summary>
    private static List<string> EnumerateCandidates(string root)
    {
        var results = new List<string>();

        // Root-level executables.
        AddExesFrom(root, results);

        // First-level subdirectories (skip common non-game dirs).
        foreach (var dir in Directory.EnumerateDirectories(root))
        {
            var name = Path.GetFileName(dir);
            if (IsNonGameDirectory(name))
                continue;
            AddExesFrom(dir, results);
        }

        return results;
    }

    private static void AddExesFrom(string dir, List<string> results)
    {
        try
        {
            foreach (var file in Directory.EnumerateFiles(dir, "*.exe"))
            {
                if (IsCandidate(Path.GetFileName(file)))
                    results.Add(file);
            }
        }
        catch (UnauthorizedAccessException) { }
        catch (DirectoryNotFoundException)  { }
    }

    private static bool IsCandidate(string fileName) =>
        !s_blocklist.Contains(fileName) &&
        // Additional pattern-based exclusions.
        !fileName.StartsWith("unins",    StringComparison.OrdinalIgnoreCase) &&
        !fileName.StartsWith("vcredist", StringComparison.OrdinalIgnoreCase) &&
        !fileName.StartsWith("dotnet",   StringComparison.OrdinalIgnoreCase) &&
        !fileName.StartsWith("crash",    StringComparison.OrdinalIgnoreCase) &&
        !fileName.StartsWith("report",   StringComparison.OrdinalIgnoreCase);

    private static bool IsNonGameDirectory(string name) =>
        name.Equals("_CommonRedist",   StringComparison.OrdinalIgnoreCase) ||
        name.Equals("__redist",        StringComparison.OrdinalIgnoreCase) ||
        name.Equals("Redist",          StringComparison.OrdinalIgnoreCase) ||
        name.Equals("Redistributables", StringComparison.OrdinalIgnoreCase) ||
        name.Equals("DirectX",         StringComparison.OrdinalIgnoreCase) ||
        name.Equals("vcredist",        StringComparison.OrdinalIgnoreCase);

    /// <summary>
    /// Composite score: 70 % file size (normalised) + 30 % title similarity.
    /// Larger executables and executables whose names resemble the game title win.
    /// </summary>
    private static double Score(string exePath, string canonicalTitle)
    {
        try
        {
            var info = new FileInfo(exePath);
            // Normalise size to [0, 1] with a soft cap at 500 MB.
            var sizeMb    = info.Length / (1024.0 * 1024.0);
            var sizeScore = Math.Min(sizeMb / 500.0, 1.0);

            var nameSimilarity = FuzzyRatio(
                Path.GetFileNameWithoutExtension(exePath),
                canonicalTitle);

            return 0.7 * sizeScore + 0.3 * nameSimilarity;
        }
        catch
        {
            return 0;
        }
    }

    // ---------------------------------------------------------------------------
    // Fuzzy string ratio (simple character bi-gram overlap)
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns a [0, 1] similarity score between two strings using bi-gram overlap.
    /// Fast enough for a few dozen candidates, no external dependencies.
    /// </summary>
    public static double FuzzyRatio(string a, string b)
    {
        if (string.IsNullOrEmpty(a) || string.IsNullOrEmpty(b))
            return 0;

        a = a.ToLowerInvariant();
        b = b.ToLowerInvariant();

        if (a == b) return 1.0;

        var aGrams = GetBigrams(a);
        var bGrams = GetBigrams(b);

        int matches = 0;
        foreach (var g in aGrams)
            if (bGrams.Remove(g))
                matches++;

        return 2.0 * matches / (GetBigrams(a).Count + GetBigrams(b).Count);
    }

    private static List<string> GetBigrams(string s)
    {
        var grams = new List<string>(s.Length);
        for (var i = 0; i < s.Length - 1; i++)
            grams.Add(s.Substring(i, 2));
        return grams;
    }
}
