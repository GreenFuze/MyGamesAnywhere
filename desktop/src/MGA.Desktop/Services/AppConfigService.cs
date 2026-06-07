using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Desktop.Services;

// ---------------------------------------------------------------------------
// Config model — simple values
// ---------------------------------------------------------------------------

public sealed class ServerProfile
{
    public string Name { get; set; } = string.Empty;
    public string Url { get; set; } = string.Empty;
}

// ---------------------------------------------------------------------------
// Emulator config models
// ---------------------------------------------------------------------------

/// <summary>
/// Records a single emulator executable installed on this device.
/// MGA-managed installs (downloaded by MGA) are distinguished from
/// user-located installs by the <see cref="MgaManaged"/> flag.
/// </summary>
public sealed class EmulatorInstallRecord
{
    /// <summary>Unique ID for this install (stable across renames / moves).</summary>
    public string Id { get; set; } = Guid.NewGuid().ToString();

    /// <summary>
    /// Matching ID in the emulator catalog (<c>Assets/Data/emulators.json</c>).
    /// Null for user-defined installs that are not in the catalog.
    /// </summary>
    public string? CatalogId { get; set; }

    /// <summary>Human-readable display name (from catalog or user-provided).</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>Absolute path to the emulator executable on this device.</summary>
    public string ExecutablePath { get; set; } = string.Empty;

    /// <summary>True when MGA downloaded and installed this emulator automatically.</summary>
    public bool MgaManaged { get; set; }

    /// <summary>Detected or reported version string (may be empty).</summary>
    public string Version { get; set; } = string.Empty;

    /// <summary>UTC timestamp of when this record was created.</summary>
    public DateTime InstalledAt { get; set; } = DateTime.UtcNow;
}

/// <summary>
/// A specific launch configuration for an emulator — one install can have
/// multiple configs (e.g. "RetroArch — Standard" and "RetroArch — RetroAchievements").
/// </summary>
public sealed class EmulatorConfig
{
    /// <summary>Unique ID for this configuration.</summary>
    public string Id { get; set; } = Guid.NewGuid().ToString();

    /// <summary>References <see cref="EmulatorInstallRecord.Id"/>.</summary>
    public string InstallId { get; set; } = string.Empty;

    /// <summary>User-visible label shown in the config picker, e.g. "RetroArch — RA".</summary>
    public string DisplayName { get; set; } = string.Empty;

    /// <summary>
    /// Platform IDs this config handles (server-side Platform strings, e.g. "ps1", "snes").
    /// A config can cover multiple platforms (e.g. RetroArch handles all retro consoles).
    /// </summary>
    public List<string> Platforms { get; set; } = [];

    /// <summary>
    /// When true, this config should use RetroAchievements-enabled execution.
    /// Only shown in the picker when a RetroAchievements integration is configured.
    /// </summary>
    public bool RetroAchievementsEnabled { get; set; }

    /// <summary>
    /// For RetroArch and similar multi-core frontends:
    /// maps platform ID → core filename (e.g. "ps1" → "mednafen_psx_hw_libretro.dll").
    /// </summary>
    public Dictionary<string, string> CorePerPlatform { get; set; } = [];

    /// <summary>
    /// Custom launch args template for this config, overriding the emulator default.
    /// Null means use the emulator's <see cref="EmulatorCatalogEntry.DefaultArgsTemplate"/>.
    /// Placeholders: {rom}, {core}, {gamedir}, {conf}.
    /// </summary>
    public string? ArgsTemplate { get; set; }

    /// <summary>
    /// Sort order among configs that cover the same platform.
    /// Lower value = higher priority = shown first.
    /// </summary>
    public int Priority { get; set; }
}

/// <summary>
/// Records a packed game that has been installed on this device, either by MGA
/// running the installer/extractor, or by the user pointing MGA to an existing installation.
/// </summary>
public sealed class GameInstallRecord
{
    /// <summary>References the <c>source_game.id</c> from the MGA server.</summary>
    public string SourceGameId { get; set; } = string.Empty;

    /// <summary>Root directory of the installed game on this device.</summary>
    public string InstallPath { get; set; } = string.Empty;

    /// <summary>
    /// Detected or user-confirmed executable path.
    /// Null when MGA could not auto-detect the exe (user may confirm later).
    /// </summary>
    public string? DetectedExePath { get; set; }

    /// <summary>UTC timestamp when this record was created.</summary>
    public DateTime InstalledAt { get; set; } = DateTime.UtcNow;

    /// <summary>
    /// True when the user pointed MGA to an existing installation (rather than MGA
    /// running the installer). Affects whether MGA offers an Uninstall action —
    /// we only uninstall what we installed ourselves.
    /// </summary>
    public bool UserLocated { get; set; }
}

// ---------------------------------------------------------------------------
// Top-level AppConfig
// ---------------------------------------------------------------------------

public sealed class AppConfig
{
    // ── Server connection ────────────────────────────────────────────────────
    public List<ServerProfile> Servers { get; set; } = [];
    public string ActiveServer { get; set; } = string.Empty;

    // ── Appearance ───────────────────────────────────────────────────────────
    public string ThemeId { get; set; } = "midnight";
    public bool SidebarCollapsed { get; set; } = false;

    // ── Identity ─────────────────────────────────────────────────────────────
    /// <summary>
    /// Selected gamer profile ID from the First-Run Wizard.
    /// Empty string means "no profile selected yet".
    /// </summary>
    public string GamerProfileId { get; set; } = string.Empty;

    // ── Library paths (device-local, not profile-shared) ─────────────────────
    /// <summary>
    /// Default directory where packed games are installed on this device.
    /// Empty = use <c>{DataDirectory}/library</c>.
    /// </summary>
    public string LibraryDirectory { get; set; } = string.Empty;

    /// <summary>
    /// Directory where MGA stores managed BIOS files on this device.
    /// Empty = use <c>{DataDirectory}/bios</c>.
    /// </summary>
    public string BiosDirectory { get; set; } = string.Empty;

    // ── Emulator management (device-local) ───────────────────────────────────
    /// <summary>Emulator executables installed or located on this device.</summary>
    public List<EmulatorInstallRecord> EmulatorInstalls { get; set; } = [];

    /// <summary>Launch configurations derived from installed emulators.</summary>
    public List<EmulatorConfig> EmulatorConfigs { get; set; } = [];

    // ── Game install records (device-local) ──────────────────────────────────
    /// <summary>
    /// Packed games that have been installed on this device.
    /// Only present for "GroupKind=packed" source games.
    /// </summary>
    public List<GameInstallRecord> GameInstalls { get; set; } = [];

    // ── Computed ─────────────────────────────────────────────────────────────
    /// <summary>
    /// True when the wizard should show: either there is no server configured,
    /// or a server is configured but no gamer profile has been selected yet.
    /// </summary>
    [JsonIgnore]
    public bool IsFirstRun =>
        Servers.Count == 0 ||
        string.IsNullOrWhiteSpace(ActiveServer) ||
        string.IsNullOrWhiteSpace(GamerProfileId);
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

/// <summary>
/// Reads and writes the desktop-local config.json.
///
/// <b>Portable mode</b> (dev / uninstalled): if <c>config.json</c> already exists
/// next to the executable, or a <c>.mga-portable</c> marker file exists there, the
/// config is stored beside the exe. This is the behaviour when running from the
/// build output directory during development.
///
/// <b>Installed mode</b>: config lives in the OS user-data directory:
///   Windows: %APPDATA%\MGA\config.json
///   macOS:   ~/Library/Application Support/MGA/config.json
///   Linux:   ~/.config/mga/config.json
///
/// RAII — the config directory is created (if needed) in the constructor.
/// </summary>
public sealed class AppConfigService
{
    private static readonly JsonSerializerOptions s_writeOptions = new()
    {
        WriteIndented = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    private readonly string _configPath;
    private readonly string _dataDirectory;
    private AppConfig _config;

    public AppConfigService()
    {
        (_configPath, _dataDirectory) = ResolveConfigPath();

        // Ensure the directory exists (RAII).
        Directory.CreateDirectory(_dataDirectory);

        // Load immediately.
        _config = TryLoad() ?? new AppConfig();
    }

    /// <summary>
    /// Test constructor — uses a caller-specified path instead of platform defaults.
    /// Does not create directories; the caller is responsible for the path.
    /// </summary>
    internal AppConfigService(string configPath)
    {
        _configPath    = configPath;
        _dataDirectory = Path.GetDirectoryName(configPath)!;
        _config        = TryLoad() ?? new AppConfig();
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    public AppConfig Config => _config;

    /// <summary>
    /// Base directory for all MGA local data files (same folder as config.json).
    /// Other services should use this for caching, logs, etc. instead of
    /// re-deriving %APPDATA% themselves.
    /// </summary>
    public string DataDirectory => _dataDirectory;

    public void Update(Action<AppConfig> mutate)
    {
        mutate(_config);
        Persist();
    }

    /// <summary>Persists the current in-memory config to disk.</summary>
    public void Save() => Persist();

    // ---------------------------------------------------------------------------
    // Portable vs. installed path resolution
    // ---------------------------------------------------------------------------

    private static (string configPath, string dataDir) ResolveConfigPath()
    {
        // The directory that contains the running executable.
        var exeDir = AppContext.BaseDirectory;

        var portableConfig = Path.Combine(exeDir, "config.json");
        var portableMarker = Path.Combine(exeDir, ".mga-portable");

        // Portable if: config.json already exists beside the exe (first run in a portable dir),
        //              OR a .mga-portable sentinel file is present.
        if (File.Exists(portableConfig) || File.Exists(portableMarker))
            return (portableConfig, exeDir);

        // Installed mode: use the OS user-data directory.
        var appData = GetPlatformDataDirectory();
        return (Path.Combine(appData, "config.json"), appData);
    }

    private static string GetPlatformDataDirectory()
    {
        if (OperatingSystem.IsWindows())
            return Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "MGA");

        if (OperatingSystem.IsMacOS())
            return Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.Personal),
                "Library", "Application Support", "MGA");

        // Linux / other POSIX
        var xdgConfig = Environment.GetEnvironmentVariable("XDG_CONFIG_HOME");
        var configHome = string.IsNullOrWhiteSpace(xdgConfig)
            ? Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Personal), ".config")
            : xdgConfig;

        return Path.Combine(configHome, "mga");
    }

    // ---------------------------------------------------------------------------
    // Private
    // ---------------------------------------------------------------------------

    private AppConfig? TryLoad()
    {
        if (!File.Exists(_configPath))
            return null;

        try
        {
            var json = File.ReadAllText(_configPath);
            return JsonSerializer.Deserialize<AppConfig>(json);
        }
        catch
        {
            // Corrupt or unreadable config — start fresh rather than crashing.
            return null;
        }
    }

    private void Persist()
    {
        try
        {
            var json = JsonSerializer.Serialize(_config, s_writeOptions);
            File.WriteAllText(_configPath, json);
        }
        catch
        {
            // Non-fatal: in-memory state is always up-to-date even if persist fails.
        }
    }
}
