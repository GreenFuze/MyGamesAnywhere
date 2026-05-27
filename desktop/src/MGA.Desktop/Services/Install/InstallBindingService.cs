using System.Text.Json;

namespace MGA.Desktop.Services.Install;

/// <summary>
/// Persists the user-confirmed executable paths that override auto-detection.
///
/// Stored at <c>%APPDATA%\MGA\install-bindings.json</c> as a JSON dictionary
/// of canonical-game-ID → absolute exe path.
///
/// <see cref="InstallDetectionService"/> checks this before any other strategy,
/// so a user-confirmed path always wins over heuristic detection.
/// </summary>
public sealed class InstallBindingService
{
    private readonly string _filePath;
    private Dictionary<string, string> _bindings = [];

    public InstallBindingService()
    {
        var dir   = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "MGA");
        _filePath = Path.Combine(dir, "install-bindings.json");

        Load();
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns the manually bound exe path for <paramref name="gameId"/>,
    /// or <see langword="null"/> when no override has been set.
    /// </summary>
    public string? GetBinding(string gameId) =>
        _bindings.TryGetValue(gameId, out var path) ? path : null;

    /// <summary>
    /// Stores (or replaces) the exe path for <paramref name="gameId"/>
    /// and writes the file immediately.
    /// </summary>
    public void SetBinding(string gameId, string exePath)
    {
        _bindings[gameId] = exePath;
        Save();
    }

    /// <summary>
    /// Removes the override for <paramref name="gameId"/> (reverts to auto-detection)
    /// and writes the file immediately.
    /// </summary>
    public void RemoveBinding(string gameId)
    {
        if (_bindings.Remove(gameId))
            Save();
    }

    // ---------------------------------------------------------------------------
    // Private — persistence
    // ---------------------------------------------------------------------------

    private void Load()
    {
        try
        {
            if (!File.Exists(_filePath))
                return;

            var json = File.ReadAllText(_filePath);
            _bindings = JsonSerializer.Deserialize<Dictionary<string, string>>(json) ?? [];
        }
        catch
        {
            // Best-effort — corrupt file means start fresh.
            _bindings = [];
        }
    }

    private void Save()
    {
        try
        {
            var dir = Path.GetDirectoryName(_filePath)!;
            Directory.CreateDirectory(dir);

            var json = JsonSerializer.Serialize(_bindings,
                new JsonSerializerOptions { WriteIndented = true });
            File.WriteAllText(_filePath, json);
        }
        catch
        {
            // Best-effort — if disk write fails, keep in-memory state and continue.
        }
    }
}
