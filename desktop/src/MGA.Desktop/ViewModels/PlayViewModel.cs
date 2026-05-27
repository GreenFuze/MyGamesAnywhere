using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Play page — recently played shelf and the launchable-games grid.
///
/// Only games with <see cref="GameCardModel.CanPlay"/> = true are shown in the
/// main grid. The RecentGames shelf shows the first 10 of those same games.
/// </summary>
public sealed partial class PlayViewModel : ViewModelBase
{
    private readonly ServerConnectionService    _server;
    private readonly NavigationService          _nav;
    private readonly ToastService               _toast;
    private readonly AppConfigService           _config;
    private readonly InstallDetectionService?   _installDetector;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    /// <summary>Only games with CanPlay = true — drives the main grid.</summary>
    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    [ObservableProperty]
    private int _launchableCount;

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _recentGames = [];

    /// <summary>True when the recently-played shelf has no games to show.</summary>
    [ObservableProperty]
    private bool _hasNoRecentGames = true;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public PlayViewModel(
        ServerConnectionService  server,
        NavigationService        nav,
        ToastService             toast,
        AppConfigService         config,
        InstallDetectionService? installDetector = null)
    {
        _server          = server;
        _nav             = nav;
        _toast           = toast;
        _config          = config;
        _installDetector = installDetector;

        // Start loading immediately — fire-and-forget with error handling inside.
        _ = LoadAsync();

        // Subscribe to library scan events so the grid refreshes automatically
        // and the user gets a heads-up toast while a scan is running.
        if (_server.Events is not null)
        {
            Disposables.Add(
                _server.Events.Of("scan_started")
                    .Subscribe(_ => _toast.Info("Library scan", "Scanning for games…")));

            Disposables.Add(
                _server.Events.Of("scan_complete")
                    .Subscribe(__ =>
                    {
                        _toast.Success("Scan complete", "Library updated.");
                        _ = LoadAsync();
                    }));

            Disposables.Add(
                _server.Events.Of("scan_error")
                    .Subscribe(_ => _toast.Error("Scan failed", "Check server logs for details.")));
        }
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Navigate to the full game detail page for the given game ID.</summary>
    [RelayCommand]
    private void OpenGame(string gameId)
    {
        _nav.NavigateTo(new GameDetailViewModel(
            gameId, _server, _nav, _toast, _config, _installDetector));
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
            // Fetch full game list (up to 500 for the initial view).
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 500)
                                            .ConfigureAwait(true);

            // Map API models → display models, keeping only launchable games.
            var allCards      = response.Games.Select(g => ToCard(g)).ToList();
            var launchable    = allCards.Where(c => c.CanPlay).ToList();

            Games           = new ObservableCollection<GameCardModel>(launchable);
            LaunchableCount = launchable.Count;

            // Recent shelf: first 10 launchable games.
            var recent = launchable.Take(10).ToList();
            RecentGames      = new ObservableCollection<GameCardModel>(recent);
            HasNoRecentGames = recent.Count == 0;

            // Kick off install detection in the background for all launchable games.
            // Each card's InstallStatus is updated individually as results arrive.
            StartDetectionAsync(launchable);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load games", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private GameCardModel ToCard(MGA.Api.GameDetail g) => new(g, _server.Api);

    /// <summary>
    /// Starts background install detection for the given cards.
    /// Subscribes to <see cref="InstallDetectionService.StatusUpdated"/> so that
    /// each result is marshalled back to the UI thread and applied to the correct card.
    /// </summary>
    private void StartDetectionAsync(List<GameCardModel> cards)
    {
        if (_installDetector is null) return;

        // Build lookup: gameId → card for O(1) updates.
        var cardById = cards.ToDictionary(c => c.Id);

        // Subscribe: receive each status update and apply it on the UI thread.
        Disposables.Add(
            _installDetector.StatusUpdated
                .Subscribe(evt =>
                {
                    if (!cardById.TryGetValue(evt.GameId, out var card)) return;
                    Avalonia.Threading.Dispatcher.UIThread.Post(
                        () => card.InstallStatus = evt.Status);
                }));

        // Fire off detection; results arrive via the subscription above.
        var gameInfos = cards.Select(c => (c.Id, c.Title, c.Sources));
        _ = Task.Run(() => _installDetector.DetectAllAsync(gameInfos));
    }
}
