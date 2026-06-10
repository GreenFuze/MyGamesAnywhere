using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Library page — full game collection with live text search, advanced filter bar
/// (multi-select platform/genre, developer, publisher, integration, year range,
/// favorites toggle), sort order, view mode cycling (Grid / List / Timeline / Shelf),
/// and a Scan Library button.
///
/// FilteredGames is recomputed whenever any filter/sort property changes.
/// TimelineGroups and ShelfRows are rebuilt automatically from FilteredGames.
/// </summary>
public sealed partial class LibraryViewModel : ViewModelBase
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
    // Sort options
    // ---------------------------------------------------------------------------

    public string[] SortOptions { get; } =
    [
        "Title (A–Z)",
        "Title (Z–A)",
        "Platform",
        "Developer",
        "Year (newest)",
        "Year (oldest)",
    ];

    // ---------------------------------------------------------------------------
    // Observable state — loading / scanning / view mode
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isScanning;

    [ObservableProperty]
    private int _totalCount;

    /// <summary>Active display mode. Cycles Grid → List → Timeline → Shelf.</summary>
    [ObservableProperty]
    private ViewMode _viewMode = ViewMode.Grid;

    // ---------------------------------------------------------------------------
    // Observable state — all loaded games (source for rebuilds)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<GameCardModel> _games = [];

    // ---------------------------------------------------------------------------
    // Observable state — filter bar
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _searchText = string.Empty;

    /// <summary>Multi-select platform options; each item fires RebuildFilteredGames on change.</summary>
    public ObservableCollection<FilterOptionModel> PlatformOptions { get; } = [];

    /// <summary>Multi-select genre options; each item fires RebuildFilteredGames on change.</summary>
    public ObservableCollection<FilterOptionModel> GenreOptions { get; } = [];

    [ObservableProperty]
    private ObservableCollection<string> _developers = ["All Developers"];

    [ObservableProperty]
    private string _selectedDeveloper = "All Developers";

    [ObservableProperty]
    private ObservableCollection<string> _publishers = ["All Publishers"];

    [ObservableProperty]
    private string _selectedPublisher = "All Publishers";

    [ObservableProperty]
    private ObservableCollection<string> _integrations = ["All Sources"];

    [ObservableProperty]
    private string _selectedIntegration = "All Sources";

    [ObservableProperty]
    private string _yearFrom = string.Empty;

    [ObservableProperty]
    private string _yearTo = string.Empty;

    [ObservableProperty]
    private bool _showFavoritesOnly;

    [ObservableProperty]
    private int _selectedSortIndex;

    // ---------------------------------------------------------------------------
    // Derived state — live-filtered, sorted subset
    // ---------------------------------------------------------------------------

    public ObservableCollection<GameCardModel>           FilteredGames  { get; } = [];
    public ObservableCollection<TimelineYearGroupViewModel> TimelineGroups { get; } = [];
    public ObservableCollection<ShelfRowViewModel>       ShelfRows      { get; } = [];

    // ---------------------------------------------------------------------------
    // Derived — filter bar labels
    // ---------------------------------------------------------------------------

    /// <summary>Label for the Platforms multi-select button, e.g. "Platforms (3)" or "Platforms".</summary>
    public string SelectedPlatformsText
    {
        get
        {
            var count = PlatformOptions.Count(p => p.IsSelected);
            return count == 0 ? "Platforms" : $"Platforms ({count})";
        }
    }

    /// <summary>Label for the Genres multi-select button, e.g. "Genres (2)" or "Genres".</summary>
    public string SelectedGenresText
    {
        get
        {
            var count = GenreOptions.Count(g => g.IsSelected);
            return count == 0 ? "Genres" : $"Genres ({count})";
        }
    }

    // ---------------------------------------------------------------------------
    // Derived — view visibility + mode toggle label
    // ---------------------------------------------------------------------------

    public bool ShowGridView     => !IsLoading && ViewMode == ViewMode.Grid;
    public bool ShowListView     => !IsLoading && ViewMode == ViewMode.List;
    public bool ShowTimelineView => !IsLoading && ViewMode == ViewMode.Timeline;
    public bool ShowShelfView    => !IsLoading && ViewMode == ViewMode.Shelf;

    /// <summary>
    /// Label for the view-mode cycle button — shows what the NEXT mode will be,
    /// so the user can see what clicking will switch TO.
    /// </summary>
    public string ViewModeNextLabel => ViewMode switch
    {
        ViewMode.Grid     => "☰ List",
        ViewMode.List     => "⊞ Timeline",
        ViewMode.Timeline => "▤ Shelf",
        _                 => "▦ Grid",
    };

    // ---------------------------------------------------------------------------
    // Bulk-select state
    // ---------------------------------------------------------------------------

    /// <summary>Whether the library is in multi-select mode (checkboxes visible on cards).</summary>
    [ObservableProperty]
    private bool _isBulkSelectMode;

    /// <summary>Number of currently selected games.</summary>
    public int SelectedCount => FilteredGames.Count(g => g.IsSelected);

    /// <summary>True when at least one game is selected — enables the Delete/Reclassify buttons.</summary>
    public bool HasSelectedGames => SelectedCount > 0;

    /// <summary>Label for the Delete button, e.g. "Delete 3 games".</summary>
    public string BulkDeleteLabel => $"Delete {SelectedCount} game{(SelectedCount == 1 ? "" : "s")}";

    /// <summary>Label for the Reclassify button, e.g. "Reclassify 3 games".</summary>
    public string BulkReclassifyLabel => $"Reclassify {SelectedCount} game{(SelectedCount == 1 ? "" : "s")}";

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public LibraryViewModel(
        ServerConnectionService  server,
        NavigationService        nav,
        ToastService             toast,
        AppConfigService         config,
        string?                  initialSearch    = null,
        InstallDetectionService? installDetector  = null,
        RecentPlayedService?     recentPlayed     = null,
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

        if (!string.IsNullOrEmpty(initialSearch))
            SearchText = initialSearch;

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
                        _gameCache?.Invalidate();  // Force re-fetch after scan.
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
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnSearchTextChanged(string value)          => RebuildFilteredGames();
    partial void OnSelectedDeveloperChanged(string value)   => RebuildFilteredGames();
    partial void OnSelectedPublisherChanged(string value)   => RebuildFilteredGames();
    partial void OnSelectedIntegrationChanged(string value) => RebuildFilteredGames();
    partial void OnYearFromChanged(string value)            => RebuildFilteredGames();
    partial void OnYearToChanged(string value)              => RebuildFilteredGames();
    partial void OnSelectedSortIndexChanged(int value)      => RebuildFilteredGames();
    partial void OnShowFavoritesOnlyChanged(bool value)     => RebuildFilteredGames();

    partial void OnGamesChanged(ObservableCollection<GameCardModel> value)
    {
        RebuildPlatformOptions();
        RebuildGenreOptions();
        RebuildDevelopers();
        RebuildPublishers();
        RebuildIntegrations();
        RebuildFilteredGames();
    }

    partial void OnViewModeChanged(ViewMode value)
    {
        OnPropertyChanged(nameof(ShowGridView));
        OnPropertyChanged(nameof(ShowListView));
        OnPropertyChanged(nameof(ShowTimelineView));
        OnPropertyChanged(nameof(ShowShelfView));
        OnPropertyChanged(nameof(ViewModeNextLabel));
    }

    partial void OnIsBulkSelectModeChanged(bool value)
    {
        // Clear all selections when exiting bulk-select mode.
        if (!value)
            foreach (var g in FilteredGames)
                g.IsSelected = false;

        RefreshBulkCounters();
    }

    partial void OnIsLoadingChanged(bool value)
    {
        OnPropertyChanged(nameof(ShowGridView));
        OnPropertyChanged(nameof(ShowListView));
        OnPropertyChanged(nameof(ShowTimelineView));
        OnPropertyChanged(nameof(ShowShelfView));
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private void ToggleFavoritesOnly() => ShowFavoritesOnly = !ShowFavoritesOnly;

    /// <summary>Cycles the view mode: Grid → List → Timeline → Shelf → Grid.</summary>
    [RelayCommand]
    private void ToggleViewMode() => ViewMode = ViewMode switch
    {
        ViewMode.Grid     => ViewMode.List,
        ViewMode.List     => ViewMode.Timeline,
        ViewMode.Timeline => ViewMode.Shelf,
        _                 => ViewMode.Grid,
    };

    /// <summary>Clears all active filter selections back to defaults.</summary>
    [RelayCommand]
    private void ClearFilters()
    {
        SearchText            = string.Empty;
        SelectedDeveloper     = "All Developers";
        SelectedPublisher     = "All Publishers";
        SelectedIntegration   = "All Sources";
        YearFrom              = string.Empty;
        YearTo                = string.Empty;
        ShowFavoritesOnly     = false;
        SelectedSortIndex     = 0;

        foreach (var p in PlatformOptions) p.IsSelected = false;
        foreach (var g in GenreOptions)    g.IsSelected = false;

        // Button labels update immediately.
        OnPropertyChanged(nameof(SelectedPlatformsText));
        OnPropertyChanged(nameof(SelectedGenresText));
    }

    [RelayCommand]
    private void OpenGame(string gameId)
    {
        _nav.NavigateTo(new GameDetailViewModel(
            gameId, _server, _nav, _toast, _config,
            _installDetector, _recentPlayed, gameStateService: _gameStateService));
    }

    [RelayCommand]
    private async Task ScanAsync()
    {
        if (_server.Api is null) return;

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
    }

    /// <summary>Enters or exits bulk-select mode.</summary>
    [RelayCommand]
    private void ToggleBulkSelectMode() => IsBulkSelectMode = !IsBulkSelectMode;

    /// <summary>Selects every visible (filtered) game card.</summary>
    [RelayCommand]
    private void SelectAll()
    {
        foreach (var g in FilteredGames)
            g.IsSelected = true;
        RefreshBulkCounters();
    }

    /// <summary>Deselects all game cards without leaving bulk-select mode.</summary>
    [RelayCommand]
    private void ClearSelection()
    {
        foreach (var g in FilteredGames)
            g.IsSelected = false;
        RefreshBulkCounters();
    }

    /// <summary>
    /// Hard-deletes all source games of every selected canonical game.
    /// Builds the batch items array from <see cref="GameCardModel.SourceGameIds"/>
    /// (populated at load time) so no extra API round-trips are needed.
    /// </summary>
    [RelayCommand]
    private async Task BulkHardDeleteAsync()
    {
        if (_server.Api is null) return;

        var selected = FilteredGames.Where(g => g.IsSelected).ToList();
        if (selected.Count == 0) return;

        // Build {canonical_game_id, source_game_id} pairs for every source of each selected game.
        var items = selected
            .SelectMany(game => game.SourceGameIds
                .Select(sid => new MGA.Api.DeleteSourceGameBatchItem
                {
                    CanonicalGameId = game.Id,
                    SourceGameId    = sid,
                }))
            .ToList();

        if (items.Count == 0)
        {
            _toast.Info("Nothing to delete", "Selected games have no source records.");
            return;
        }

        try
        {
            var result = await _server.Api.DeleteSourcesBatchAsync(items).ConfigureAwait(true);

            var deleted = result.DeletedSourceGameIds.Count;
            _toast.Success("Deleted", $"{deleted} source record{(deleted == 1 ? "" : "s")} removed.");

            // Exit bulk-select mode and refresh the library.
            IsBulkSelectMode = false;
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Bulk delete failed", ex.Message);
        }
    }

    // ---------------------------------------------------------------------------
    // Private — bulk-select counter refresh
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Reclassifies all selected canonical games: clears canonical pins for all
    /// their source games so the server's automatic grouping algorithm can re-classify them.
    /// Games that can't be re-matched automatically will surface in Undetected Games.
    /// </summary>
    [RelayCommand]
    private async Task BulkReclassifyAsync()
    {
        if (_server.Api is null) return;

        var selected = FilteredGames.Where(g => g.IsSelected).ToList();
        if (selected.Count == 0) return;

        // Gather (canonicalId, sourceId) pairs for all sources of all selected games.
        var pairs = selected
            .SelectMany(game => game.SourceGameIds
                .Select(sid => (CanonicalId: game.Id, SourceId: sid)))
            .ToList();

        if (pairs.Count == 0)
        {
            _toast.Info("Nothing to reclassify", "Selected games have no source records.");
            return;
        }

        int cleared = 0;
        int errors  = 0;

        foreach (var (canonicalId, sourceId) in pairs)
        {
            try
            {
                await _server.Api.ClearCanonicalPinAsync(canonicalId, sourceId)
                                  .ConfigureAwait(true);
                cleared++;
            }
            catch
            {
                errors++;
            }
        }

        if (errors > 0)
            _toast.Warning("Reclassify partial", $"{cleared} source(s) cleared; {errors} failed.");
        else
            _toast.Success("Reclassified", $"{cleared} source record{(cleared == 1 ? "" : "s")} sent for re-detection.");

        // Exit bulk-select mode and refresh the library.
        IsBulkSelectMode = false;
        await LoadAsync().ConfigureAwait(true);
    }

    /// <summary>Notifies the view that SelectedCount / HasSelectedGames / label properties changed.</summary>
    internal void RefreshBulkCounters()
    {
        OnPropertyChanged(nameof(SelectedCount));
        OnPropertyChanged(nameof(HasSelectedGames));
        OnPropertyChanged(nameof(BulkDeleteLabel));
        OnPropertyChanged(nameof(BulkReclassifyLabel));
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        var serverUrl = _server.ActiveUrl;

        // ── Cache-first: render instantly if warm cache exists ──────────────
        if (_gameCache is not null && _gameCache.TryGet(serverUrl, out var cached))
        {
            // pageSize: 0 means "all games" — cached.Count equals the true server total.
            ApplyGames(cached, cached.Count);
            // Refresh silently in the background to pick up any new/removed games.
            _ = RefreshFromServerAsync(serverUrl);
            return;
        }

        // ── Cold load: show spinner ─────────────────────────────────────────
        IsLoading = true;

        try
        {
            // pageSize: 0 = "all games" (server cap: 20 000). Avoids silent truncation
            // and keeps cached.Count == response.Total so the header always shows the
            // correct total even on cache-first renders.
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 0)
                                            .ConfigureAwait(true);
            _gameCache?.Update(serverUrl, response.Games);
            ApplyGames(response.Games, response.Total);
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

    private async Task RefreshFromServerAsync(string serverUrl)
    {
        if (_server.Api is null) return;

        try
        {
            var response = await _server.Api.ListGamesAsync(page: 0, pageSize: 0)
                                            .ConfigureAwait(true);
            _gameCache?.Update(serverUrl, response.Games);
            ApplyGames(response.Games, response.Total);
        }
        catch
        {
            // Background refresh — silently ignore transient failures.
        }
    }

    private void ApplyGames(IReadOnlyList<MGA.Api.GameDetail> games, int total)
    {
        var cards = games.Select(ToCard).ToList();
        Games      = new ObservableCollection<GameCardModel>(cards);
        TotalCount = total;

        // Pre-warm the media cache in background: covers + preview art for all games.
        // On the next visit (or next launch) images come from disk instead of the network.
        if (_mediaCache is not null)
            _ = _mediaCache.WarmAsync(games.SelectMany(CollectMediaUrls).OfType<string>());
    }

    /// <summary>
    /// Collects raw remote media URLs (cover + preview) for a game.
    /// Used to pre-warm <see cref="MediaCacheService"/> after each game list load.
    /// </summary>
    private IEnumerable<string?> CollectMediaUrls(MGA.Api.GameDetail g)
    {
        if (_server.Api is null) yield break;
        var api = _server.Api;

        var cover = g.CoverOverride ?? g.Media.FirstOrDefault(m => m.Type == "cover");
        if (cover is not null) yield return api.GetMediaUrl(cover.Url);

        var preview = g.Media.FirstOrDefault(m =>
            m.Type is "hover" or "background" or "screenshot" or "header");
        if (preview is not null) yield return api.GetMediaUrl(preview.Url);
    }

    // ---------------------------------------------------------------------------
    // Private — option list rebuilds
    // ---------------------------------------------------------------------------

    private void RebuildPlatformOptions()
    {
        var previouslySelected = PlatformOptions
            .Where(p => p.IsSelected)
            .Select(p => p.Name)
            .ToHashSet();

        PlatformOptions.Clear();

        foreach (var name in Games.Select(g => g.Platform)
                                  .Where(p => !string.IsNullOrEmpty(p))
                                  .Distinct()
                                  .OrderBy(p => p))
        {
            var opt = new FilterOptionModel(name, OnPlatformOptionChanged);
            if (previouslySelected.Contains(name)) opt.IsSelected = true;
            PlatformOptions.Add(opt);
        }

        OnPropertyChanged(nameof(SelectedPlatformsText));
    }

    private void RebuildGenreOptions()
    {
        var previouslySelected = GenreOptions
            .Where(g => g.IsSelected)
            .Select(g => g.Name)
            .ToHashSet();

        GenreOptions.Clear();

        foreach (var name in Games.SelectMany(g => g.Genres)
                                  .Where(g => !string.IsNullOrEmpty(g))
                                  .Distinct()
                                  .OrderBy(g => g))
        {
            var opt = new FilterOptionModel(name, OnGenreOptionChanged);
            if (previouslySelected.Contains(name)) opt.IsSelected = true;
            GenreOptions.Add(opt);
        }

        OnPropertyChanged(nameof(SelectedGenresText));
    }

    private void RebuildDevelopers()
    {
        var distinct = Games.Select(g => g.Developer)
                            .Where(d => !string.IsNullOrEmpty(d))
                            .Distinct()
                            .OrderBy(d => d)
                            .ToList();

        Developers.Clear();
        Developers.Add("All Developers");
        foreach (var d in distinct) Developers.Add(d);

        if (!Developers.Contains(SelectedDeveloper))
            SelectedDeveloper = "All Developers";
    }

    private void RebuildPublishers()
    {
        var distinct = Games.Select(g => g.Publisher)
                            .Where(p => !string.IsNullOrEmpty(p))
                            .Distinct()
                            .OrderBy(p => p)
                            .ToList();

        Publishers.Clear();
        Publishers.Add("All Publishers");
        foreach (var p in distinct) Publishers.Add(p);

        if (!Publishers.Contains(SelectedPublisher))
            SelectedPublisher = "All Publishers";
    }

    private void RebuildIntegrations()
    {
        var distinct = Games.Select(g => g.IntegrationLabel)
                            .Where(i => !string.IsNullOrEmpty(i))
                            .Distinct()
                            .OrderBy(i => i)
                            .ToList();

        Integrations.Clear();
        Integrations.Add("All Sources");
        foreach (var i in distinct) Integrations.Add(i);

        if (!Integrations.Contains(SelectedIntegration))
            SelectedIntegration = "All Sources";
    }

    // ---------------------------------------------------------------------------
    // Private — filter application + derived view rebuilds
    // ---------------------------------------------------------------------------

    private void OnPlatformOptionChanged()
    {
        OnPropertyChanged(nameof(SelectedPlatformsText));
        RebuildFilteredGames();
    }

    private void OnGenreOptionChanged()
    {
        OnPropertyChanged(nameof(SelectedGenresText));
        RebuildFilteredGames();
    }

    private void RebuildFilteredGames()
    {
        var selectedPlatforms = PlatformOptions
            .Where(p => p.IsSelected)
            .Select(p => p.Name)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

        var selectedGenres = GenreOptions
            .Where(g => g.IsSelected)
            .Select(g => g.Name)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

        var criteria = new FilterCriteria
        {
            SearchText    = SearchText,
            Platforms     = selectedPlatforms,
            Genres        = selectedGenres,
            Developer     = SelectedDeveloper == "All Developers" ? string.Empty : SelectedDeveloper,
            Publisher     = SelectedPublisher == "All Publishers" ? string.Empty : SelectedPublisher,
            Integration   = SelectedIntegration == "All Sources" ? string.Empty : SelectedIntegration,
            YearFrom      = int.TryParse(YearFrom, out var yf) ? yf : null,
            YearTo        = int.TryParse(YearTo,   out var yt) ? yt : null,
            FavoritesOnly = ShowFavoritesOnly,
            SortIndex     = SelectedSortIndex,
        };

        var filtered = LibraryFilter.Apply(Games, criteria).ToList();

        FilteredGames.Clear();
        foreach (var card in filtered)
            FilteredGames.Add(card);

        // Keep Timeline and Shelf collections in sync with the current filter result.
        RebuildTimelineGroups(filtered);
        RebuildShelfRows(filtered);
    }

    // ---------------------------------------------------------------------------
    // Private — Timeline view rebuild
    // ---------------------------------------------------------------------------

    private void RebuildTimelineGroups(IReadOnlyList<GameCardModel> source)
    {
        TimelineGroups.Clear();

        // Group by release year descending; sentinel / unknown years (0 or ≥ 3000)
        // are normalised to -1 so they sort below all real years.
        var groups = source
            .GroupBy(g => g.ReleaseYear)
            .OrderByDescending(g => (g.Key > 0 && g.Key < 3000) ? g.Key : -1);

        foreach (var grp in groups)
            TimelineGroups.Add(new TimelineYearGroupViewModel(grp.Key, grp));
    }

    // ---------------------------------------------------------------------------
    // Private — Shelf view rebuild
    // ---------------------------------------------------------------------------

    private void RebuildShelfRows(IReadOnlyList<GameCardModel> source)
    {
        ShelfRows.Clear();

        // "Favorites" shelf — only when there are favorited games.
        var favorites = source.Where(g => g.Favorite).ToList();
        if (favorites.Count > 0)
            ShelfRows.Add(new ShelfRowViewModel("Favorites", favorites));

        // Per-platform shelves — one row per platform, alphabetically sorted.
        var byPlatform = source
            .Where(g => !string.IsNullOrEmpty(g.Platform))
            .GroupBy(g => g.Platform, StringComparer.OrdinalIgnoreCase)
            .OrderBy(g => g.Key, StringComparer.OrdinalIgnoreCase);

        foreach (var grp in byPlatform)
            ShelfRows.Add(new ShelfRowViewModel(grp.Key, grp));
    }

    // ---------------------------------------------------------------------------
    // Private — DTO mapping
    // ---------------------------------------------------------------------------

    private GameCardModel ToCard(MGA.Api.GameDetail g) => new(g, _server.Api, RefreshBulkCounters, _mediaCache);
}
