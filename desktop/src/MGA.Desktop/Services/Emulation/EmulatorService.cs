using System.Security.Cryptography;

namespace MGA.Desktop.Services.Emulation;

/// <summary>
/// Runtime BIOS-check result for a specific platform and emulator.
/// </summary>
public sealed class BiosCheckResult
{
    /// <summary>Platform this result covers (e.g. "ps1").</summary>
    public string Platform { get; init; } = string.Empty;

    /// <summary>
    /// True when all <em>required</em> BIOS files are present and,
    /// where a SHA-256 hash is documented, hash-verified.
    /// Optional missing files do not affect this flag.
    /// </summary>
    public bool AllRequiredPresent { get; init; }

    /// <summary>Required BIOS files that are absent or hash-mismatched.</summary>
    public IReadOnlyList<BiosCatalogEntry> Missing { get; init; } = [];

    /// <summary>Optional BIOS files that are absent (informational).</summary>
    public IReadOnlyList<BiosCatalogEntry> MissingOptional { get; init; } = [];
}

/// <summary>
/// Result of verifying a candidate BIOS file stream against the catalog.
/// </summary>
public sealed class BiosVerifyResult
{
    /// <summary>
    /// The matching catalog entry when the file hash matches a known BIOS,
    /// or null when the file is unrecognised.
    /// </summary>
    public BiosCatalogEntry? Match { get; init; }

    /// <summary>SHA-256 of the provided file in lowercase hex.</summary>
    public string ComputedHash { get; init; } = string.Empty;

    /// <summary>True when the file is recognised and matches a catalog entry.</summary>
    public bool IsRecognised => Match is not null;
}

/// <summary>
/// Aggregated BIOS check result for one emulator install + platform combination.
/// Returned by <see cref="EmulatorService.CheckBiosForAllInstallsAsync"/>.
/// </summary>
public sealed class BiosInstallCheckResult
{
    public string          InstallId    { get; init; } = string.Empty;
    public string          EmulatorName { get; init; } = string.Empty;
    public string          Platform     { get; init; } = string.Empty;
    public BiosCheckResult Check        { get; init; } = new();
}

/// <summary>
/// Result returned by <see cref="EmulatorService.TryIdentifyBiosFileAsync"/> when
/// a dropped / imported file matches a known BIOS catalog entry.
/// </summary>
public sealed class BiosIdentifyResult
{
    public string          EmulatorId   { get; init; } = string.Empty;
    public string          EmulatorName { get; init; } = string.Empty;
    public string          Platform     { get; init; } = string.Empty;
    public BiosCatalogEntry BiosEntry   { get; init; } = null!;
    public string          ComputedHash { get; init; } = string.Empty;
}

/// <summary>
/// Manages MGA-owned emulator installs, launch configurations, BIOS files,
/// and game install records on this device.
///
/// All persistent mutations go through <see cref="AppConfigService"/> to keep
/// config.json as the single source of truth.
///
/// RAII: no background work in the constructor; paths are resolved lazily.
/// </summary>
public sealed class EmulatorService
{
    private readonly AppConfigService    _config;
    private readonly EmulatorCatalogService _catalog;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public EmulatorService(AppConfigService config, EmulatorCatalogService catalog)
    {
        _config  = config;
        _catalog = catalog;
    }

    // ---------------------------------------------------------------------------
    // Managed directories
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Resolved path to the BIOS directory on this device.
    /// Defaults to <c>{DataDirectory}/bios</c> when not overridden in config.
    /// </summary>
    public string BiosDirectory
    {
        get
        {
            var dir = _config.Config.BiosDirectory?.Trim();
            return string.IsNullOrEmpty(dir)
                ? Path.Combine(_config.DataDirectory, "bios")
                : dir;
        }
    }

    /// <summary>
    /// Resolved default game library directory on this device.
    /// Defaults to <c>{DataDirectory}/library</c> when not overridden in config.
    /// </summary>
    public string LibraryDirectory
    {
        get
        {
            var dir = _config.Config.LibraryDirectory?.Trim();
            return string.IsNullOrEmpty(dir)
                ? Path.Combine(_config.DataDirectory, "library")
                : dir;
        }
    }

    // ---------------------------------------------------------------------------
    // Emulator installs
    // ---------------------------------------------------------------------------

    /// <summary>All emulator installs recorded on this device (read-only view).</summary>
    public IReadOnlyList<EmulatorInstallRecord> Installs => _config.Config.EmulatorInstalls;

    /// <summary>Returns the install record with the given ID, or null.</summary>
    public EmulatorInstallRecord? GetInstall(string installId) =>
        _config.Config.EmulatorInstalls.FirstOrDefault(
            i => string.Equals(i.Id, installId, StringComparison.Ordinal));

    /// <summary>
    /// Records a new emulator install (either MGA-managed or user-located).
    /// </summary>
    /// <returns>The new record's stable ID.</returns>
    public string AddInstall(
        string?  catalogId,
        string   displayName,
        string   executablePath,
        bool     mgaManaged,
        string   version = "")
    {
        var record = new EmulatorInstallRecord
        {
            CatalogId      = catalogId,
            Name           = displayName,
            ExecutablePath = executablePath,
            MgaManaged     = mgaManaged,
            Version        = version,
            InstalledAt    = DateTime.UtcNow,
        };

        _config.Update(c => c.EmulatorInstalls.Add(record));
        return record.Id;
    }

    /// <summary>
    /// Removes an emulator install and all configurations that reference it.
    /// </summary>
    public void RemoveInstall(string installId)
    {
        _config.Update(c =>
        {
            c.EmulatorInstalls.RemoveAll(i => i.Id == installId);

            // Cascade: orphaned configs are invalid; remove them too.
            c.EmulatorConfigs.RemoveAll(cfg => cfg.InstallId == installId);
        });
    }

    // ---------------------------------------------------------------------------
    // Emulator configs
    // ---------------------------------------------------------------------------

    /// <summary>All launch configurations on this device (read-only view).</summary>
    public IReadOnlyList<EmulatorConfig> Configs => _config.Config.EmulatorConfigs;

    /// <summary>
    /// Returns configs that cover the specified platform, ordered ascending by
    /// <see cref="EmulatorConfig.Priority"/> (lower = higher priority).
    /// </summary>
    public IReadOnlyList<EmulatorConfig> GetConfigsForPlatform(string platform) =>
        _config.Config.EmulatorConfigs
            .Where(c => c.Platforms.Contains(platform, StringComparer.OrdinalIgnoreCase))
            .OrderBy(c => c.Priority)
            .ToList();

    /// <summary>Adds a new launch configuration.</summary>
    /// <returns>The new config's stable ID.</returns>
    public string AddConfig(
        string                     installId,
        string                     displayName,
        List<string>               platforms,
        bool                       raEnabled      = false,
        Dictionary<string,string>? corePerPlatform = null,
        string?                    argsTemplate    = null,
        int                        priority        = 0)
    {
        var cfg = new EmulatorConfig
        {
            InstallId                = installId,
            DisplayName              = displayName,
            Platforms                = platforms,
            RetroAchievementsEnabled = raEnabled,
            CorePerPlatform          = corePerPlatform ?? [],
            ArgsTemplate             = argsTemplate,
            Priority                 = priority,
        };

        _config.Update(c => c.EmulatorConfigs.Add(cfg));
        return cfg.Id;
    }

    /// <summary>Removes a launch configuration by ID.</summary>
    public void RemoveConfig(string configId)
    {
        _config.Update(c => c.EmulatorConfigs.RemoveAll(cfg => cfg.Id == configId));
    }

    // ---------------------------------------------------------------------------
    // Game install records
    // ---------------------------------------------------------------------------

    /// <summary>Returns the install record for a source game, or null if not installed.</summary>
    public GameInstallRecord? GetGameInstall(string sourceGameId) =>
        _config.Config.GameInstalls.FirstOrDefault(
            r => string.Equals(r.SourceGameId, sourceGameId, StringComparison.Ordinal));

    /// <summary>
    /// Records a completed game installation (or an existing user-located install).
    /// Overwrites any previous record for the same source game.
    /// </summary>
    public void SetGameInstall(
        string  sourceGameId,
        string  installPath,
        string? detectedExePath,
        bool    userLocated)
    {
        _config.Update(c =>
        {
            // Remove any stale record for this source game.
            c.GameInstalls.RemoveAll(r => r.SourceGameId == sourceGameId);

            c.GameInstalls.Add(new GameInstallRecord
            {
                SourceGameId    = sourceGameId,
                InstallPath     = installPath,
                DetectedExePath = detectedExePath,
                InstalledAt     = DateTime.UtcNow,
                UserLocated     = userLocated,
            });
        });
    }

    /// <summary>
    /// Removes the install record for a source game.
    /// Call this after a successful uninstall to clean up state.
    /// </summary>
    public void RemoveGameInstall(string sourceGameId)
    {
        _config.Update(c => c.GameInstalls.RemoveAll(r => r.SourceGameId == sourceGameId));
    }

    // ---------------------------------------------------------------------------
    // Directory configuration setters
    // ---------------------------------------------------------------------------

    /// <summary>Persists a custom BIOS directory path. Pass an empty string to restore the default.</summary>
    public void SetBiosDirectory(string path)
    {
        _config.Update(c => c.BiosDirectory = path.Trim());
    }

    /// <summary>Persists a custom library directory path. Pass an empty string to restore the default.</summary>
    public void SetLibraryDirectory(string path)
    {
        _config.Update(c => c.LibraryDirectory = path.Trim());
    }

    // ---------------------------------------------------------------------------
    // Catalog lookup helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns the catalog entry for an emulator install, or null when the install
    /// has no catalog ID or the catalog entry cannot be found.
    /// </summary>
    public EmulatorCatalogEntry? GetCatalogEntryForInstall(string installId)
    {
        var install = GetInstall(installId);
        return install?.CatalogId is not null ? _catalog.GetById(install.CatalogId) : null;
    }

    /// <summary>
    /// Runs BIOS checks for every installed emulator that has a catalog entry and
    /// at least one BIOS requirement.  Returns one result per (install, platform) pair.
    /// </summary>
    public async Task<IReadOnlyList<BiosInstallCheckResult>> CheckBiosForAllInstallsAsync(
        CancellationToken ct = default)
    {
        var results = new List<BiosInstallCheckResult>();

        foreach (var install in Installs)
        {
            if (install.CatalogId is null) continue;
            var entry = _catalog.GetById(install.CatalogId);
            if (entry is null) continue;
            if (entry.BiosRequirements is null || entry.BiosRequirements.Count == 0) continue;

            foreach (var platform in entry.BiosRequirements.Keys)
            {
                ct.ThrowIfCancellationRequested();
                var check = await CheckBiosAsync(entry, platform, ct).ConfigureAwait(false);

                results.Add(new BiosInstallCheckResult
                {
                    InstallId    = install.Id,
                    EmulatorName = install.Name,
                    Platform     = platform,
                    Check        = check,
                });
            }
        }

        return results;
    }

    /// <summary>
    /// Scans all emulator catalog entries and tries to match a BIOS stream by SHA-256.
    /// Returns the first match found, or null when the file is not recognised.
    /// </summary>
    public async Task<BiosIdentifyResult?> TryIdentifyBiosFileAsync(
        Stream            fileStream,
        CancellationToken ct = default)
    {
        // Compute the hash once, then compare against all known entries.
        using var sha    = System.Security.Cryptography.SHA256.Create();
        var hashBytes    = await sha.ComputeHashAsync(fileStream, ct).ConfigureAwait(false);
        var hash         = Convert.ToHexString(hashBytes).ToLowerInvariant();

        foreach (var entry in _catalog.All)
        {
            if (entry.BiosRequirements is null) continue;

            foreach (var (platform, bioses) in entry.BiosRequirements)
            {
                foreach (var bios in bioses)
                {
                    if (string.IsNullOrEmpty(bios.Sha256)) continue;
                    if (string.Equals(bios.Sha256, hash, StringComparison.OrdinalIgnoreCase))
                    {
                        return new BiosIdentifyResult
                        {
                            EmulatorId   = entry.Id,
                            EmulatorName = entry.DisplayName,
                            Platform     = platform,
                            BiosEntry    = bios,
                            ComputedHash = hash,
                        };
                    }
                }
            }
        }

        return null;
    }

    // ---------------------------------------------------------------------------
    // BIOS checking
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Checks the managed BIOS directory for files required by the given emulator
    /// and platform. Verifies SHA-256 hashes for entries where a hash is documented.
    /// </summary>
    /// <returns>
    /// A <see cref="BiosCheckResult"/> summarising which required and optional
    /// BIOS files are present or missing.
    /// </returns>
    public async Task<BiosCheckResult> CheckBiosAsync(
        EmulatorCatalogEntry entry,
        string               platform,
        CancellationToken    ct = default)
    {
        var requirements = entry.GetBiosRequirements(platform);

        // No BIOS needed for this platform → immediately OK.
        if (requirements.Count == 0)
        {
            return new BiosCheckResult
            {
                Platform           = platform,
                AllRequiredPresent = true,
            };
        }

        var biosDir    = BiosDirectory;
        var missing    = new List<BiosCatalogEntry>();
        var missingOpt = new List<BiosCatalogEntry>();

        foreach (var req in requirements)
        {
            ct.ThrowIfCancellationRequested();

            var filePath = Path.Combine(biosDir, req.Filename);

            if (!File.Exists(filePath))
            {
                (req.Required ? missing : missingOpt).Add(req);
                continue;
            }

            // Verify hash when documented.
            if (!string.IsNullOrEmpty(req.Sha256))
            {
                var actual = await ComputeSha256Async(filePath, ct).ConfigureAwait(false);
                if (!string.Equals(actual, req.Sha256, StringComparison.OrdinalIgnoreCase))
                {
                    // File is present but hash mismatch → treat as missing.
                    (req.Required ? missing : missingOpt).Add(req);
                }
            }
        }

        return new BiosCheckResult
        {
            Platform           = platform,
            AllRequiredPresent = missing.Count == 0,
            Missing            = missing,
            MissingOptional    = missingOpt,
        };
    }

    /// <summary>
    /// Computes the SHA-256 of a stream and tries to match it against all catalog
    /// BIOS entries for the given emulator and platform.
    ///
    /// Used by the drag-and-drop BIOS UI to verify a dropped file before placing it.
    /// </summary>
    public async Task<BiosVerifyResult> VerifyBiosFileAsync(
        Stream               fileStream,
        EmulatorCatalogEntry entry,
        string               platform,
        CancellationToken    ct = default)
    {
        var hash = await ComputeSha256Async(fileStream, ct).ConfigureAwait(false);

        // Match against any requirement that has a documented hash.
        foreach (var req in entry.GetBiosRequirements(platform))
        {
            if (string.IsNullOrEmpty(req.Sha256)) continue;
            if (string.Equals(hash, req.Sha256, StringComparison.OrdinalIgnoreCase))
                return new BiosVerifyResult { Match = req, ComputedHash = hash };
        }

        return new BiosVerifyResult { Match = null, ComputedHash = hash };
    }

    /// <summary>
    /// Copies a BIOS file into the managed BIOS directory under its correct filename.
    /// Creates the directory if needed. Overwrites any existing file with the same name.
    /// </summary>
    public async Task PlaceBiosFileAsync(
        Stream            sourceStream,
        string            targetFilename,
        CancellationToken ct = default)
    {
        // Sanitise: strip any path components — only the bare filename is placed here.
        var safeName = Path.GetFileName(targetFilename.Trim());
        if (string.IsNullOrEmpty(safeName))
            throw new ArgumentException($"Invalid BIOS filename: '{targetFilename}'.", nameof(targetFilename));

        var biosDir  = BiosDirectory;
        Directory.CreateDirectory(biosDir);

        var destPath = Path.Combine(biosDir, safeName);
        using var dest = File.Open(destPath, FileMode.Create, FileAccess.Write, FileShare.None);
        await sourceStream.CopyToAsync(dest, ct).ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private static async Task<string> ComputeSha256Async(string filePath, CancellationToken ct)
    {
        await using var fs = File.OpenRead(filePath);
        return await ComputeSha256Async(fs, ct).ConfigureAwait(false);
    }

    private static async Task<string> ComputeSha256Async(Stream stream, CancellationToken ct)
    {
        using var sha = SHA256.Create();
        var hashBytes = await sha.ComputeHashAsync(stream, ct).ConfigureAwait(false);
        return Convert.ToHexString(hashBytes).ToLowerInvariant();
    }
}
