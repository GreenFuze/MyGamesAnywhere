using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Library page — full game collection with live text search.
///
/// FilteredGames is recomputed whenever SearchText or Games changes so
/// the view always reflects the current filter without extra plumbing.
/// </summary>
public sealed partial class LibraryViewModel : ViewModelBase
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
    private string _searchText = string.Empty;

    [ObservableProperty]
    private int _totalCount;

    // ---------------------------------------------------------------------------
    // Derived state
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Subset of Games matching the current SearchText.
    /// Recomputed every time SearchText or Games changes.
    /// </summary>
    public ObservableCollection<GameCardModel> FilteredGames { get; } = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public LibraryViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnSearchTextChanged(string value) => RebuildFilteredGames();

    partial void OnGamesChanged(ObservableCollection<GameCardModel> value) => RebuildFilteredGames();

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
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 500)
                                            .ConfigureAwait(true);

            var cards = response.Games.Select(ToCard).ToList();
            Games      = new ObservableCollection<GameCardModel>(cards);
            TotalCount = response.Total;
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load library", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void RebuildFilteredGames()
    {
        FilteredGames.Clear();

        var filter = SearchText.Trim();

        var matches = string.IsNullOrEmpty(filter)
            ? Games
            : new ObservableCollection<GameCardModel>(
                Games.Where(g =>
                    g.Title.Contains(filter, StringComparison.OrdinalIgnoreCase) ||
                    g.Platform.Contains(filter, StringComparison.OrdinalIgnoreCase)));

        foreach (var card in matches)
            FilteredGames.Add(card);
    }

    private GameCardModel ToCard(MGA.Api.GameDetail g)
    {
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
