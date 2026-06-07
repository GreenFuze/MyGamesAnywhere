using MGA.Api;
using MGA.Desktop.Services.Emulation;

namespace MGA.Desktop.Services;

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

/// <summary>High-level play-state bucket for a source game on this device.</summary>
public enum GamePlayStateKind
{
    /// <summary>GroupKind=extras — not playable. Show extras badge only.</summary>
    Extras,

    /// <summary>GroupKind=unknown — classification failed. Show warning badge.</summary>
    NotClassified,

    /// <summary>GroupKind=packed and no install record on this device → show Install.</summary>
    NotInstalled,

    /// <summary>GroupKind=packed, installed, Windows PC platform → direct exe launch.</summary>
    InstalledDirect,

    /// <summary>GroupKind=packed, installed, emulated platform → emulator launch.</summary>
    InstalledEmulated,

    /// <summary>GroupKind=self_contained, Windows PC platform → direct exe launch.</summary>
    PlainDirect,

    /// <summary>GroupKind=self_contained, emulated platform → emulator launch.</summary>
    PlainEmulated,
}

/// <summary>Emulator readiness for platforms that require emulation.</summary>
public enum EmulatorAvailability
{
    /// <summary>Platform runs natively — emulation not applicable.</summary>
    NotRequired,

    /// <summary>No emulator is installed or configured for this platform.</summary>
    NotConfigured,

    /// <summary>Emulator is configured but a required BIOS file is absent.</summary>
    BiosMissing,

    /// <summary>Emulator is configured and all required BIOS files are present.</summary>
    Ready,
}

/// <summary>
/// Full computed play state for a single source game on this device.
/// Drives the game-detail Play section button logic.
/// </summary>
public sealed class SourceGamePlayState
{
    // ── Primary state ────────────────────────────────────────────────────────

    /// <summary>Determines which button(s) to show in the game detail view.</summary>
    public GamePlayStateKind Kind { get; init; }

    /// <summary>
    /// For emulated states: whether emulation is ready, needs setup, or needs BIOS.
    /// <see cref="EmulatorAvailability.NotRequired"/> for direct-launch states.
    /// </summary>
    public EmulatorAvailability EmulatorState { get; init; } = EmulatorAvailability.NotRequired;

    // ── Launch payloads ──────────────────────────────────────────────────────

    /// <summary>
    /// For direct-launch states: resolved path to the executable.
    /// Null when not applicable or when the exe could not be auto-detected.
    /// </summary>
    public string? LaunchPath { get; init; }

    /// <summary>
    /// For installed states: root directory of the game installation.
    /// Null when not applicable.
    /// </summary>
    public string? InstallPath { get; init; }

    // ── Emulator details ─────────────────────────────────────────────────────

    /// <summary>
    /// For emulated states: configs available for this platform, ordered by priority.
    /// The UI shows these as the chevron-expanded config picker.
    /// </summary>
    public IReadOnlyList<EmulatorConfig> AvailableConfigs { get; init; } = [];

    /// <summary>
    /// For <see cref="EmulatorAvailability.BiosMissing"/>: which BIOS files are needed.
    /// Null when not applicable.
    /// </summary>
    public BiosCheckResult? BiosCheck { get; init; }

    // ── Action availability ──────────────────────────────────────────────────

    /// <summary>
    /// True when an Uninstall action is valid — only for games that MGA installed
    /// (not for user-located installs and never for plain-files games).
    /// </summary>
    public bool CanUninstall { get; init; }

    /// <summary>
    /// True when a Hard Delete action is valid — only for filesystem-backed sources
    /// on this device where server confirms eligibility.
    /// </summary>
    public bool CanHardDelete { get; init; }
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

/// <summary>
/// Computes the local play state for a source game by combining server-side
/// data (GroupKind, Platform, files, source type) with device-local state
/// (emulator installs, emulator configs, BIOS files, game install records).
///
/// Results should be cached by the caller for the duration of the session;
/// re-run after any config change (new emulator, BIOS placed, game installed).
///
/// RAII: no async work in the constructor.
/// </summary>
public sealed class GameStateService
{
    // Platforms that require an emulator rather than a direct native launch.
    // Mirrors the server-side emulatedPlatforms map in classify.go.
    private static readonly HashSet<string> s_emulatedPlatforms =
        new(StringComparer.OrdinalIgnoreCase)
        {
            "nes", "snes", "gb", "gbc", "gba", "n64",
            "genesis", "sega_master_system", "game_gear", "sega_cd", "sega_32x",
            "ps1", "ps2", "ps3", "psp", "xbox_360",
            "arcade", "ms_dos", "scummvm",
        };

    private readonly EmulatorService        _emulators;
    private readonly EmulatorCatalogService _catalog;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public GameStateService(EmulatorService emulators, EmulatorCatalogService catalog)
    {
        _emulators = emulators;
        _catalog   = catalog;
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Asynchronously computes the play state for one source game on this device.
    ///
    /// This involves file-system I/O for BIOS verification; results should be
    /// cached for the session to avoid repeated disk reads.
    /// </summary>
    public async Task<SourceGamePlayState> ComputeAsync(
        SourceGameSummary sg,
        string            canonicalPlatform,
        CancellationToken ct = default)
    {
        var platform  = ResolveEffectivePlatform(sg.Platform, canonicalPlatform);
        var groupKind = (sg.GroupKind ?? string.Empty).ToLowerInvariant();
        var isEmulated = s_emulatedPlatforms.Contains(platform);

        // Hard-delete is only available on file-backed sources that the server marks eligible.
        bool canHardDelete = !string.IsNullOrEmpty(sg.RootPath)
                          && sg.HardDelete?.Eligible == true;

        return groupKind switch
        {
            "extras"  => new SourceGamePlayState
                         {
                             Kind          = GamePlayStateKind.Extras,
                             CanHardDelete = canHardDelete,
                         },

            "unknown" => new SourceGamePlayState
                         {
                             Kind          = GamePlayStateKind.NotClassified,
                             CanHardDelete = canHardDelete,
                         },

            "packed"  => await ComputePackedAsync(sg, platform, isEmulated, canHardDelete, ct)
                             .ConfigureAwait(false),

            // "self_contained" and any unrecognised value → treat as plain files.
            _         => await ComputeSelfContainedAsync(sg, platform, isEmulated, canHardDelete, ct)
                             .ConfigureAwait(false),
        };
    }

    // ---------------------------------------------------------------------------
    // Private — packed games
    // ---------------------------------------------------------------------------

    private async Task<SourceGamePlayState> ComputePackedAsync(
        SourceGameSummary sg,
        string            platform,
        bool              isEmulated,
        bool              canHardDelete,
        CancellationToken ct)
    {
        var record = _emulators.GetGameInstall(sg.Id);

        // No install record → prompt to install.
        if (record is null)
        {
            return new SourceGamePlayState
            {
                Kind          = GamePlayStateKind.NotInstalled,
                CanHardDelete = canHardDelete,
            };
        }

        if (!isEmulated)
        {
            // Installed Windows/PC game: launch the exe directly.
            return new SourceGamePlayState
            {
                Kind         = GamePlayStateKind.InstalledDirect,
                LaunchPath   = record.DetectedExePath ?? record.InstallPath,
                InstallPath  = record.InstallPath,
                CanUninstall = !record.UserLocated,
                CanHardDelete = canHardDelete,
            };
        }

        // Installed emulated game: go through emulator + BIOS checks.
        return await BuildEmulatedStateAsync(
            platform,
            launchPath:    record.InstallPath,
            stateKind:     GamePlayStateKind.InstalledEmulated,
            canUninstall:  !record.UserLocated,
            canHardDelete: canHardDelete,
            ct:            ct)
            .ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Private — self-contained (plain file) games
    // ---------------------------------------------------------------------------

    private async Task<SourceGamePlayState> ComputeSelfContainedAsync(
        SourceGameSummary sg,
        string            platform,
        bool              isEmulated,
        bool              canHardDelete,
        CancellationToken ct)
    {
        if (!isEmulated)
        {
            // Plain Windows game: resolve the root exe from the file list.
            return new SourceGamePlayState
            {
                Kind          = GamePlayStateKind.PlainDirect,
                LaunchPath    = ResolveRootExePath(sg),
                CanHardDelete = canHardDelete,
            };
        }

        // Plain ROM/disc image: go through emulator + BIOS checks.
        return await BuildEmulatedStateAsync(
            platform,
            launchPath:    null,   // launch path determined at runtime (root file)
            stateKind:     GamePlayStateKind.PlainEmulated,
            canUninstall:  false,  // no separate install; only hard delete applies
            canHardDelete: canHardDelete,
            ct:            ct)
            .ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------------
    // Private — emulator + BIOS resolution
    // ---------------------------------------------------------------------------

    private async Task<SourceGamePlayState> BuildEmulatedStateAsync(
        string            platform,
        string?           launchPath,
        GamePlayStateKind stateKind,
        bool              canUninstall,
        bool              canHardDelete,
        CancellationToken ct)
    {
        var configs = _emulators.GetConfigsForPlatform(platform);

        if (configs.Count == 0)
        {
            // No emulator set up for this platform.
            return new SourceGamePlayState
            {
                Kind             = stateKind,
                EmulatorState    = EmulatorAvailability.NotConfigured,
                LaunchPath       = launchPath,
                AvailableConfigs = configs,
                CanUninstall     = canUninstall,
                CanHardDelete    = canHardDelete,
            };
        }

        // Check BIOS state using the highest-priority config's catalog entry.
        var primaryConfig = configs[0];
        var install       = _emulators.GetInstall(primaryConfig.InstallId);
        var catalogEntry  = install?.CatalogId is not null
            ? _catalog.GetById(install.CatalogId)
            : null;

        BiosCheckResult? biosCheck = null;
        if (catalogEntry is not null)
        {
            biosCheck = await _emulators.CheckBiosAsync(catalogEntry, platform, ct)
                .ConfigureAwait(false);
        }

        var emulatorState = (biosCheck is null || biosCheck.AllRequiredPresent)
            ? EmulatorAvailability.Ready
            : EmulatorAvailability.BiosMissing;

        return new SourceGamePlayState
        {
            Kind             = stateKind,
            EmulatorState    = emulatorState,
            LaunchPath       = launchPath,
            AvailableConfigs = configs,
            BiosCheck        = biosCheck,
            CanUninstall     = canUninstall,
            CanHardDelete    = canHardDelete,
        };
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Resolves the effective platform for a source game, inheriting the canonical
    /// platform when the source has no platform of its own ("unknown" or empty).
    /// This mirrors server-side <c>EffectiveBrowserPlayPlatform</c> logic.
    /// </summary>
    private static string ResolveEffectivePlatform(string? sourcePlatform, string canonicalPlatform)
    {
        return string.IsNullOrEmpty(sourcePlatform) || sourcePlatform == "unknown"
            ? canonicalPlatform
            : sourcePlatform;
    }

    /// <summary>
    /// Attempts to resolve an absolute exe path for a self-contained Windows game
    /// by finding the root-role file in the source game's file list.
    /// Returns null when no root file is identified or the source has no file list.
    /// </summary>
    private static string? ResolveRootExePath(SourceGameSummary sg)
    {
        if (string.IsNullOrEmpty(sg.RootPath) || sg.Files is null || sg.Files.Count == 0)
            return null;

        var root = sg.Files.FirstOrDefault(f =>
            string.Equals(f.Role, "root", StringComparison.OrdinalIgnoreCase));

        if (root is null || string.IsNullOrEmpty(root.Path))
            return null;

        // Join the server-side root path with the relative file path.
        var relative = root.Path.TrimStart('/', '\\').Replace('/', Path.DirectorySeparatorChar);
        return Path.Combine(sg.RootPath, relative);
    }
}
