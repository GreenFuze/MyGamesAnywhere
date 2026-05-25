using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Display model for a single game row in the achievements game list.
/// Plain class — no change notifications needed.
/// </summary>
public sealed class AchievementGameRowModel
{
    public string  GameId      { get; init; } = string.Empty;
    public string  Title       { get; init; } = string.Empty;
    public string? CoverUrl    { get; init; }
    public int     Total       { get; init; }
    public int     Unlocked    { get; init; }
    public string  PercentText { get; init; } = string.Empty;
}

/// <summary>
/// Achievements page — overall progress summary, per-source breakdown,
/// and per-game unlock list with a refresh action.
///
/// Loaded on construction via _ = LoadAsync().
/// </summary>
public sealed partial class AchievementsViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Loading state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    // ---------------------------------------------------------------------------
    // Summary stats
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _totalAchievements;

    [ObservableProperty]
    private int _unlockedAchievements;

    /// <summary>Human-readable unlock percentage, e.g. "42%".</summary>
    [ObservableProperty]
    private string _unlockPercent = "0%";

    // ---------------------------------------------------------------------------
    // Collections
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<AchievementSystemRowModel> _systems = [];

    [ObservableProperty]
    private ObservableCollection<AchievementGameRowModel> _games = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public AchievementsViewModel(
        ServerConnectionService server,
        ToastService            toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Kicks off a server-side achievements refresh job, then reloads the dashboard.</summary>
    [RelayCommand]
    private async Task RefreshAsync()
    {
        if (_server.Api is null)
            return;

        try
        {
            await _server.Api.StartAchievementsRefreshAsync().ConfigureAwait(true);
            _toast.Info("Refresh started", "The server is refreshing achievements in the background.");

            // Reload the dashboard so the UI reflects the updated state.
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Refresh failed", ex.Message);
        }
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
            var dashboard = await _server.Api.GetAchievementsDashboardAsync().ConfigureAwait(true);

            // Populate summary totals.
            TotalAchievements    = dashboard.Totals.TotalCount;
            UnlockedAchievements = dashboard.Totals.UnlockedCount;
            UnlockPercent        = FormatPercent(dashboard.Totals.UnlockedCount, dashboard.Totals.TotalCount);

            // Populate systems collection.
            Systems = new ObservableCollection<AchievementSystemRowModel>(
                dashboard.Systems.Select(s => new AchievementSystemRowModel
                {
                    Source       = s.Source,
                    Total        = s.TotalCount,
                    Unlocked     = s.UnlockedCount,
                    PercentText  = FormatPercent(s.UnlockedCount, s.TotalCount),
                    TotalPoints  = s.TotalPoints,
                    EarnedPoints = s.EarnedPoints,
                }));

            // Populate games collection.
            Games = new ObservableCollection<AchievementGameRowModel>(
                dashboard.Games.Select(entry =>
                {
                    // Aggregate totals across all sources for this game.
                    int total    = entry.Systems.Sum(s => s.TotalCount);
                    int unlocked = entry.Systems.Sum(s => s.UnlockedCount);

                    // Resolve cover URL.
                    var coverMedia = entry.Game.CoverOverride
                                     ?? entry.Game.Media.FirstOrDefault(m => m.Type == "cover");

                    string? coverUrl = coverMedia is not null && _server.Api is not null
                        ? _server.Api.GetMediaUrl(coverMedia.Url)
                        : null;

                    return new AchievementGameRowModel
                    {
                        GameId      = entry.Game.Id,
                        Title       = entry.Game.Title,
                        CoverUrl    = coverUrl,
                        Total       = total,
                        Unlocked    = unlocked,
                        PercentText = FormatPercent(unlocked, total),
                    };
                }));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load achievements", ex.Message);
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
