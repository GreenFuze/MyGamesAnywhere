using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Play page — recently played shelf and the full game grid.
///
/// Loads all games from the server on construction.  The RecentGames shelf
/// shows the first 10 games that have a play record (CanPlay = true).
/// </summary>
public sealed partial class PlayViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;
    private readonly AppConfigService        _config;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _recentGames = [];

    /// <summary>True when the recently-played shelf has no games to show.</summary>
    [ObservableProperty]
    private bool _hasNoRecentGames = true;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public PlayViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast,
        AppConfigService        config)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;
        _config = config;

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
        _nav.NavigateTo(new GameDetailViewModel(gameId, _server, _nav, _toast, _config));
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

            // Map API models → display models.
            var cards = response.Games.Select(g => ToCard(g)).ToList();

            Games = new ObservableCollection<GameCardModel>(cards);

            // Recent shelf: first 10 games that can be played.
            var recent = cards.Where(c => c.CanPlay).Take(10).ToList();
            RecentGames      = new ObservableCollection<GameCardModel>(recent);
            HasNoRecentGames = recent.Count == 0;
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
}
