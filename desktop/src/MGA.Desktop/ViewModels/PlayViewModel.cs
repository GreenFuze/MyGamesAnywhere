using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Install;
using MGA.Desktop.Services.Emulation;

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
    private readonly RecentPlayedService?       _recentPlayed;
    private readonly GameCacheService?          _gameCache;
    private readonly MediaCacheService?         _mediaCache;
    private readonly GameStateService?          _gameStateService;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    /// <summary>First game in the full list — drives the hero banner at the top of Play.</summary>
    [ObservableProperty]
    private GameCardModel? _featuredGame;

    /// <summary>Only games with CanPlay = true — drives the main grid.</summary>
    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    [ObservableProperty]
    private int _gameCount;

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
        InstallDetectionService? installDetector = null,
        RecentPlayedService?     recentPlayed    = null,
        GameCacheService?        gameCache        = null,
        MediaCacheService?       mediaCache       = null,
        GameStateService?        gameStateService = null)
    {
        _server          = server;
        _nav             = nav;
        _toast           = toast;
        _config          = config;
        _installDetector = installDetector;
        _recentPlayed    = recentPlayed;
        _gameCache        = gameCache;
        _mediaCache       = mediaCache;
        _gameStateService = gameStateService;

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
            gameId, _server, _nav, _toast, _config,
            _installDetector, _recentPlayed, gameStateService: _gameStateService));
    }

    /// <summary>Removes an entry from the recent-played history and reloads the shelf.</summary>
    [RelayCommand]
    private async Task RemoveRecentEntry(string gameId)
    {
        _recentPlayed?.RemoveEntry(gameId);

        // Reload so the shelf reflects the removal immediately.
        await LoadAsync().ConfigureAwait(true);
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null)
            return;

        var serverUrl = _server.ActiveUrl;

        // ── Cache-first: render immediately if we have a warm cache ──────────
        if (_gameCache is not null && _gameCache.TryGet(serverUrl, out var cached))
        {
            ApplyGames(cached);
            // Still refresh in the background (don't show spinner for cache hits).
            _ = RefreshFromServerAsync(serverUrl);
            return;
        }

        // ── Cold load: show spinner while we fetch ─────────────────────────
        IsLoading = true;

        try
        {
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 500)
                                            .ConfigureAwait(true);
            _gameCache?.Update(serverUrl, response.Games);
            ApplyGames(response.Games);
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

    /// <summary>
    /// Silently refreshes from the server without showing a loading spinner.
    /// Used when the cache was served immediately; runs in the background.
    /// </summary>
    private async Task RefreshFromServerAsync(string serverUrl)
    {
        if (_server.Api is null)
            return;

        try
        {
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 500)
                                            .ConfigureAwait(true);
            _gameCache?.Update(serverUrl, response.Games);
            ApplyGames(response.Games);
        }
        catch
        {
            // Background refresh — silently ignore transient failures.
            // The user already sees cached data; no need for an error toast.
        }
    }

    /// <summary>
    /// Maps a raw game list onto the observable properties and kicks off
    /// install detection.  Safe to call from both cold-load and cache paths.
    /// </summary>
    private void ApplyGames(IReadOnlyList<MGA.Api.GameDetail> games)
    {
        var allCards = games.Select(g => ToCard(g)).ToList();

        // Show the full game collection — all titles are "playable" in the
        // sense that they can be launched (PC exe, emulated ROM, or browser).
        Games        = new ObservableCollection<GameCardModel>(allCards);
        GameCount    = allCards.Count;
        FeaturedGame = allCards.FirstOrDefault();

        // Recent shelf: use actual play history, fall back to first 10 games.
        if (_recentPlayed is not null)
        {
            var history  = _recentPlayed.GetEntries();
            var byId     = allCards.ToDictionary(c => c.Id);
            var fromHistory = history
                .Select(e => byId.GetValueOrDefault(e.GameId))
                .OfType<GameCardModel>()
                .Take(10)
                .ToList();

            if (fromHistory.Count > 0)
            {
                RecentGames      = new ObservableCollection<GameCardModel>(fromHistory);
                HasNoRecentGames = false;
            }
            else
            {
                var fallback = allCards.Take(10).ToList();
                RecentGames      = new ObservableCollection<GameCardModel>(fallback);
                HasNoRecentGames = fallback.Count == 0;
            }
        }
        else
        {
            var recent = allCards.Take(10).ToList();
            RecentGames      = new ObservableCollection<GameCardModel>(recent);
            HasNoRecentGames = recent.Count == 0;
        }

        StartDetectionAsync(allCards);
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private GameCardModel ToCard(MGA.Api.GameDetail g) => new(g, _server.Api, mediaCache: _mediaCache);

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
