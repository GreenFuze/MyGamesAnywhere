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

public sealed class AppConfig
{
    public List<ServerProfile> Servers { get; set; } = [];
    public string ActiveServer { get; set; } = string.Empty;
    public string ThemeId { get; set; } = "midnight";
    public bool SidebarCollapsed { get; set; } = false;

    [JsonIgnore]
    public bool IsFirstRun => Servers.Count == 0 || string.IsNullOrWhiteSpace(ActiveServer);
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

/// <summary>
/// Reads and writes the desktop-local config.json.
/// Path resolution (RAII — config directory is created in constructor):
///   Windows: %APPDATA%\MGA\config.json
///   macOS:   ~/Library/Application Support/MGA/config.json
///   Linux:   ~/.config/mga/config.json
/// </summary>
public sealed class AppConfigService
{
    private static readonly JsonSerializerOptions s_writeOptions = new()
    {
        WriteIndented = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    private readonly string _configPath;
    private AppConfig _config;

    public AppConfigService()
    {
        var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        var dir = Path.Combine(appData, "MGA");
        Directory.CreateDirectory(dir);
        _configPath = Path.Combine(dir, "config.json");

        // Load immediately (RAII).
        _config = TryLoad() ?? new AppConfig();
    }

    /// <summary>
    /// Test constructor — uses a caller-specified path instead of %APPDATA%.
    /// Does not create directories; the caller is responsible for the path.
    /// </summary>
    internal AppConfigService(string configPath)
    {
        _configPath = configPath;
        _config = TryLoad() ?? new AppConfig();
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    public AppConfig Config => _config;

    public void Update(Action<AppConfig> mutate)
    {
        mutate(_config);
        Persist();
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
