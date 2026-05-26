using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>Display row for one scan report entry.</summary>
public sealed class ScanReportRowViewModel
{
    public string Id           { get; init; } = string.Empty;
    public string StartedAt    { get; init; } = string.Empty;
    public string Duration     { get; init; } = string.Empty;
    public int    GamesAdded   { get; init; }
    public int    GamesRemoved { get; init; }
    public int    GamesUpdated { get; init; }
    public int    TotalGames   { get; init; }
    public bool   MetadataOnly { get; init; }
    public string Summary      { get; init; } = string.Empty;
}

/// <summary>
/// Stats page — Library and Gamer tabs.
///
/// Library tab: total game count, platform breakdown, genre breakdown.
/// Gamer tab: favorite count, achievement totals, per-source achievement systems.
///
/// Loaded on construction via _ = LoadAsync().
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
    // Tab selection (0 = Library, 1 = Gamer)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _selectedTabIndex;

    // ---------------------------------------------------------------------------
    // Library tab
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _totalGames;

    [ObservableProperty]
    private ObservableCollection<CountStatModel> _platformBreakdown = [];

    [ObservableProperty]
    private ObservableCollection<CountStatModel> _genreBreakdown = [];

    // ---------------------------------------------------------------------------
    // Gamer tab
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _favoriteGames;

    [ObservableProperty]
    private int _totalAchievements;

    [ObservableProperty]
    private int _unlockedAchievements;

    /// <summary>Human-readable unlock percentage, e.g. "42%".</summary>
    [ObservableProperty]
    private string _unlockPercent = "0%";

    [ObservableProperty]
    private ObservableCollection<AchievementSystemRowModel> _achievementSystems = [];

    // ---------------------------------------------------------------------------
    // Scan history
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
            // Fetch library, gamer stats, and scan reports in parallel.
            var libraryTask = _server.Api.GetLibraryStatisticsAsync();
            var gamerTask   = _server.Api.GetGamerStatisticsAsync();
            var reportsTask = _server.Api.ListScanReportsAsync(limit: 10);

            await Task.WhenAll(libraryTask, gamerTask, reportsTask).ConfigureAwait(true);

            var library = await libraryTask;
            var gamer   = await gamerTask;

            // Populate library tab.
            TotalGames = library.Summary.CanonicalGameCount;

            // Compute per-list maximums so bar widths are proportional within each group.
            int platformMax = library.Platforms.Count > 0 ? library.Platforms.Max(p => p.Count) : 1;
            int genreMax    = library.Genres.Count    > 0 ? library.Genres.Max(g => g.Count)    : 1;

            PlatformBreakdown = new ObservableCollection<CountStatModel>(
                library.Platforms.Select(p => new CountStatModel
                {
                    Label    = p.Label,
                    Count    = p.Count,
                    MaxCount = platformMax,
                }));

            GenreBreakdown = new ObservableCollection<CountStatModel>(
                library.Genres.Select(g => new CountStatModel
                {
                    Label    = g.Label,
                    Count    = g.Count,
                    MaxCount = genreMax,
                }));

            // Populate gamer tab.
            FavoriteGames        = gamer.FavoriteGames;
            TotalAchievements    = gamer.TotalAchievements;
            UnlockedAchievements = gamer.UnlockedAchievements;
            UnlockPercent        = PercentFormatter.Format(gamer.UnlockedAchievements, gamer.TotalAchievements);

            AchievementSystems = new ObservableCollection<AchievementSystemRowModel>(
                gamer.AchievementSystems.Select(s => new AchievementSystemRowModel
                {
                    Source      = s.Source,
                    Total       = s.TotalCount,
                    Unlocked    = s.UnlockedCount,
                    PercentText = PercentFormatter.Format(s.UnlockedCount, s.TotalCount),
                }));

            // Populate scan history.
            var reports = await reportsTask.ConfigureAwait(true);
            ScanReports = new ObservableCollection<ScanReportRowViewModel>(
                reports.Select(r =>
                {
                    var duration    = TimeSpan.FromMilliseconds(r.DurationMs);
                    var durationStr = duration.TotalMinutes >= 1
                        ? $"{(int)duration.TotalMinutes}m {duration.Seconds}s"
                        : $"{duration.Seconds}s";
                    return new ScanReportRowViewModel
                    {
                        Id           = r.Id,
                        StartedAt    = r.StartedAt,
                        Duration     = durationStr,
                        GamesAdded   = r.GamesAdded,
                        GamesRemoved = r.GamesRemoved,
                        GamesUpdated = r.GamesUpdated,
                        TotalGames   = r.TotalGames,
                        MetadataOnly = r.MetadataOnly,
                        Summary      = $"+{r.GamesAdded} added, −{r.GamesRemoved} removed, {r.GamesUpdated} updated",
                    };
                }));
            HasScanReports = ScanReports.Count > 0;
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

}
