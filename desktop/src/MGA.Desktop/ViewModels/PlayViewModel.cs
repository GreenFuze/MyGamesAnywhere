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

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _recentGames = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public PlayViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;

        // Start loading immediately — fire-and-forget with error handling inside.
        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Navigate to the full game detail page for the given game ID.</summary>
    [RelayCommand]
    private void OpenGame(string gameId)
    {
        _nav.NavigateTo(new GameDetailViewModel(gameId, _server, _nav, _toast));
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
            RecentGames = new ObservableCollection<GameCardModel>(recent);
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

    private GameCardModel ToCard(MGA.Api.GameDetail g)
    {
        // Prefer cover_override, then the first media asset of type "cover".
        var coverMedia = g.CoverOverride
            ?? g.Media.FirstOrDefault(m => m.Type == "cover");

        string? coverUrl = coverMedia is not null && _server.Api is not null
            ? _server.Api.GetMediaUrl(coverMedia.Url)
            : null;

        return new GameCardModel
        {
            Id       = g.Id,
            Title    = g.Title,
            Platform = g.Platform,
            CoverUrl = coverUrl,
            Favorite = g.Favorite,
            CanPlay  = g.Kind == "game",
        };
    }
}
