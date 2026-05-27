using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// Display models — each handles its own mapping from API types
// ---------------------------------------------------------------------------

/// <summary>
/// Single media item (screenshot or header) for the detail-page carousel.
/// Resolves the absolute URL through the API service on construction.
/// </summary>
public sealed class MediaItemModel
{
    public string Url { get; }

    public MediaItemModel(GameMedia m, MgaApiService api)
    {
        Url = api.GetMediaUrl(m.Url);
    }
}

/// <summary>
/// Display model for one source-game row.
/// Maps from a <see cref="SourceGameSummary"/> API record on construction.
/// Extends <see cref="ObservableObject"/> so <see cref="IsDeletePending"/> can
/// drive the inline confirmation UI without round-trips through the parent VM.
/// </summary>
public sealed partial class SourceGameRowViewModel : ObservableObject
{
    /// <summary>Server-assigned source-game ID used for all mutation APIs.</summary>
    public string SourceGameId     { get; }
    public string IntegrationLabel { get; }
    public string Platform         { get; }
    public string Kind             { get; }
    public string RawTitle         { get; }
    public string Status           { get; }
    public int    FileCount        { get; }

    /// <summary>Human-readable file summary, e.g. "3 file(s) in /roms/snes".</summary>
    public string FileSummary { get; }

    /// <summary>Absolute file paths for ROM launch — first entry is the primary ROM.</summary>
    public List<string> FilePaths { get; }

    // ---------------------------------------------------------------------------
    // Delete confirmation state
    // ---------------------------------------------------------------------------

    /// <summary>
    /// True when the user has clicked Delete once and the inline confirmation is visible.
    /// Drives the conditional button strip in the AXAML.
    /// </summary>
    [ObservableProperty]
    private bool _isDeletePending;

    // ---------------------------------------------------------------------------
    // Merge search state — drives the inline merge panel
    // ---------------------------------------------------------------------------

    /// <summary>True when the user has opened the merge-target search panel for this row.</summary>
    [ObservableProperty]
    private bool _isMergePending;

    /// <summary>The current text typed into the merge search box.</summary>
    [ObservableProperty]
    private string _mergeSearchQuery = string.Empty;

    /// <summary>True while the merge-target search API call is in-flight.</summary>
    [ObservableProperty]
    private bool _isMergeSearching;

    /// <summary>Results returned by the last merge-target search.</summary>
    [ObservableProperty]
    private ObservableCollection<CanonicalSearchResultViewModel> _mergeSearchResults = [];

    /// <summary>True when the search has returned at least one result.</summary>
    [ObservableProperty]
    private bool _hasMergeSearchResults;

    // ---------------------------------------------------------------------------
    // Resolver matches — read-only display, loaded from server
    // ---------------------------------------------------------------------------

    /// <summary>Metadata resolver matches attached to this source game.</summary>
    public ObservableCollection<SourceResolverMatchViewModel> ResolverMatches { get; }

    /// <summary>True when at least one resolver match is present.</summary>
    public bool HasResolverMatches => ResolverMatches.Count > 0;

    /// <summary>Human-readable header for the resolver-matches section.</summary>
    public string ResolverMatchCountText => $"{ResolverMatches.Count} resolver match(es)";

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public SourceGameRowViewModel(SourceGameSummary sg)
    {
        SourceGameId     = sg.Id;
        IntegrationLabel = sg.IntegrationLabel ?? sg.IntegrationId;
        Platform         = sg.Platform;
        Kind             = sg.Kind;
        RawTitle         = sg.RawTitle;
        Status           = sg.Status;
        FileCount        = sg.Files.Count;
        FileSummary      = sg.RootPath is not null
            ? $"{sg.Files.Count} file(s) in {sg.RootPath}"
            : $"{sg.Files.Count} file(s)";
        FilePaths = sg.Files.Select(f => f.Path).ToList();

        // Map resolver matches into display models.
        ResolverMatches = new ObservableCollection<SourceResolverMatchViewModel>(
            sg.ResolverMatches.Select(m => new SourceResolverMatchViewModel(m)));
    }
}

/// <summary>
/// Display model for one metadata resolver match attached to a source game.
/// Derives a human-readable <see cref="StatusLabel"/> from the match flags on construction.
/// </summary>
public sealed class SourceResolverMatchViewModel
{
    public string  PluginId          { get; }
    public string  Title             { get; }
    public string  Platform          { get; }
    public string  ExternalId        { get; }
    public string? Url               { get; }
    public bool    IsOutvoted        { get; }
    public bool    IsManualSelection { get; }

    /// <summary>"Manual" when manually pinned; "Outvoted" when overridden; "Active" otherwise.</summary>
    public string StatusLabel =>
        IsManualSelection ? "Manual"   :
        IsOutvoted        ? "Outvoted" :
                            "Active";

    public SourceResolverMatchViewModel(SourceResolverMatch m)
    {
        PluginId          = m.PluginId;
        Title             = m.Title ?? "(no title)";
        Platform          = m.Platform ?? string.Empty;
        ExternalId        = m.ExternalId;
        Url               = m.Url;
        IsOutvoted        = m.Outvoted;
        IsManualSelection = m.ManualSelection;
    }
}

/// <summary>
/// Display model for one canonical-game search result (merge-target candidate).
/// Builds a subtitle line from platform and source count on construction.
/// </summary>
public sealed class CanonicalSearchResultViewModel
{
    public string Id          { get; }
    public string Title       { get; }
    public string Platform    { get; }
    public int    SourceCount { get; }

    /// <summary>Platform + source count shown below the title in the merge search list.</summary>
    public string Subtitle => SourceCount > 0
        ? $"{Platform} · {SourceCount} source(s)"
        : Platform;

    public CanonicalSearchResultViewModel(CanonicalGameSearchResult r)
    {
        Id          = r.Id;
        Title       = r.Title;
        Platform    = r.Platform;
        SourceCount = r.SourceCount;
    }
}

/// <summary>
/// Display model for one external ID link (IGDB, Steam, etc.).
/// Maps from an <see cref="ExternalIdDto"/> API record on construction.
/// </summary>
public sealed class ExternalLinkViewModel
{
    public string Source     { get; }
    public string ExternalId { get; }
    public string? Url       { get; }

    public bool HasUrl => !string.IsNullOrEmpty(Url);

    public ExternalLinkViewModel(ExternalIdDto e)
    {
        Source     = e.Source;
        ExternalId = e.ExternalId;
        Url        = e.Url;
    }
}

// ---------------------------------------------------------------------------
// GameDetailViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Game detail page — full-bleed hero banner, metadata panel, and action bar.
///
/// Loaded on construction via _ = LoadAsync().
/// All navigation and toast calls happen on the UI thread (ConfigureAwait(true)).
/// </summary>
public sealed partial class GameDetailViewModel : ViewModelBase
{
    private readonly ServerConnectionService    _server;
    private readonly NavigationService          _nav;
    private readonly ToastService               _toast;
    private readonly AppConfigService           _config;
    private readonly InstallDetectionService?   _installDetector;
    private readonly RecentPlayedService?       _recentPlayed;

    // Cached for re-detection after manual binding changes.
    private IReadOnlyList<SourceGameInfo> _sourcesForDetection = [];

    // ---------------------------------------------------------------------------
    // Identity
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _gameId = string.Empty;

    // ---------------------------------------------------------------------------
    // Loading state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    // ---------------------------------------------------------------------------
    // Metadata
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _title = string.Empty;

    [ObservableProperty]
    private string _platform = string.Empty;

    [ObservableProperty]
    private string? _description;

    [ObservableProperty]
    private string? _releaseDate;

    [ObservableProperty]
    private string? _developer;

    [ObservableProperty]
    private string? _publisher;

    [ObservableProperty]
    private double _rating;

    /// <summary>Comma-separated genre list, e.g. "Action, RPG".</summary>
    [ObservableProperty]
    private string _genresText = string.Empty;

    // ---------------------------------------------------------------------------
    // Media
    // ---------------------------------------------------------------------------

    /// <summary>URL for the hero background image (background media, or cover as fallback).</summary>
    [ObservableProperty]
    private string? _heroImageUrl;

    [ObservableProperty]
    private string? _coverUrl;

    /// <summary>Screenshot and header images — shown in the media carousel strip.</summary>
    [ObservableProperty]
    private ObservableCollection<MediaItemModel> _screenshots = [];

    [ObservableProperty]
    private bool _hasScreenshots;

    /// <summary>True when this game has at least one media asset (enables the Manage Media button).</summary>
    [ObservableProperty]
    private bool _hasMedia;

    // ---------------------------------------------------------------------------
    // Favorite / Achievements
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _favorite;

    [ObservableProperty]
    private int _achievementUnlocked;

    [ObservableProperty]
    private int _achievementTotal;

    [ObservableProperty]
    private bool _hasAchievements;

    // ---------------------------------------------------------------------------
    // Source games + external IDs
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<SourceGameRowViewModel> _sourceGames = [];

    [ObservableProperty]
    private bool _hasSourceGames;

    [ObservableProperty]
    private ObservableCollection<ExternalLinkViewModel> _externalLinks = [];

    [ObservableProperty]
    private bool _hasExternalLinks;

    [ObservableProperty]
    private bool _hasRating;

    // ---------------------------------------------------------------------------
    // Metadata refresh
    // ---------------------------------------------------------------------------

    /// <summary>True while a metadata refresh call is in-flight — disables the button.</summary>
    [ObservableProperty]
    private bool _isRefreshingMetadata;

    // ---------------------------------------------------------------------------
    // Emulator launch
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _canLaunchWithEmulator;

    [ObservableProperty]
    private string _emulatorName = string.Empty;

    // ---------------------------------------------------------------------------
    // Install detection
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Latest install detection result for this game on this machine.
    /// Null until detection has completed (shows "Detecting…" spinner in UI).
    /// Updated on the UI thread by the background detection task.
    /// </summary>
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsInstalled))]
    [NotifyPropertyChangedFor(nameof(CanInstallGame))]
    [NotifyPropertyChangedFor(nameof(NeedsClientInstall))]
    [NotifyPropertyChangedFor(nameof(NeedsManualBind))]
    [NotifyPropertyChangedFor(nameof(IsDetectingInstall))]
    [NotifyPropertyChangedFor(nameof(InstallStateText))]
    [NotifyPropertyChangedFor(nameof(ClientMissingMessage))]
    [NotifyPropertyChangedFor(nameof(ClientDownloadUrl))]
    private InstallStatus? _installStatus;

    // Derived convenience booleans — computed fresh whenever InstallStatus changes.
    // Note: use the generated property `InstallStatus`, not the backing field `_installStatus`.
    public bool IsInstalled         => InstallStatus?.IsInstalled ?? false;
    public bool CanInstallGame      => InstallStatus?.CanInstall ?? false;
    public bool NeedsClientInstall  => InstallStatus?.NeedsClientInstall ?? false;
    public bool NeedsManualBind     => InstallStatus?.NeedsManualBind ?? false;
    public bool IsDetectingInstall  => InstallStatus is null;

    public string? ClientMissingMessage => InstallStatus?.ClientMissingMessage;
    public string? ClientDownloadUrl    => InstallStatus?.ClientDownloadUrl;

    /// <summary>Short human-readable install state label shown in the action bar.</summary>
    public string InstallStateText => InstallStatus?.State switch
    {
        InstallState.Installed             => "✓ Installed",
        InstallState.NotInstalled          => "Not installed",
        InstallState.ClientMissing         => "Client missing",
        InstallState.ManualBindNeeded      => "Setup required",
        InstallState.RomRemote             => "Remote ROM",
        InstallState.EmulatorMissing       => "Emulator missing",
        InstallState.EmulatorNotConfigured => "Emulator not set up",
        InstallState.Unknown               => "Detecting…",
        _                                  => string.Empty,
    };

    // ---------------------------------------------------------------------------
    // Merge tracking — only one row can be in merge mode at a time
    // ---------------------------------------------------------------------------

    /// <summary>The source row whose merge panel is currently open; null when none.</summary>
    private SourceGameRowViewModel? _pendingMergeRow;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public GameDetailViewModel(
        string                   gameId,
        ServerConnectionService  server,
        NavigationService        nav,
        ToastService             toast,
        AppConfigService         config,
        InstallDetectionService? installDetector = null,
        RecentPlayedService?     recentPlayed    = null)
    {
        GameId           = gameId;
        _server          = server;
        _nav             = nav;
        _toast           = toast;
        _config          = config;
        _installDetector = installDetector;
        _recentPlayed    = recentPlayed;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Opens the game page in the system browser.</summary>
    [RelayCommand]
    private void PlayInBrowser()
    {
        var url = $"{_server.ActiveUrl}/game/{Uri.EscapeDataString(GameId)}/play";
        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(url) { UseShellExecute = true });
    }

    /// <summary>
    /// Launches the game using the resolved exe path or a storefront URI
    /// stored in <see cref="InstallStatus.LaunchUri"/>.
    /// </summary>
    [RelayCommand]
    private void LaunchGame()
    {
        // Check if an emulator is needed but not configured before attempting launch.
        if (InstallStatus?.NeedsEmulator == true)
        {
            _toast.Warning(
                "Emulator not configured",
                $"This game requires an emulator for {Platform}. " +
                "Go to Settings → Emulators to add one.");
            return;
        }

        var launchUri = InstallStatus?.LaunchUri;
        if (string.IsNullOrEmpty(launchUri))
        {
            _toast.Error("Cannot launch", "No launch path is available for this game.");
            return;
        }

        try
        {
            System.Diagnostics.Process.Start(
                new System.Diagnostics.ProcessStartInfo(launchUri) { UseShellExecute = true });

            // Record in recent-played history after a successful launch.
            _recentPlayed?.RecordPlay(GameId, Title, CoverUrl);
        }
        catch (Exception ex)
        {
            _toast.Error("Launch failed", ex.Message);
        }
    }

    /// <summary>
    /// Opens the storefront install flow (e.g. <c>steam://install/730</c>)
    /// or navigates to a download page when the game is not installed.
    /// </summary>
    [RelayCommand]
    private void InstallGame()
    {
        var installUri = InstallStatus?.LaunchUri;
        if (string.IsNullOrEmpty(installUri))
        {
            _toast.Error("Cannot install", "No install URI available for this game.");
            return;
        }

        try
        {
            System.Diagnostics.Process.Start(
                new System.Diagnostics.ProcessStartInfo(installUri) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Install failed", ex.Message);
        }
    }

    /// <summary>Opens the client download URL when the storefront client is missing.</summary>
    [RelayCommand]
    private void OpenClientDownload()
    {
        var url = InstallStatus?.ClientDownloadUrl;
        if (string.IsNullOrEmpty(url)) return;

        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(url) { UseShellExecute = true });
    }

    /// <summary>Opens the game's install folder in Explorer.</summary>
    [RelayCommand]
    private void OpenInstallFolder()
    {
        var path = InstallStatus?.InstallPath;
        if (string.IsNullOrEmpty(path)) return;

        try
        {
            System.Diagnostics.Process.Start(
                new System.Diagnostics.ProcessStartInfo("explorer.exe", $"\"{path}\"")
                { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Cannot open folder", ex.Message);
        }
    }

    /// <summary>
    /// Re-runs install detection for this game, clearing cached state first.
    /// Useful after the user installs/uninstalls the game.
    /// </summary>
    [RelayCommand]
    private async Task RefreshInstallStatusAsync()
    {
        if (_installDetector is null) return;

        InstallStatus = null; // shows "Detecting…" spinner

        var status = await _installDetector
            .DetectGameAsync(GameId, Title, _sourcesForDetection)
            .ConfigureAwait(true);

        InstallStatus = status;
    }

    /// <summary>
    /// Opens a file picker for the user to manually select the game executable.
    /// Stores the selected path as a permanent override via <see cref="InstallDetectionService"/>.
    /// </summary>
    [RelayCommand]
    private async Task PickExePathAsync()
    {
        // Obtain the main window from the Avalonia application lifetime.
        var lifetime = Avalonia.Application.Current?.ApplicationLifetime
            as Avalonia.Controls.ApplicationLifetimes.IClassicDesktopStyleApplicationLifetime;

        var mainWindow = lifetime?.MainWindow;
        if (mainWindow is null) return;

        var options = new Avalonia.Platform.Storage.FilePickerOpenOptions
        {
            Title          = $"Select executable for \"{Title}\"",
            AllowMultiple  = false,
            FileTypeFilter =
            [
                new Avalonia.Platform.Storage.FilePickerFileType("Executable (*.exe)")
                {
                    Patterns = ["*.exe"],
                },
                new Avalonia.Platform.Storage.FilePickerFileType("All files")
                {
                    Patterns = ["*"],
                },
            ],
        };

        var result   = await mainWindow.StorageProvider.OpenFilePickerAsync(options)
            .ConfigureAwait(true);
        var selected = result.FirstOrDefault();
        if (selected is null) return;

        var exePath = selected.Path.IsAbsoluteUri && selected.Path.IsFile
            ? selected.Path.LocalPath
            : selected.Path.AbsolutePath;
        SetManualBinding(exePath);
    }

    /// <summary>
    /// Stores a manually chosen exe path as the permanent override for this game.
    /// </summary>
    public void SetManualBinding(string exePath)
    {
        if (_installDetector is null) return;

        _installDetector.SetManualBinding(GameId, exePath);
        InstallStatus = new InstallStatus
        {
            State      = InstallState.Installed,
            ExePath    = exePath,
            LaunchUri  = exePath,
            Confidence = 1.0,
        };
        _toast.Success("Executable bound", $"Game will now launch from:\n{exePath}");
    }

    /// <summary>Toggles the favorite flag via PUT/DELETE /api/games/{id}/favorite.</summary>
    [RelayCommand]
    private async Task ToggleFavoriteAsync()
    {
        if (_server.Api is null)
            return;

        bool newValue = !Favorite;
        try
        {
            await _server.Api.SetFavoriteAsync(GameId, newValue).ConfigureAwait(true);
            Favorite = newValue;
            _toast.Success(
                newValue ? "Added to favorites" : "Removed from favorites",
                Title);
        }
        catch (Exception ex)
        {
            _toast.Error("Could not update favorite", ex.Message);
        }
    }

    /// <summary>Navigates back to the library.</summary>
    [RelayCommand]
    private void GoBack()
    {
        _nav.NavigateTo(new LibraryViewModel(
            _server, _nav, _toast, _config, installDetector: _installDetector, recentPlayed: _recentPlayed));
    }

    /// <summary>Opens the Media Manager page for this game.</summary>
    [RelayCommand]
    private void OpenMediaManager()
    {
        _nav.NavigateTo(new MediaManagerViewModel(
            GameId, _server, _nav, _toast, _config, _installDetector));
    }

    /// <summary>Launches the game using the configured emulator for its platform.</summary>
    [RelayCommand]
    private void LaunchWithEmulator()
    {
        var emulators = _config.GetEmulators();
        var emulator  = FindEmulatorForPlatform(Platform, emulators);

        if (emulator is null)
        {
            _toast.Error("No emulator",
                $"No emulator configured for platform \"{Platform}\". Add one in Settings → Emulators.");
            return;
        }

        // Find the primary ROM file path.
        var romPath = SourceGames
            .SelectMany(sg => sg.FilePaths)
            .FirstOrDefault();

        if (string.IsNullOrEmpty(romPath))
        {
            _toast.Error("No files", "This game has no local files available for launch.");
            return;
        }

        try
        {
            var args = emulator.ArgsTemplate.Replace("{rom}", $"\"{romPath}\"");
            System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
            {
                FileName        = emulator.ExecutablePath,
                Arguments       = args,
                UseShellExecute = true,
            });
            _toast.Success("Launched", $"Started \"{Title}\" with {emulator.Name}.");

            // Record in recent-played history after a successful emulator launch.
            _recentPlayed?.RecordPlay(GameId, Title, CoverUrl);
        }
        catch (Exception ex)
        {
            _toast.Error("Launch failed", ex.Message);
        }
    }

    /// <summary>Opens an external link in the system browser.</summary>
    [RelayCommand]
    private void OpenExternalLink(ExternalLinkViewModel link)
    {
        if (string.IsNullOrEmpty(link.Url)) return;
        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(link.Url) { UseShellExecute = true });
    }

    /// <summary>
    /// Triggers an online metadata refresh for this game.
    /// Returns the updated detail synchronously — no SSE needed.
    /// </summary>
    [RelayCommand]
    private async Task RefreshMetadataAsync()
    {
        if (_server.Api is null || IsRefreshingMetadata)
            return;

        IsRefreshingMetadata = true;
        try
        {
            await _server.Api.RefreshGameMetadataAsync(GameId).ConfigureAwait(true);
            _toast.Success("Metadata refreshed", Title);

            // Reload the full game detail to reflect the new metadata.
            await LoadAsync().ConfigureAwait(true);
        }
        catch (MgaApiException ex) when (ex.StatusCode == 409)
        {
            _toast.Info("No change", "No eligible metadata provider found for this game.");
        }
        catch (MgaApiException ex) when (ex.StatusCode == 422)
        {
            _toast.Error("Providers unavailable", ex.Message);
        }
        catch (Exception ex)
        {
            _toast.Error("Metadata refresh failed", ex.Message);
        }
        finally
        {
            IsRefreshingMetadata = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Commands — source actions
    // ---------------------------------------------------------------------------

    /// <summary>Arms the inline delete confirmation for the given source row.</summary>
    [RelayCommand]
    private static void RequestDeleteSource(SourceGameRowViewModel row)
        => row.IsDeletePending = true;

    /// <summary>Dismisses the inline delete confirmation without deleting.</summary>
    [RelayCommand]
    private static void CancelDeleteSource(SourceGameRowViewModel row)
        => row.IsDeletePending = false;

    /// <summary>
    /// Executes the hard delete after the user has confirmed.
    /// If the canonical game was also deleted, navigates back to the Library.
    /// </summary>
    [RelayCommand]
    private async Task ConfirmDeleteSourceAsync(SourceGameRowViewModel row)
    {
        if (_server.Api is null)
            return;

        row.IsDeletePending = false;

        try
        {
            var result = await _server.Api.DeleteSourceGameAsync(GameId, row.SourceGameId)
                .ConfigureAwait(true);

            var warnSuffix = result.Warnings.Count > 0
                ? $" ({result.Warnings.Count} warning(s))"
                : string.Empty;
            _toast.Success("Source deleted", $"\"{row.RawTitle}\" removed{warnSuffix}.");

            if (!result.CanonicalExists)
            {
                // The canonical game itself was deleted — navigate back.
                _nav.NavigateTo(new LibraryViewModel(
            _server, _nav, _toast, _config, installDetector: _installDetector, recentPlayed: _recentPlayed));
            }
            else
            {
                await LoadAsync().ConfigureAwait(true);
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Delete failed", ex.Message);
        }
    }

    /// <summary>
    /// Removes the canonical pin that forced this source to this game,
    /// then reloads the page.
    /// </summary>
    [RelayCommand]
    private async Task ClearPinAsync(SourceGameRowViewModel row)
    {
        if (_server.Api is null)
            return;

        try
        {
            await _server.Api.ClearCanonicalPinAsync(GameId, row.SourceGameId)
                .ConfigureAwait(true);
            _toast.Success("Pin cleared", $"Canonical pin removed for \"{row.RawTitle}\".");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Clear pin failed", ex.Message);
        }
    }

    /// <summary>
    /// Splits the source-game into its own canonical entry,
    /// then reloads the page (which will now have one fewer source).
    /// </summary>
    [RelayCommand]
    private async Task SplitSourceAsync(SourceGameRowViewModel row)
    {
        if (_server.Api is null)
            return;

        try
        {
            await _server.Api.SplitSourceGameAsync(GameId, row.SourceGameId)
                .ConfigureAwait(true);
            _toast.Success("Source split", $"\"{row.RawTitle}\" is now its own game entry.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Split failed", ex.Message);
        }
    }

    // ---------------------------------------------------------------------------
    // Commands — merge source
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Opens the inline merge-search panel for the given source row.
    /// Closes any other row's merge panel first (only one open at a time).
    /// </summary>
    [RelayCommand]
    private void RequestMergeSource(SourceGameRowViewModel row)
    {
        // Close the previously open merge panel, if any.
        if (_pendingMergeRow is not null && _pendingMergeRow != row)
            CloseMergePanelFor(_pendingMergeRow);

        _pendingMergeRow   = row;
        row.IsMergePending = true;
    }

    /// <summary>Cancels the open merge panel and resets all its state.</summary>
    [RelayCommand]
    private void CancelMergeSource(SourceGameRowViewModel row)
    {
        CloseMergePanelFor(row);
        if (_pendingMergeRow == row)
            _pendingMergeRow = null;
    }

    /// <summary>
    /// Calls GET /api/canonical-games/search using the query typed in the merge panel
    /// and populates <see cref="SourceGameRowViewModel.MergeSearchResults"/>.
    /// </summary>
    [RelayCommand]
    private async Task SearchMergeTargetsAsync(SourceGameRowViewModel row)
    {
        if (_server.Api is null || string.IsNullOrWhiteSpace(row.MergeSearchQuery))
            return;

        row.IsMergeSearching       = true;
        row.MergeSearchResults     = [];
        row.HasMergeSearchResults  = false;

        try
        {
            var response = await _server.Api
                .SearchCanonicalGamesAsync(row.MergeSearchQuery.Trim())
                .ConfigureAwait(true);

            row.MergeSearchResults    = new ObservableCollection<CanonicalSearchResultViewModel>(
                response.Games.Select(r => new CanonicalSearchResultViewModel(r)));
            row.HasMergeSearchResults = row.MergeSearchResults.Count > 0;
        }
        catch (Exception ex)
        {
            _toast.Error("Search failed", ex.Message);
        }
        finally
        {
            row.IsMergeSearching = false;
        }
    }

    /// <summary>
    /// Merges the currently pending source row into the chosen canonical game.
    /// Reloads the page on success; navigates back if the current canonical was dissolved.
    /// </summary>
    [RelayCommand]
    private async Task ConfirmMergeSourceAsync(CanonicalSearchResultViewModel target)
    {
        if (_server.Api is null || _pendingMergeRow is null)
            return;

        var row = _pendingMergeRow;

        try
        {
            var result = await _server.Api
                .MergeSourceGameAsync(GameId, row.SourceGameId, target.Id)
                .ConfigureAwait(true);

            _toast.Success("Source merged",
                $"\"{row.RawTitle}\" merged into \"{target.Title}\".");

            _pendingMergeRow = null;

            // Navigate to the target canonical if the current game changed identity.
            if (result.CanonicalGameId != GameId)
            {
                _nav.NavigateTo(new GameDetailViewModel(
                    result.CanonicalGameId, _server, _nav, _toast, _config, _installDetector, _recentPlayed));
            }
            else
            {
                await LoadAsync().ConfigureAwait(true);
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Merge failed", ex.Message);
        }
    }

    // ---------------------------------------------------------------------------
    // Private — merge helpers
    // ---------------------------------------------------------------------------

    private static void CloseMergePanelFor(SourceGameRowViewModel row)
    {
        row.IsMergePending        = false;
        row.IsMergeSearching      = false;
        row.MergeSearchQuery      = string.Empty;
        row.MergeSearchResults    = [];
        row.HasMergeSearchResults = false;
    }

    // ---------------------------------------------------------------------------
    // Private — emulator helpers
    // ---------------------------------------------------------------------------

    private void CheckEmulatorAvailability()
    {
        var emulators = _config.GetEmulators();
        var matched   = FindEmulatorForPlatform(Platform, emulators);

        CanLaunchWithEmulator = matched is not null;
        EmulatorName          = matched?.Name ?? string.Empty;
    }

    private static EmulatorEntry? FindEmulatorForPlatform(string platform, List<EmulatorEntry> emulators)
    {
        if (string.IsNullOrEmpty(platform))
            return null;

        return emulators.FirstOrDefault(e =>
            e.Platforms
             .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)
             .Any(p => p.Equals(platform, StringComparison.OrdinalIgnoreCase)));
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
            var game = await _server.Api.GetGameAsync(GameId).ConfigureAwait(true);
            var api  = _server.Api;

            // Populate scalar metadata.
            Title       = game.Title;
            Platform    = game.Platform;
            Description = game.Description;
            ReleaseDate = game.ReleaseDate;
            Developer   = game.Developer;
            Publisher   = game.Publisher;
            Rating      = game.Rating;
            Favorite    = game.Favorite;
            GenresText  = string.Join(", ", game.Genres);
            HasRating   = Rating > 0;

            // Resolve media URLs.
            var coverMedia = game.CoverOverride ?? game.Media.FirstOrDefault(m => m.Type == "cover");
            CoverUrl = coverMedia is not null ? api.GetMediaUrl(coverMedia.Url) : null;

            var heroMedia = game.Media.FirstOrDefault(m => m.Type == "background");
            HeroImageUrl  = heroMedia is not null ? api.GetMediaUrl(heroMedia.Url) : CoverUrl;

            // Screenshot carousel — each MediaItemModel resolves its own URL.
            Screenshots = new ObservableCollection<MediaItemModel>(
                game.Media
                    .Where(m => m.Type == "screenshot" || m.Type == "header")
                    .Select(m => new MediaItemModel(m, api)));
            HasScreenshots = Screenshots.Count > 0;
            HasMedia       = game.Media.Count > 0;

            // Check for configured emulator.
            CheckEmulatorAvailability();

            // Achievement summary.
            if (game.AchievementSummary is not null)
            {
                HasAchievements     = true;
                AchievementTotal    = game.AchievementSummary.TotalCount;
                AchievementUnlocked = game.AchievementSummary.UnlockedCount;
            }

            // Source games — each SourceGameRowViewModel maps its own SourceGameSummary.
            SourceGames    = new ObservableCollection<SourceGameRowViewModel>(
                game.SourceGames.Select(sg => new SourceGameRowViewModel(sg)));
            HasSourceGames = SourceGames.Count > 0;

            // External links — filtered to entries that have a resolvable URL.
            ExternalLinks    = new ObservableCollection<ExternalLinkViewModel>(
                game.ExternalIds
                    .Where(e => !string.IsNullOrEmpty(e.Url))
                    .Select(e => new ExternalLinkViewModel(e)));
            HasExternalLinks = ExternalLinks.Count > 0;

            // Cache SourceGameInfo list for install detection + re-detection.
            _sourcesForDetection = game.SourceGames.Select(sg => new SourceGameInfo
            {
                SourceGameId = sg.Id,
                PluginId     = sg.PluginId,
                ExternalId   = sg.ExternalId,
                RootPath     = sg.RootPath,
                Label        = sg.IntegrationLabel,
            }).ToList();

            // Kick off install detection in the background (non-blocking).
            // Result arrives via InstallStatus property update on the UI thread.
            if (_installDetector is not null)
                _ = RunInstallDetectionAsync();
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load game", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    /// <summary>
    /// Background helper: runs detection and marshals the result to the UI thread.
    /// Fire-and-forget from <see cref="LoadAsync"/> — exceptions are caught internally.
    /// </summary>
    private async Task RunInstallDetectionAsync()
    {
        if (_installDetector is null) return;

        try
        {
            // Run detection off the UI thread (registry + file I/O).
            var status = await Task.Run(async () =>
                await _installDetector.DetectGameAsync(
                    GameId, Title, _sourcesForDetection).ConfigureAwait(false))
                .ConfigureAwait(false);

            // Apply result on the UI thread.
            await Avalonia.Threading.Dispatcher.UIThread.InvokeAsync(() =>
            {
                InstallStatus = status;
            });
        }
        catch
        {
            // Non-fatal — install state remains null (shows "Detecting…").
        }
    }
}
