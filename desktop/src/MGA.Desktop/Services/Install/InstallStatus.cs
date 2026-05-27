namespace MGA.Desktop.Services.Install;

// ---------------------------------------------------------------------------
// SourceGameInfo
// ---------------------------------------------------------------------------

/// <summary>
/// Minimal per-source record extracted from <see cref="MGA.Api.SourceGameSummary"/>
/// and stored on <see cref="MGA.Desktop.ViewModels.GameCardModel"/> so that
/// <see cref="InstallDetectionService"/> can run without loading full game detail.
/// </summary>
public sealed record SourceGameInfo
{
    public string SourceGameId { get; init; } = string.Empty;
    public string PluginId     { get; init; } = string.Empty;

    /// <summary>
    /// Plugin-specific game identifier, e.g. Steam AppID "730", Epic AppName "Fortnite".
    /// </summary>
    public string ExternalId   { get; init; } = string.Empty;

    /// <summary>
    /// Server-side root path for file-backed sources (may be null for storefront sources).
    /// </summary>
    public string? RootPath    { get; init; }

    /// <summary>Human-readable label shown in the UI, e.g. "Steam", "Epic".</summary>
    public string? Label       { get; init; }
}

// ---------------------------------------------------------------------------
// InstallState enum
// ---------------------------------------------------------------------------

/// <summary>All possible install-detection outcomes for a game on this machine.</summary>
public enum InstallState
{
    /// <summary>Detection has not run yet.</summary>
    Unknown = 0,

    /// <summary>The game is fully installed and launchable.</summary>
    Installed,

    /// <summary>
    /// The game is in the user's storefront library but not downloaded/installed.
    /// The <see cref="InstallStatus.LaunchUri"/> can trigger a store install.
    /// </summary>
    NotInstalled,

    /// <summary>
    /// The storefront client (Steam, Epic Launcher, …) is not installed on this machine.
    /// </summary>
    ClientMissing,

    /// <summary>
    /// An emulator is configured for this platform but its executable is missing.
    /// </summary>
    EmulatorMissing,

    /// <summary>No emulator entry exists for this game's platform at all.</summary>
    EmulatorNotConfigured,

    /// <summary>
    /// The ROM/game files are stored on a remote MGA server that is not localhost.
    /// Download support is pending the server's download endpoint.
    /// </summary>
    RomRemote,

    /// <summary>
    /// ARP matched this game but executable detection was inconclusive.
    /// The user must confirm or provide the correct path.
    /// </summary>
    ManualBindNeeded,
}

// ---------------------------------------------------------------------------
// InstallStatus
// ---------------------------------------------------------------------------

/// <summary>
/// Rich install-detection result for a single game on this machine.
/// All properties are optional — only those relevant to the detected state are populated.
/// </summary>
public sealed record InstallStatus
{
    // ---------------------------------------------------------------------------
    // Sentinel
    // ---------------------------------------------------------------------------

    public static readonly InstallStatus NotChecked = new() { State = InstallState.Unknown };

    // ---------------------------------------------------------------------------
    // Core fields
    // ---------------------------------------------------------------------------

    public InstallState State { get; init; } = InstallState.Unknown;

    /// <summary>URI to launch the game, e.g. "steam://rungameid/730" or a full exe path.</summary>
    public string? LaunchUri { get; init; }

    /// <summary>URI or command to uninstall the game.</summary>
    public string? UninstallUri { get; init; }

    /// <summary>Root install directory, when known.</summary>
    public string? InstallPath { get; init; }

    /// <summary>Resolved game executable, when known (file-backed installs).</summary>
    public string? ExePath { get; init; }

    // ---------------------------------------------------------------------------
    // Client-missing fields
    // ---------------------------------------------------------------------------

    /// <summary>Human-readable message when the storefront client is not installed.</summary>
    public string? ClientMissingMessage { get; init; }

    /// <summary>URL to download the missing storefront client.</summary>
    public string? ClientDownloadUrl { get; init; }

    // ---------------------------------------------------------------------------
    // Emulator fields
    // ---------------------------------------------------------------------------

    public string? EmulatorName       { get; init; }
    public string? EmulatorMissingPath { get; init; }

    // ---------------------------------------------------------------------------
    // ARP detection quality
    // ---------------------------------------------------------------------------

    /// <summary>Fuzzy-match confidence for ARP-based matches (0.0 – 1.0).</summary>
    public double Confidence { get; init; } = 1.0;

    // ---------------------------------------------------------------------------
    // Convenience booleans
    // ---------------------------------------------------------------------------

    public bool IsInstalled          => State == InstallState.Installed;
    public bool CanInstall           => State == InstallState.NotInstalled;
    public bool NeedsClientInstall   => State == InstallState.ClientMissing;
    public bool NeedsEmulator        => State is InstallState.EmulatorMissing
                                                or InstallState.EmulatorNotConfigured;
    public bool NeedsManualBind      => State == InstallState.ManualBindNeeded;
    public bool IsRomRemote          => State == InstallState.RomRemote;
}
