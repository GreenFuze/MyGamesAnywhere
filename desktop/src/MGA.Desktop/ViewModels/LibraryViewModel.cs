using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Library page — full game collection with live text search, platform filter,
/// sort order, favorites toggle, and a Scan Library button.
///
/// FilteredGames is recomputed whenever any filter/sort property changes.
/// </summary>
public sealed partial class LibraryViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;
    private readonly AppConfigService        _config;

    // ---------------------------------------------------------------------------
    // Sort options (constant; exposed as instance property for compiled bindings)
    // ---------------------------------------------------------------------------

    public string[] SortOptions { get; } = ["Title (A–Z)", "Title (Z–A)", "Platform", "Developer"];

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isScanning;

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    [ObservableProperty]
    private string _searchText = string.Empty;

    [ObservableProperty]
    private int _totalCount;

    // --- Filter / sort ---

    [ObservableProperty]
    private ObservableCollection<string> _platforms = ["All Platforms"];

    [ObservableProperty]
    private string _selectedPlatform = "All Platforms";

    [ObservableProperty]
    private ObservableCollection<string> _genres = ["All Genres"];

    [ObservableProperty]
    private string _selectedGenre = "All Genres";

    [ObservableProperty]
    private int _selectedSortIndex;

    [ObservableProperty]
    private bool _showFavoritesOnly;

    [ObservableProperty]
    private bool _isListView;

    // ---------------------------------------------------------------------------
    // Derived state — the live-filtered, sorted subset shown in the grid
    // ---------------------------------------------------------------------------

    public ObservableCollection<GameCardModel> FilteredGames { get; } = [];

    /// <summary>True when the grid view should be shown (not loading, not in list mode).</summary>
    public bool ShowGridView => !IsLoading && !IsListView;

    /// <summary>True when the list view should be shown (not loading, in list mode).</summary>
    public bool ShowListView => !IsLoading && IsListView;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public LibraryViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast,
        AppConfigService        config)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;
        _config = config;

        _ = LoadAsync();

        // Subscribe to library scan SSE events so the grid refreshes automatically.
        if (_server.Events is not null)
        {
            Disposables.Add(
                _server.Events.Of("scan_started")
                    .Subscribe(__ => _toast.Info("Library scan", "Scanning for games…")));

            Disposables.Add(
                _server.Events.Of("scan_complete")
                    .Subscribe(__ =>
                    {
                        IsScanning = false;
                        _toast.Success("Scan complete", "Library updated.");
                        _ = LoadAsync();
                    }));

            Disposables.Add(
                _server.Events.Of("scan_error")
                    .Subscribe(__ =>
                    {
                        IsScanning = false;
                        _toast.Error("Scan failed", "Check server logs for details.");
                    }));
        }
    }

    // ---------------------------------------------------------------------------
    // Property change hooks — trigger filter rebuild
    // ---------------------------------------------------------------------------

    partial void OnSearchTextChanged(string value)          => RebuildFilteredGames();
    partial void OnGamesChanged(ObservableCollection<GameCardModel> value)
    {
        RebuildPlatforms();
        RebuildGenres();
        RebuildFilteredGames();
    }
    partial void OnSelectedPlatformChanged(string value)    => RebuildFilteredGames();
    partial void OnSelectedGenreChanged(string value)       => RebuildFilteredGames();
    partial void OnSelectedSortIndexChanged(int value)      => RebuildFilteredGames();
    partial void OnShowFavoritesOnlyChanged(bool value)     => RebuildFilteredGames();
    partial void OnIsListViewChanged(bool value)
    {
        OnPropertyChanged(nameof(ShowGridView));
        OnPropertyChanged(nameof(ShowListView));
    }
    partial void OnIsLoadingChanged(bool value)
    {
        OnPropertyChanged(nameof(ShowGridView));
        OnPropertyChanged(nameof(ShowListView));
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Toggles the ShowFavoritesOnly filter.</summary>
    [RelayCommand]
    private void ToggleFavoritesOnly() => ShowFavoritesOnly = !ShowFavoritesOnly;

    /// <summary>Toggles between grid view and list view.</summary>
    [RelayCommand]
    private void ToggleViewMode() => IsListView = !IsListView;

    /// <summary>Navigate to the full game detail page for the given game ID.</summary>
    [RelayCommand]
    private void OpenGame(string gameId)
    {
        _nav.NavigateTo(new GameDetailViewModel(gameId, _server, _nav, _toast, _config));
    }

    /// <summary>Triggers a full library scan via POST /api/scan.</summary>
    [RelayCommand]
    private async Task ScanAsync()
    {
        if (_server.Api is null)
            return;

        IsScanning = true;

        try
        {
            await _server.Api.TriggerScanAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            IsScanning = false;
            _toast.Error("Scan failed to start", ex.Message);
        }
        // IsScanning reset to false when scan_complete / scan_error SSE fires.
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

    /// <summary>Rebuilds the Platforms dropdown from the currently loaded games.</summary>
    private void RebuildPlatforms()
    {
        var distinct = Games
            .Select(g => g.Platform)
            .Where(p => !string.IsNullOrEmpty(p))
            .Distinct()
            .OrderBy(p => p)
            .ToList();

        Platforms.Clear();
        Platforms.Add("All Platforms");
        foreach (var p in distinct)
            Platforms.Add(p);

        // If the previously selected platform is no longer present, reset.
        if (!Platforms.Contains(SelectedPlatform))
            SelectedPlatform = "All Platforms";
    }

    /// <summary>Rebuilds the Genres dropdown from the currently loaded games.</summary>
    private void RebuildGenres()
    {
        var distinct = Games
            .SelectMany(g => g.Genres)
            .Where(g => !string.IsNullOrEmpty(g))
            .Distinct()
            .OrderBy(g => g)
            .ToList();

        Genres.Clear();
        Genres.Add("All Genres");
        foreach (var g in distinct)
            Genres.Add(g);

        // If the previously selected genre is no longer present, reset.
        if (!Genres.Contains(SelectedGenre))
            SelectedGenre = "All Genres";
    }

    /// <summary>
    /// Applies all active filters (search, platform, favorites) and the selected
    /// sort order, then replaces the contents of FilteredGames.
    /// </summary>
    private void RebuildFilteredGames()
    {
        var filtered = LibraryFilter.Apply(
            Games,
            SearchText,
            SelectedPlatform,
            SelectedGenre,
            ShowFavoritesOnly,
            SelectedSortIndex);

        FilteredGames.Clear();
        foreach (var card in filtered)
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
            Id        = g.Id,
            Title     = g.Title,
            Platform  = g.Platform,
            CoverUrl  = coverUrl,
            Favorite  = g.Favorite,
            CanPlay   = g.Kind == "game",
            Kind      = g.Kind,
            Developer = g.Developer ?? string.Empty,
            Genres    = g.Genres,
        };
    }
}
