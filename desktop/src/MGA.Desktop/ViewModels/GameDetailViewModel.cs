using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// Shared slug formatting utility
// ---------------------------------------------------------------------------

/// <summary>
/// Converts raw API plugin-ID slugs to human-readable brand names.
/// Shared by <see cref="ExternalLinkViewModel"/> and <see cref="SourceResolverMatchViewModel"/>.
/// </summary>
internal static class PluginSlugFormatter
{
    /// <summary>
    /// Strips known prefixes ("metadata-", "game-source-") and applies
    /// well-known acronym/brand casing so raw API slugs display nicely.
    /// e.g. "metadata-launchbox" → "LaunchBox", "metadata-igdb" → "IGDB".
    /// </summary>
    public static string Prettify(string slug)
    {
        if (string.IsNullOrEmpty(slug)) return slug;

        // Strip known plugin-category prefixes.
        string name = slug;
        if (name.StartsWith("metadata-",    StringComparison.OrdinalIgnoreCase))
            name = name["metadata-".Length..];
        else if (name.StartsWith("game-source-", StringComparison.OrdinalIgnoreCase))
            name = name["game-source-".Length..];

        // Well-known brand/acronym overrides (lowercased slug → display name).
        return name.ToLowerInvariant() switch
        {
            "igdb"               => "IGDB",
            "rawg"               => "RAWG",
            "gog"                => "GOG",
            "steam"              => "Steam",
            "launchbox"          => "LaunchBox",
            "retroachievements"  => "RetroAchievements",
            "xbox"               => "Xbox",
            "psn"                => "PSN",
            "epic"               => "Epic Games",
            "itchio"             => "itch.io",
            _ => System.Globalization.CultureInfo.CurrentCulture.TextInfo
                     .ToTitleCase(name.Replace('-', ' ').Replace('_', ' '))
        };
    }
}

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

    /// <summary>True when this source has at least one local file; drives the file-path panel.</summary>
    public bool HasFiles => FileCount > 0;

    /// <summary>All file paths joined by newlines; used for the read-only file-path TextBox.</summary>
    public string FormattedFilePaths => string.Join(Environment.NewLine, FilePaths);

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

    /// <summary>Human-readable plugin name — strips prefixes and applies brand casing.</summary>
    public string DisplayPluginId => PluginSlugFormatter.Prettify(PluginId);

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

    /// <summary>
    /// Human-readable source name for display.
    /// Strips common plugin-ID prefixes and applies known acronym mappings.
    /// e.g. "metadata-launchbox" → "LaunchBox", "metadata-igdb" → "IGDB".
    /// </summary>
    public string DisplaySource => PluginSlugFormatter.Prettify(Source);

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
    private readonly EmulatorService?           _emulatorService;
    private readonly GameStateService?          _gameStateService;

    // Cached for re-detection after manual binding changes.
    private IReadOnlyList<SourceGameInfo>            _sourcesForDetection = [];

    // Raw source-game list stored at load time for GameStateService computation.
    private IReadOnlyList<MGA.Api.SourceGameSummary> _rawSourceGames      = [];

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

    /// <summary>Human-readable platform label, e.g. "DOS" instead of "ms_dos".</summary>
    public string FormattedPlatform => PlatformHelper.FormatPlatform(Platform);

    /// <summary>Platform badge accent color hex, e.g. "#334155" for PC.</summary>
    public string PlatformBadgeColor => PlatformHelper.GetBadgeColor(Platform);

    partial void OnPlatformChanged(string value)
    {
        OnPropertyChanged(nameof(FormattedPlatform));
        OnPropertyChanged(nameof(PlatformBadgeColor));
        OnPropertyChanged(nameof(IsEmulatedGame));
    }

    partial void OnPrimaryPlayStateChanged(SourceGamePlayState? value)
    {
        // Auto-select the highest-priority config when state changes.
        OnPropertyChanged(nameof(EmulatorConfigs));
        SelectedEmulatorConfig = value?.AvailableConfigs?.Count > 0
            ? value.AvailableConfigs[0]
            : null;
    }

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
    // Emulator / emulation-state (Phase 2)
    // ---------------------------------------------------------------------------

    /// <summary>Legacy: true when an emulator config covers this platform (used when GameStateService absent).</summary>
    [ObservableProperty]
    private bool _canLaunchWithEmulator;

    /// <summary>Legacy: emulator display name (used when GameStateService absent).</summary>
    [ObservableProperty]
    private string _emulatorName = string.Empty;

    /// <summary>
    /// Full emulation play state for the best source game on this device.
    /// Null until <see cref="ComputeEmulationStateAsync"/> completes.
    /// </summary>
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CanPlayViaEmulator))]
    [NotifyPropertyChangedFor(nameof(NeedsEmulatedInstall))]
    [NotifyPropertyChangedFor(nameof(NeedsEmulatorSetup))]
    [NotifyPropertyChangedFor(nameof(NeedsBiosSetup))]
    [NotifyPropertyChangedFor(nameof(HasMultipleEmulatorConfigs))]
    [NotifyPropertyChangedFor(nameof(PrimaryEmulatorLabel))]
    [NotifyPropertyChangedFor(nameof(MissingBiosMessage))]
    [NotifyPropertyChangedFor(nameof(CanUninstallEmulatedGame))]
    [NotifyPropertyChangedFor(nameof(CanHardDeleteEmulated))]
    private SourceGamePlayState? _primaryPlayState;

    /// <summary>True while emulation state is being computed (BIOS I/O in background).</summary>
    [ObservableProperty]
    private bool _isComputingEmulationState;

    // ── Derived from PrimaryPlayState ────────────────────────────────────────

    /// <summary>True when this game's canonical platform maps to the emulation path.</summary>
    public bool IsEmulatedGame => GameStateService.IsEmulatedPlatform(Platform ?? string.Empty);

    /// <summary>Emulator is configured and ready — show the primary Play button.</summary>
    public bool CanPlayViaEmulator =>
        PrimaryPlayState?.EmulatorState == EmulatorAvailability.Ready
        && PrimaryPlayState.Kind is GamePlayStateKind.PlainEmulated
                                 or GamePlayStateKind.InstalledEmulated;

    /// <summary>Packed game with no install record on this device — show Install/Locate button.</summary>
    public bool NeedsEmulatedInstall => PrimaryPlayState?.Kind == GamePlayStateKind.NotInstalled;

    /// <summary>No emulator is configured for this platform — show Setup Emulator button.</summary>
    public bool NeedsEmulatorSetup =>
        PrimaryPlayState?.EmulatorState == EmulatorAvailability.NotConfigured;

    /// <summary>Emulator configured but required BIOS files missing — show BIOS warning button.</summary>
    public bool NeedsBiosSetup =>
        PrimaryPlayState?.EmulatorState == EmulatorAvailability.BiosMissing;

    /// <summary>True when the primary config is an MGA-managed install that can be uninstalled.</summary>
    public bool CanUninstallEmulatedGame => PrimaryPlayState?.CanUninstall ?? false;

    /// <summary>True when a hard-delete of source files is eligible.</summary>
    public bool CanHardDeleteEmulated => PrimaryPlayState?.CanHardDelete ?? false;

    /// <summary>
    /// More than one emulator config covers this platform — show chevron for config picker.
    /// </summary>
    public bool HasMultipleEmulatorConfigs =>
        (PrimaryPlayState?.AvailableConfigs?.Count ?? 0) > 1;

    /// <summary>
    /// Label for the primary Play button, e.g. "RetroArch" or "PCSX2".
    /// Empty string when no emulator is configured.
    /// </summary>
    public string PrimaryEmulatorLabel
    {
        get
        {
            var configs = PrimaryPlayState?.AvailableConfigs;
            if (configs is null || configs.Count == 0)
                return string.Empty;

            var install = _emulatorService?.GetInstall(configs[0].InstallId);
            return install?.Name ?? configs[0].DisplayName;
        }
    }

    /// <summary>
    /// Comma-separated list of missing required BIOS file names.
    /// Shown as a tooltip on the "BIOS Missing" button.
    /// </summary>
    public string MissingBiosMessage
    {
        get
        {
            var missing = PrimaryPlayState?.BiosCheck?.Missing;
            if (missing is null || missing.Count == 0)
                return "Required BIOS files are missing.";

            return "Missing BIOS: " + string.Join(", ", missing.Select(b => b.Filename));
        }
    }

    /// <summary>
    /// All available emulator configs for the platform — bound to the config-picker ComboBox.
    /// </summary>
    public IReadOnlyList<EmulatorConfig> EmulatorConfigs =>
        PrimaryPlayState?.AvailableConfigs ?? [];

    /// <summary>
    /// The config chosen in the multi-config picker.
    /// Defaults to the first (highest-priority) config when PrimaryPlayState changes.
    /// </summary>
    [ObservableProperty]
    private EmulatorConfig? _selectedEmulatorConfig;

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
        InstallDetectionService? installDetector  = null,
        RecentPlayedService?     recentPlayed     = null,
        EmulatorService?         emulatorService  = null,
        GameStateService?        gameStateService = null)
    {
        GameId            = gameId;
        _server           = server;
        _nav              = nav;
        _toast            = toast;
        _config           = config;
        _installDetector  = installDetector;
        _recentPlayed     = recentPlayed;
        _emulatorService  = emulatorService;
        _gameStateService = gameStateService;

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

    /// <summary>Navigates back — restores the prior page from history when available.</summary>
    [RelayCommand]
    private void GoBack()
    {
        if (_nav.CanGoBack)
        {
            _nav.NavigateBack();
            return;
        }

        // Fallback: no history (e.g. deep-linked directly), create a fresh Library.
        _nav.NavigateTo(new LibraryViewModel(
            _server, _nav, _toast, _config,
            installDetector:  _installDetector,
            recentPlayed:     _recentPlayed,
            gameStateService: _gameStateService));
    }

    /// <summary>Opens the Media Manager page for this game.</summary>
    [RelayCommand]
    private void OpenMediaManager()
    {
        _nav.NavigateTo(new MediaManagerViewModel(
            GameId, _server, _nav, _toast, _config,
            _installDetector, _gameStateService));
    }

    /// <summary>
    /// Launches the game using the selected emulator config (or highest-priority if none selected).
    /// </summary>
    [RelayCommand]
    private void LaunchWithEmulator()
    {
        if (_emulatorService is null)
        {
            _toast.Error("No emulator",
                $"No emulator configured for platform \"{Platform}\". Add one in Settings → Emulators.");
            return;
        }

        // Use the user-selected config from the picker, or fall back to highest-priority.
        var configs = _emulatorService.GetConfigsForPlatform(Platform);
        if (configs.Count == 0)
        {
            _toast.Error("No emulator",
                $"No emulator configured for platform \"{Platform}\". Add one in Settings → Emulators.");
            return;
        }

        var config  = SelectedEmulatorConfig ?? configs[0];
        var install = _emulatorService.GetInstall(config.InstallId);
        if (install is null || string.IsNullOrEmpty(install.ExecutablePath))
        {
            _toast.Error("Emulator not found",
                $"The emulator \"{config.DisplayName}\" has no executable path. Edit it in Settings → Emulators.");
            return;
        }

        // Find the primary ROM file path from any source game with local files.
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
            // Build args: prefer the config's custom template, then the install's default template.
            var template = !string.IsNullOrWhiteSpace(config.ArgsTemplate)
                ? config.ArgsTemplate
                : "\"{rom}\"";

            var args = template.Replace("{rom}", $"\"{romPath}\"");

            System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
            {
                FileName        = install.ExecutablePath,
                Arguments       = args,
                UseShellExecute = true,
            });

            _toast.Success("Launched", $"Started \"{Title}\" with {install.Name}.");

            // Record in recent-played history after a successful emulator launch.
            _recentPlayed?.RecordPlay(GameId, Title, CoverUrl);
        }
        catch (Exception ex)
        {
            _toast.Error("Launch failed", ex.Message);
        }
    }

    /// <summary>
    /// Launches the game with a specific emulator config (from the multi-config picker).
    /// </summary>
    [RelayCommand]
    private void LaunchWithEmulatorConfig(EmulatorConfig config)
    {
        if (_emulatorService is null) return;

        var install = _emulatorService.GetInstall(config.InstallId);
        if (install is null || string.IsNullOrEmpty(install.ExecutablePath))
        {
            _toast.Error("Emulator not found",
                $"The emulator \"{config.DisplayName}\" has no executable path.");
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
            var template = !string.IsNullOrWhiteSpace(config.ArgsTemplate)
                ? config.ArgsTemplate
                : "\"{rom}\"";
            var args = template.Replace("{rom}", $"\"{romPath}\"");

            System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
            {
                FileName        = install.ExecutablePath,
                Arguments       = args,
                UseShellExecute = true,
            });

            _toast.Success("Launched", $"Started \"{Title}\" with {install.Name}.");
            _recentPlayed?.RecordPlay(GameId, Title, CoverUrl);
        }
        catch (Exception ex)
        {
            _toast.Error("Launch failed", ex.Message);
        }
    }

    /// <summary>
    /// Opens a folder picker so the user can locate an existing install
    /// of this packed/emulated game (e.g., a pre-extracted ROM folder).
    /// Saves the result as a <see cref="GameInstallRecord"/>.
    /// </summary>
    [RelayCommand]
    private async Task LocateEmulatedGameAsync()
    {
        if (_emulatorService is null) return;

        var lifetime = Avalonia.Application.Current?.ApplicationLifetime
            as Avalonia.Controls.ApplicationLifetimes.IClassicDesktopStyleApplicationLifetime;
        var mainWindow = lifetime?.MainWindow;
        if (mainWindow is null) return;

        // Open a file picker — for emulated games the "install" is usually a single ROM file.
        var options = new Avalonia.Platform.Storage.FilePickerOpenOptions
        {
            Title         = $"Locate game files for \"{Title}\"",
            AllowMultiple = false,
            FileTypeFilter =
            [
                new Avalonia.Platform.Storage.FilePickerFileType("All files") { Patterns = ["*"] },
            ],
        };

        var result   = await mainWindow.StorageProvider.OpenFilePickerAsync(options)
                           .ConfigureAwait(true);
        var selected = result.FirstOrDefault();
        if (selected is null) return;

        var filePath = selected.Path.IsFile
            ? selected.Path.LocalPath
            : selected.Path.AbsolutePath;

        // Save install record — parent directory is the "install" root.
        var installPath = Path.GetDirectoryName(filePath) ?? filePath;
        _emulatorService.SetGameInstall(
            sourceGameId:   SourceGames.FirstOrDefault()?.SourceGameId ?? GameId,
            installPath:    installPath,
            detectedExePath: filePath,
            userLocated:    true);

        _toast.Success("Install located", $"Saved: {filePath}");

        // Recompute play state to reflect the new install record.
        _ = ComputeEmulationStateAsync();
    }

    /// <summary>
    /// Removes the install record for this emulated game from this device.
    /// Only available for MGA-managed installs (CanUninstallEmulatedGame).
    /// </summary>
    [RelayCommand]
    private void UninstallEmulatedGame()
    {
        if (_emulatorService is null) return;

        var sgId = SourceGames.FirstOrDefault()?.SourceGameId ?? GameId;
        _emulatorService.RemoveGameInstall(sgId);

        _toast.Success("Uninstalled", $"\"{Title}\" removed from this device.");

        // Recompute state — game should move back to NotInstalled.
        _ = ComputeEmulationStateAsync();
    }

    /// <summary>
    /// Navigates to Settings → Emulators tab so the user can add an emulator
    /// for this platform.
    /// </summary>
    [RelayCommand]
    private void SetupEmulator()
    {
        _toast.Info(
            "Setup emulator",
            $"Go to Settings → Emulators to add an emulator for \"{FormattedPlatform}\".");
    }

    /// <summary>
    /// Navigates to Settings → Emulators → BIOS section so the user can
    /// add the required BIOS files.
    /// </summary>
    [RelayCommand]
    private void OpenBiosManager()
    {
        _toast.Info(
            "BIOS files needed",
            $"{MissingBiosMessage}\n\nGo to Settings → Emulators to add BIOS files.");
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
                // The canonical game itself was deleted — go back (history preferred).
                if (_nav.CanGoBack)
                    _nav.NavigateBack();
                else
                    _nav.NavigateTo(new LibraryViewModel(
                        _server, _nav, _toast, _config,
                        installDetector:  _installDetector,
                        recentPlayed:     _recentPlayed,
                        gameStateService: _gameStateService));
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
                    result.CanonicalGameId, _server, _nav, _toast, _config,
                    _installDetector, _recentPlayed, _emulatorService, _gameStateService));
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
        if (_emulatorService is null || string.IsNullOrEmpty(Platform))
        {
            CanLaunchWithEmulator = false;
            EmulatorName          = string.Empty;
            return;
        }

        // Find configs for the current platform, ordered by priority.
        var configs = _emulatorService.GetConfigsForPlatform(Platform);

        if (configs.Count == 0)
        {
            CanLaunchWithEmulator = false;
            EmulatorName          = string.Empty;
            return;
        }

        // Use the highest-priority config's display name as the label.
        var primaryConfig    = configs[0];
        var install          = _emulatorService.GetInstall(primaryConfig.InstallId);
        CanLaunchWithEmulator = install is not null && !string.IsNullOrEmpty(install.ExecutablePath);
        EmulatorName          = install?.Name ?? primaryConfig.DisplayName;
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
            ReleaseDate = FormatReleaseDate(game.ReleaseDate);
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

            // Legacy emulator availability (used as fallback when GameStateService absent).
            CheckEmulatorAvailability();

            // Achievement summary.
            if (game.AchievementSummary is not null)
            {
                HasAchievements     = true;
                AchievementTotal    = game.AchievementSummary.TotalCount;
                AchievementUnlocked = game.AchievementSummary.UnlockedCount;
            }

            // Source games — each SourceGameRowViewModel maps its own SourceGameSummary.
            // Also keep the raw records for GameStateService emulation state computation.
            _rawSourceGames = game.SourceGames;
            SourceGames     = new ObservableCollection<SourceGameRowViewModel>(
                game.SourceGames.Select(sg => new SourceGameRowViewModel(sg)));
            HasSourceGames  = SourceGames.Count > 0;

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

            // Kick off emulation-state computation in the background (BIOS I/O).
            // Result arrives via PrimaryPlayState property update on the UI thread.
            if (_gameStateService is not null && GameStateService.IsEmulatedPlatform(Platform))
                _ = ComputeEmulationStateAsync();
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
    /// Parses an ISO-8601 date string from the API and returns a short "d MMM yyyy"
    /// display string (e.g. "5 Apr 2017").  Returns the raw value unchanged if parsing fails.
    /// </summary>
    private static string? FormatReleaseDate(string? raw)
    {
        if (string.IsNullOrWhiteSpace(raw)) return null;

        // Try ISO-8601 / RFC-3339 first (e.g. "2017-04-05T04:00:00Z").
        if (DateTimeOffset.TryParse(raw, out var dto))
            return dto.ToString("d MMM yyyy");

        return raw;
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

    // ---------------------------------------------------------------------------
    // Private — emulation state computation
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Computes <see cref="SourceGamePlayState"/> for all source games of this
    /// emulated game, picks the "best" state, and updates <see cref="PrimaryPlayState"/>.
    ///
    /// "Best" = highest play readiness (Ready > BiosMissing > NotConfigured > NotInstalled > Extras).
    /// </summary>
    private async Task ComputeEmulationStateAsync()
    {
        if (_gameStateService is null || _rawSourceGames.Count == 0)
            return;

        IsComputingEmulationState = true;

        try
        {
            SourceGamePlayState? best = null;
            var ct = CancellationToken.None; // page is alive; no cancellation token needed here

            // Run state computation off the UI thread (may involve BIOS file I/O).
            best = await Task.Run(async () =>
            {
                SourceGamePlayState? best = null;

                foreach (var sg in _rawSourceGames)
                {
                    var state = await _gameStateService.ComputeAsync(sg, Platform, ct)
                        .ConfigureAwait(false);

                    if (IsStateBetter(state, best))
                        best = state;
                }

                return best;
            }).ConfigureAwait(false);

            // Apply result on the UI thread.
            await Avalonia.Threading.Dispatcher.UIThread.InvokeAsync(() =>
            {
                PrimaryPlayState = best;
            });
        }
        catch
        {
            // Non-fatal — emulation state panel stays hidden on error.
        }
        finally
        {
            await Avalonia.Threading.Dispatcher.UIThread.InvokeAsync(() =>
            {
                IsComputingEmulationState = false;
            });
        }
    }

    /// <summary>
    /// Returns true when <paramref name="candidate"/> is a better play state than
    /// <paramref name="current"/> (or <paramref name="current"/> is null).
    /// </summary>
    private static bool IsStateBetter(SourceGamePlayState candidate, SourceGamePlayState? current)
    {
        if (current is null) return true;
        return StateRank(candidate) > StateRank(current);
    }

    private static int StateRank(SourceGamePlayState state) => state.Kind switch
    {
        GamePlayStateKind.PlainDirect or
        GamePlayStateKind.InstalledDirect
            => 10,

        GamePlayStateKind.PlainEmulated or
        GamePlayStateKind.InstalledEmulated
            when state.EmulatorState == EmulatorAvailability.Ready
            => 9,

        GamePlayStateKind.PlainEmulated or
        GamePlayStateKind.InstalledEmulated
            when state.EmulatorState == EmulatorAvailability.BiosMissing
            => 7,

        GamePlayStateKind.PlainEmulated or
        GamePlayStateKind.InstalledEmulated
            when state.EmulatorState == EmulatorAvailability.NotConfigured
            => 5,

        GamePlayStateKind.NotInstalled
            => 3,

        GamePlayStateKind.Extras
            => 1,

        _ => 0,
    };
}
