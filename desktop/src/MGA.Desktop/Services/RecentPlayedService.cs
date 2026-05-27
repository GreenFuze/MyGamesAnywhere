using System.Text.Json;

namespace MGA.Desktop.Services;

/// <summary>
/// Persists a recently-played game list to %APPDATA%\MGA\recent-played.json.
/// Thread-safe via lock; max 20 entries, newest-first.
/// </summary>
public sealed class RecentPlayedService
{
    private const int MaxEntries = 20;

    private readonly string _filePath;
    private readonly object _lock = new();

    public record RecentPlayedEntry(
        string   GameId,
        string   Title,
        string?  CoverUrl,
        DateTime PlayedAt);

    public RecentPlayedService(AppConfigService config)
    {
        _filePath = Path.Combine(config.DataDirectory, "recent-played.json");
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    public IReadOnlyList<RecentPlayedEntry> GetEntries()
    {
        lock (_lock)
        {
            return LoadFromDisk();
        }
    }

    public void RecordPlay(string gameId, string title, string? coverUrl = null)
    {
        lock (_lock)
        {
            var entries = LoadFromDisk().ToList();

            // Remove any existing entry for this game so it moves to the top.
            entries.RemoveAll(e => e.GameId == gameId);

            // Prepend newest.
            entries.Insert(0, new RecentPlayedEntry(gameId, title, coverUrl, DateTime.UtcNow));

            // Trim to max.
            if (entries.Count > MaxEntries)
                entries = entries.Take(MaxEntries).ToList();

            SaveToDisk(entries);
        }
    }

    public void RemoveEntry(string gameId)
    {
        lock (_lock)
        {
            var entries = LoadFromDisk()
                .Where(e => e.GameId != gameId)
                .ToList();
            SaveToDisk(entries);
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private List<RecentPlayedEntry> LoadFromDisk()
    {
        if (!File.Exists(_filePath))
            return [];

        try
        {
            var json = File.ReadAllText(_filePath);
            return JsonSerializer.Deserialize<List<RecentPlayedEntry>>(json) ?? [];
        }
        catch
        {
            return [];
        }
    }

    private void SaveToDisk(List<RecentPlayedEntry> entries)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(_filePath)!);
        var json = JsonSerializer.Serialize(entries, new JsonSerializerOptions { WriteIndented = true });
        File.WriteAllText(_filePath, json);
    }
}
