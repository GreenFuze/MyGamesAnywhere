using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

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
            // Fetch library and gamer stats in parallel.
            var libraryTask = _server.Api.GetLibraryStatisticsAsync();
            var gamerTask   = _server.Api.GetGamerStatisticsAsync();

            await Task.WhenAll(libraryTask, gamerTask).ConfigureAwait(true);

            var library = await libraryTask;
            var gamer   = await gamerTask;

            // Populate library tab.
            TotalGames = library.Summary.CanonicalGameCount;

            PlatformBreakdown = new ObservableCollection<CountStatModel>(
                library.Platforms.Select(p => new CountStatModel
                {
                    Label = p.Label,
                    Count = p.Count,
                }));

            GenreBreakdown = new ObservableCollection<CountStatModel>(
                library.Genres.Select(g => new CountStatModel
                {
                    Label = g.Label,
                    Count = g.Count,
                }));

            // Populate gamer tab.
            FavoriteGames        = gamer.FavoriteGames;
            TotalAchievements    = gamer.TotalAchievements;
            UnlockedAchievements = gamer.UnlockedAchievements;
            UnlockPercent        = FormatPercent(gamer.UnlockedAchievements, gamer.TotalAchievements);

            AchievementSystems = new ObservableCollection<AchievementSystemRowModel>(
                gamer.AchievementSystems.Select(s => new AchievementSystemRowModel
                {
                    Source      = s.Source,
                    Total       = s.TotalCount,
                    Unlocked    = s.UnlockedCount,
                    PercentText = FormatPercent(s.UnlockedCount, s.TotalCount),
                }));
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

    private static string FormatPercent(int unlocked, int total)
        => total > 0
            ? $"{(int)Math.Round(unlocked * 100.0 / total)}%"
            : "0%";
}
