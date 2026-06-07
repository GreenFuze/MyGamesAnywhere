using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// ScanReportRowViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Display model for one scan report row in the Stats → Scans tab.
/// The constructor handles duration formatting and summary-string generation.
/// </summary>
public sealed class ScanReportRowViewModel
{
    public string Id           { get; }
    public string StartedAt    { get; }
    public string Duration     { get; }
    public int    GamesAdded   { get; }
    public int    GamesRemoved { get; }
    public int    GamesUpdated { get; }
    public int    TotalGames   { get; }
    public bool   MetadataOnly { get; }

    /// <summary>One-line summary, e.g. "+3 added, −1 removed, 12 updated".</summary>
    public string Summary { get; }

    public ScanReportRowViewModel(ScanReport r)
    {
        var span = TimeSpan.FromMilliseconds(r.DurationMs);
        var durationStr = span.TotalMinutes >= 1
            ? $"{(int)span.TotalMinutes}m {span.Seconds}s"
            : $"{span.Seconds}s";

        Id           = r.Id;
        StartedAt    = DateTimeFormatter.FormatDateTime(r.StartedAt);
        Duration     = durationStr;
        GamesAdded   = r.GamesAdded;
        GamesRemoved = r.GamesRemoved;
        GamesUpdated = r.GamesUpdated;
        TotalGames   = r.TotalGames;
        MetadataOnly = r.MetadataOnly;
        Summary      = $"+{r.GamesAdded} added, −{r.GamesRemoved} removed, {r.GamesUpdated} updated";
    }
}

// ---------------------------------------------------------------------------
// StatTileModel
// ---------------------------------------------------------------------------

/// <summary>
/// Display model for one summary stat tile (icon + label + value).
/// Constructed directly from resolved values so the ViewModel stays clean.
/// </summary>
public sealed class StatTileModel
{
    /// <summary>Unicode icon character shown above the value.</summary>
    public string  Icon     { get; }

    /// <summary>Human-readable label, e.g. "Total Games".</summary>
    public string  Label    { get; }

    /// <summary>Primary value string, e.g. "1,234" or "890 / 2,100".</summary>
    public string  Value    { get; }

    /// <summary>Optional secondary line beneath the value, e.g. "42%".</summary>
    public string? SubText  { get; }

    public StatTileModel(string icon, string label, string value, string? subText = null)
    {
        Icon    = icon;
        Label   = label;
        Value   = value;
        SubText = subText;
    }
}

// ---------------------------------------------------------------------------
// CoverageStatModel
// ---------------------------------------------------------------------------

/// <summary>
/// Display model for one metadata-coverage row.
/// Maps from a <see cref="CoverageStat"/> API record and pre-formats the percentage.
/// </summary>
public sealed class CoverageStatModel
{
    public string Label       { get; }
    public int    Count       { get; }
    public double Percent     { get; }

    /// <summary>Formatted percentage string, e.g. "78.4%".</summary>
    public string PercentText { get; }

    public CoverageStatModel(CoverageStat stat)
    {
        Label       = stat.Label;
        Count       = stat.Count;
        Percent     = stat.Percent;
        PercentText = $"{stat.Percent:F1}%";
    }
}

// ---------------------------------------------------------------------------
// StatsViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Stats page — Library, Gamer, and Scans tabs.
///
/// Library tab: stat tiles, platform/kind/genre breakdowns, metadata coverage.
/// Gamer tab:   achievement totals and per-source breakdown.
/// Scans tab:   recent scan reports.
/// </summary>
public sealed partial class StatsViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Loading state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    // ---------------------------------------------------------------------------
    // Tab selection (0 = Library, 1 = Gamer, 2 = Scans)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _selectedTabIndex;

    // ---------------------------------------------------------------------------
    // Library tab
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _totalGames;

    /// <summary>Top-row summary tiles shown at the top of the Library tab.</summary>
    [ObservableProperty]
    private ObservableCollection<StatTileModel> _statTiles = [];

    [ObservableProperty]
    private ObservableCollection<CountStatModel> _platformBreakdown = [];

    [ObservableProperty]
    private ObservableCollection<CountStatModel> _kindBreakdown = [];

    [ObservableProperty]
    private ObservableCollection<CountStatModel> _genreBreakdown = [];

    [ObservableProperty]
    private ObservableCollection<CoverageStatModel> _coverage = [];

    [ObservableProperty]
    private bool _hasCoverage;

    /// <summary>Game count per integration/source, for the "By Source" bar chart.</summary>
    [ObservableProperty]
    private ObservableCollection<CountStatModel> _integrationBreakdown = [];

    [ObservableProperty]
    private bool _hasIntegrationBreakdown;

    // ---------------------------------------------------------------------------
    // Gamer tab
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _favoriteGames;

    [ObservableProperty]
    private int _totalAchievements;

    [ObservableProperty]
    private int _unlockedAchievements;

    [ObservableProperty]
    private string _unlockPercent = "0%";

    [ObservableProperty]
    private ObservableCollection<AchievementSystemRowModel> _achievementSystems = [];

    // ---------------------------------------------------------------------------
    // Scans tab
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<ScanReportRowViewModel> _scanReports = [];

    [ObservableProperty]
    private bool _hasScanReports;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public StatsViewModel(
        ServerConnectionService server,
        ToastService            toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null)
            return;

        IsLoading = true;

        try
        {
            var libraryTask    = _server.Api.GetLibraryStatisticsAsync();
            var gamerTask      = _server.Api.GetGamerStatisticsAsync();
            var reportsTask    = _server.Api.ListScanReportsAsync(limit: 10);
            var simpleStatsTask = _server.Api.GetLibraryStatsAsync();

            await Task.WhenAll(libraryTask, gamerTask, reportsTask, simpleStatsTask).ConfigureAwait(true);

            var library     = await libraryTask;
            var gamer       = await gamerTask;
            var reports     = await reportsTask;
            var simpleStats = await simpleStatsTask;

            // Populate totals used by both tabs.
            TotalGames           = library.Summary.CanonicalGameCount;
            FavoriteGames        = gamer.FavoriteGames;
            TotalAchievements    = gamer.TotalAchievements;
            UnlockedAchievements = gamer.UnlockedAchievements;
            UnlockPercent        = PercentFormatter.Format(gamer.UnlockedAchievements, gamer.TotalAchievements);

            // Stat tiles — combine library + gamer data.
            BuildStatTiles(library, gamer);

            // Breakdown bar charts — each uses the group maximum to normalise bar widths.
            int platformMax = library.Platforms.Count > 0 ? library.Platforms.Max(p => p.Count) : 1;
            int kindMax     = library.Kinds.Count     > 0 ? library.Kinds.Max(k => k.Count)     : 1;
            int genreMax    = library.Genres.Count    > 0 ? library.Genres.Max(g => g.Count)    : 1;

            PlatformBreakdown = new ObservableCollection<CountStatModel>(
                library.Platforms.Select(p => new CountStatModel(
                    new CountStat { Label = PlatformHelper.FormatPlatform(p.Label), Count = p.Count },
                    platformMax)));

            KindBreakdown = new ObservableCollection<CountStatModel>(
                library.Kinds.Select(k => new CountStatModel(
                    new CountStat { Label = PrettifyLabel(k.Label), Count = k.Count },
                    kindMax)));

            GenreBreakdown = new ObservableCollection<CountStatModel>(
                library.Genres.Select(g => new CountStatModel(g, genreMax)));

            // Metadata coverage tiles.
            Coverage     = new ObservableCollection<CoverageStatModel>(
                library.Coverage.Select(c => new CoverageStatModel(c)));
            HasCoverage  = Coverage.Count > 0;

            // Gamer achievement systems.
            AchievementSystems = new ObservableCollection<AchievementSystemRowModel>(
                gamer.AchievementSystems.Select(s => new AchievementSystemRowModel(s)));

            // Scan history.
            ScanReports    = new ObservableCollection<ScanReportRowViewModel>(
                reports.Select(r => new ScanReportRowViewModel(r)));
            HasScanReports = ScanReports.Count > 0;

            // Integration / "By Source" breakdown from the simple stats endpoint.
            if (simpleStats.ByIntegration.Count > 0)
            {
                int intMax = simpleStats.ByIntegration.Values.Max();
                IntegrationBreakdown = new ObservableCollection<CountStatModel>(
                    simpleStats.ByIntegration
                        .OrderByDescending(kv => kv.Value)
                        .Select(kv => new CountStatModel(
                            new CountStat { Label = PrettifyLabel(kv.Key), Count = kv.Value }, intMax)));
            }
            HasIntegrationBreakdown = IntegrationBreakdown.Count > 0;
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load stats", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Converts raw API slug labels (e.g. "base_game", "xbox_360") to
    /// human-readable title-case strings (e.g. "Base Game", "Xbox 360").
    /// </summary>
    private static string PrettifyLabel(string? label) =>
        string.IsNullOrEmpty(label)
            ? string.Empty
            : System.Globalization.CultureInfo.CurrentCulture.TextInfo
                  .ToTitleCase(label.Replace('_', ' '));

    /// <summary>
    /// Builds the four summary stat tiles from combined library + gamer data.
    /// Keeps the construction logic inside the ViewModel rather than scattering it.
    /// </summary>
    private void BuildStatTiles(LibraryStatistics library, GamerStatistics gamer)
    {
        // Cover art coverage percentage from the library coverage list.
        var coverStat  = library.Coverage.FirstOrDefault(c => c.Key == "cover");
        string coverPct = coverStat is not null ? $"{coverStat.Percent:F0}% covered" : string.Empty;

        // Achievement unlock fraction.
        string achieveValue = gamer.TotalAchievements > 0
            ? $"{gamer.UnlockedAchievements:N0} / {gamer.TotalAchievements:N0}"
            : "—";

        StatTiles = new ObservableCollection<StatTileModel>
        {
            new("▤",  "Total Games",  $"{library.Summary.CanonicalGameCount:N0}"),
            new("★",  "Favorites",    $"{gamer.FavoriteGames:N0}"),
            new("🏆", "Achievements", achieveValue,  UnlockPercent),
            new("🎨", "Cover Art",    coverPct.Length > 0 ? coverPct : "—"),
        };
    }
}
