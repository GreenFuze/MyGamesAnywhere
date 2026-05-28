using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Desktop.Services;

// ---------------------------------------------------------------------------
// Config model
// ---------------------------------------------------------------------------

public sealed class ServerProfile
{
    public string Name { get; set; } = string.Empty;
    public string Url { get; set; } = string.Empty;
}

/// <summary>Persisted emulator entry in config.json.</summary>
public sealed class EmulatorEntry
{
    public string Id             { get; set; } = Guid.NewGuid().ToString();
    public string Name           { get; set; } = string.Empty;
    public string ExecutablePath { get; set; } = string.Empty;
    public string Platforms      { get; set; } = string.Empty;
    public string ArgsTemplate   { get; set; } = "{rom}";
}

public sealed class AppConfig
{
    public List<ServerProfile> Servers { get; set; } = [];
    public string ActiveServer { get; set; } = string.Empty;
    public string ThemeId { get; set; } = "midnight";
    public bool SidebarCollapsed { get; set; } = false;
    public List<EmulatorEntry> Emulators { get; set; } = [];

    /// <summary>
    /// Selected gamer profile ID from the First-Run Wizard.
    /// Empty string means "no profile selected yet".
    /// </summary>
    public string GamerProfileId { get; set; } = string.Empty;

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

    /// <summary>Returns the list of configured emulators (never null).</summary>
    public List<EmulatorEntry> GetEmulators() => _config.Emulators;

    /// <summary>Replaces the emulator list and persists config to disk.</summary>
    public void SetEmulators(List<EmulatorEntry> emulators)
    {
        _config.Emulators = emulators;
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
