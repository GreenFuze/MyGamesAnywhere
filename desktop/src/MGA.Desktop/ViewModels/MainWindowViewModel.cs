using Avalonia.Media;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;
using MGA.Desktop.Services.Install;
using System.Reactive.Linq;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Top-level shell ViewModel.
///
/// Owns the onboarding gate, sidebar state, page navigation, and theme switching.
/// All services are injected manually — no DI container.
/// </summary>
public sealed partial class MainWindowViewModel : ViewModelBase
{
    private readonly AppConfigService          _config;
    private readonly ServerConnectionService   _serverConn;
    private readonly ThemeService              _theme;
    private readonly NavigationService         _nav;
    private readonly ToastService              _toast;
    private readonly InstallDetectionService?  _installDetector;
    private readonly RecentPlayedService?      _recentPlayed;
    private readonly GameCacheService?         _gameCache;
    private readonly MediaCacheService?        _mediaCache;
    private readonly EmulatorService?          _emulatorService;
    private readonly GameStateService?         _gameStateService;

    // Pre-loaded page ViewModels — created eagerly so their data fetches run
    // in the background while the user is still on the Play page.
    // Each is consumed (set to null) on first navigation; after that a fresh
    // VM is created normally.
    private AchievementsViewModel? _preloadedAchievements;
    private StatsViewModel?        _preloadedStats;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    /// <summary>True while the onboarding (first-run URL entry) overlay is shown.</summary>
    [ObservableProperty]
    private bool _isShowingOnboarding;

    /// <summary>
    /// The OnboardingViewModel instance — non-null only while onboarding is active.
    /// Bound to the onboarding ContentControl in MainWindow.axaml.
    /// </summary>
    [ObservableProperty]
    private OnboardingViewModel? _onboardingVm;

    /// <summary>The currently active page ViewModel (bound to the ContentPresenter).</summary>
    [ObservableProperty]
    private ViewModelBase? _currentPage;

    /// <summary>Whether the sidebar is in icon-only mode.</summary>
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(SidebarExpanded))]
    private bool _sidebarCollapsed;

    /// <summary>Inverse of <see cref="SidebarCollapsed"/>; used in AXAML visibility bindings.</summary>
    public bool SidebarExpanded => !SidebarCollapsed;

    /// <summary>Active theme ID ("midnight" | "daylight").</summary>
    [ObservableProperty]
    private string _currentTheme = "midnight";

    /// <summary>
    /// Base URL of the currently connected server (e.g. "http://tv2:8900").
    /// Shown in the title bar. Empty string when not connected.
    /// </summary>
    [ObservableProperty]
    private string _activeServerUrl = string.Empty;

    /// <summary>
    /// Display name for the active gamer profile shown in the sidebar pill.
    /// Falls back to the raw ID when no friendly name is known, or "No Profile"
    /// when nothing is selected.  Updated reactively via <see cref="ServerConnectionService.ProfileIdChanged"/>.
    /// </summary>
    [ObservableProperty]
    private string _activeProfileDisplay = "No Profile";

    // ---------------------------------------------------------------------------
    // Nav items
    // ---------------------------------------------------------------------------

    public IReadOnlyList<NavItem> NavItems { get; }

    /// <summary>Exposes the NavigationService so MainWindow can wire the mouse back button.</summary>
    public NavigationService Nav => _nav;

    // Reference kept for badge-count updates without list lookup on every event.
    private NavItem _libraryNavItem = null!;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public MainWindowViewModel(
        AppConfigService          config,
        ServerConnectionService   serverConn,
        ThemeService              theme,
        NavigationService         nav,
        ToastService              toast,
        InstallDetectionService?  installDetector  = null,
        RecentPlayedService?      recentPlayed     = null,
        GameCacheService?         gameCache        = null,
        MediaCacheService?        mediaCache       = null,
        EmulatorService?          emulatorService  = null,
        GameStateService?         gameStateService = null)
    {
        _config          = config;
        _serverConn      = serverConn;
        _theme           = theme;
        _nav             = nav;
        _toast           = toast;
        _installDetector = installDetector;
        _recentPlayed    = recentPlayed;
        _gameCache       = gameCache;
        _mediaCache      = mediaCache;
        _emulatorService  = emulatorService;
        _gameStateService = gameStateService;

        // Restore persisted shell state.
        SidebarCollapsed = config.Config.SidebarCollapsed;
        CurrentTheme     = theme.Current;

        // Build the sidebar nav list.
        // Icons are Segoe MDL2 Assets glyphs rendered via FontFamily="Segoe MDL2 Assets"
        // in Sidebar.axaml.  Glyphs: Play=, Library=, Trophy=,
        // BarChart=, Settings=, Info=.
        // Each item gets its own accent color so the active pill/icon feel
        // personal to that section — gaming green, electric blue, trophy gold, etc.
        NavItems = new NavItem[]
        {
            new("play",         "Play",         "", Color.Parse("#22c55e")),
            new("library",      "Library",      "", Color.Parse("#60a5fa")),
            new("achievements", "Achievements", "", Color.Parse("#fbbf24")),
            new("stats",        "Stats",        "", Color.Parse("#c084fc")),
            new("settings",     "Settings",     "", Color.Parse("#94a3b8")),
            new("about",        "About",        "", Color.Parse("#94a3b8")),
        };

        // Cache the Library item so badge updates don't scan the list each time.
        _libraryNavItem = NavItems.First(n => n.PageId == "library");

        // Mirror NavigationService → CurrentPage (with old-VM disposal).
        // When navigating back, the outgoing page was restored from history and must NOT be
        // disposed — it is still alive and referenced by the history stack.
        Disposables.Add(
            nav.CurrentPage.Subscribe(vm =>
            {
                var old = CurrentPage;
                CurrentPage = vm;
                if (!ReferenceEquals(old, vm) && !_nav.IsNavigatingBack)
                    old?.Dispose();
            }));

        // Mirror ThemeService → CurrentTheme.
        Disposables.Add(
            theme.Changed.Subscribe(id => CurrentTheme = id));

        // Reflect the currently connected server URL — BehaviorSubject fires immediately
        // on subscribe (initial value) and again on every future switch.
        Disposables.Add(
            serverConn.UrlChanged.Subscribe(url => ActiveServerUrl = url));

        // Keep the sidebar profile pill up-to-date whenever the active profile changes.
        // Seed with the persisted display name (or ID if name not yet cached) on startup.
        // Subsequent switches fire ProfileIdChanged with the friendly display name.
        var storedName = serverConn.ActiveProfileDisplayName;
        var storedId   = serverConn.ActiveProfileId;
        ActiveProfileDisplay = !string.IsNullOrWhiteSpace(storedName)
            ? storedName
            : !string.IsNullOrWhiteSpace(storedId)
                ? storedId
                : "No Profile";

        Disposables.Add(
            serverConn.ProfileIdChanged.Subscribe(label =>
                ActiveProfileDisplay = string.IsNullOrWhiteSpace(label) ? "No Profile" : label));

        // When the server is switched at runtime, GamerProfileId has been cleared.
        // Re-open onboarding so the user picks a profile on the new server.
        // Skip(1) skips the BehaviorSubject's initial replay on subscribe.
        Disposables.Add(
            serverConn.UrlChanged
                .Skip(1)
                .Subscribe(_ =>
                {
                    if (_config.Config.IsFirstRun)
                        Avalonia.Threading.Dispatcher.UIThread.Post(BeginOnboarding);
                }));

        // Wire SSE: refresh the Library badge whenever an integration refresh finishes.
        if (serverConn.Events is not null)
            Disposables.Add(
                serverConn.Events.Of("integration_refresh_complete")
                    .Subscribe(msg => Avalonia.Threading.Dispatcher.UIThread.Post(
                        () => _ = RefreshLibraryCountAsync())));

        // Show onboarding on first run; otherwise go straight to Play.
        if (config.Config.IsFirstRun)
        {
            BeginOnboarding();
        }
        else
        {
            NavigateTo("play");

            // Populate the Library badge immediately on startup.
            _ = RefreshLibraryCountAsync();

            // Pre-warm Achievements and Stats so they're instant on first navigation.
            // These VMs start their data fetch in their constructors; by the time
            // the user clicks either nav item the response should already be cached.
            _preloadedAchievements = new AchievementsViewModel(_serverConn, _toast);
            _preloadedStats        = new StatsViewModel(_serverConn, _toast);
        }
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Navigate to a page by its string ID (called from Sidebar nav items).</summary>
    [RelayCommand]
    private void NavigateTo(string pageId)
    {
        // Mark the matching nav item as active; clear all others.
        foreach (var item in NavItems)
            item.IsActive = item.PageId == pageId;

        var vm = CreatePageViewModel(pageId);
        if (vm is not null)
            _nav.NavigateTo(vm);
    }

    [RelayCommand]
    private void ToggleSidebar()
    {
        SidebarCollapsed = !SidebarCollapsed;
        _config.Update(cfg => cfg.SidebarCollapsed = SidebarCollapsed);
    }

    /// <summary>
    // ---------------------------------------------------------------------------
    // Onboarding
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Called by OnboardingViewModel when the server connection succeeds.
    /// Dismisses the onboarding overlay and navigates to the Play page.
    /// </summary>
    public void CompleteOnboarding()
    {
        // Dispose and clear the onboarding ViewModel.
        var old = OnboardingVm;
        OnboardingVm = null;
        old?.Dispose();

        IsShowingOnboarding = false;
        NavigateTo("play");
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void BeginOnboarding()
    {
        // Create the OnboardingViewModel with a callback to dismiss itself.
        OnboardingVm        = new OnboardingViewModel(_serverConn, _config, _toast, CompleteOnboarding);
        IsShowingOnboarding = true;
    }

    private ViewModelBase? CreatePageViewModel(string pageId) => pageId switch
    {
        "play"         => new PlayViewModel(_serverConn, _nav, _toast, _config, _installDetector, _recentPlayed, _gameCache, _mediaCache, _gameStateService),
        "library"      => new LibraryViewModel(_serverConn, _nav, _toast, _config, installDetector: _installDetector, recentPlayed: _recentPlayed, gameCache: _gameCache, mediaCache: _mediaCache, gameStateService: _gameStateService),
        "achievements" => ConsumePreloaded(ref _preloadedAchievements) ?? new AchievementsViewModel(_serverConn, _toast),
        "stats"        => ConsumePreloaded(ref _preloadedStats)         ?? new StatsViewModel(_serverConn, _toast),
        "settings"     => _emulatorService is not null
                              ? new SettingsViewModel(_serverConn, _theme, _config, _toast, _emulatorService)
                              : new SettingsViewModel(_serverConn, _theme, _config, _toast,
                                    new EmulatorService(_config, new EmulatorCatalogService())),
        "about"        => new AboutViewModel(_serverConn),
        _              => null,
    };

    // ---------------------------------------------------------------------------
    // Private — live badge updates
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Fetches the total game count from the server and updates the Library nav-item badge.
    /// Best-effort: exceptions are swallowed so a transient failure never shows a toast.
    /// </summary>
    private async Task RefreshLibraryCountAsync()
    {
        if (_serverConn.Api is null)
            return;

        try
        {
            // page_size=1 — we only need the Total field, not the full game list.
            var resp = await _serverConn.Api.ListGamesAsync(page: 0, pageSize: 1)
                .ConfigureAwait(true);

            _libraryNavItem.BadgeCount = resp.Total;
        }
        catch
        {
            // Badge is best-effort; don't surface transient network errors.
        }
    }

    public override void Dispose()
    {
        OnboardingVm?.Dispose();
        CurrentPage?.Dispose();
        // Dispose any pre-loaded VMs that were never navigated to.
        _preloadedAchievements?.Dispose();
        _preloadedStats?.Dispose();
        base.Dispose();
    }

    /// <summary>
    /// Returns the pre-loaded ViewModel and sets the field to null so a subsequent
    /// call (after navigate-away + navigate-back) falls through to a fresh instance.
    /// </summary>
    private static T? ConsumePreloaded<T>(ref T? field) where T : class
    {
        var vm = field;
        field  = null;
        return vm;
    }
}

// ---------------------------------------------------------------------------
// NavItem — one entry in the sidebar navigation list
// ---------------------------------------------------------------------------

/// <summary>
/// Represents a single navigation entry in the sidebar.
/// IsActive drives the per-nav colored pill, icon color, and left accent strip.
/// AccentColor is per-nav so each section has its own visual identity.
/// </summary>
public sealed partial class NavItem : ObservableObject
{
    public string PageId     { get; }
    public string Label      { get; }

    /// <summary>Icon character — Unicode symbol or Segoe MDL2 codepoint.</summary>
    public string Icon       { get; }

    /// <summary>Per-nav accent color (e.g. gaming green, trophy gold).</summary>
    public Color  AccentColor { get; init; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CurrentForeground))]
    private bool _isActive;

    /// <summary>Numeric badge shown beside the label (e.g. total game count). 0 = hidden.</summary>
    [ObservableProperty]
    private int _badgeCount;

    /// <summary>True when <see cref="BadgeCount"/> is greater than zero.</summary>
    public bool HasBadge => BadgeCount > 0;

    partial void OnBadgeCountChanged(int value) => OnPropertyChanged(nameof(HasBadge));

    /// <summary>
    /// Foreground brush: full accent when this item is active, muted grey at rest.
    /// Bound to the nav icon and label so both light up in the item's unique color.
    /// </summary>
    public IBrush CurrentForeground =>
        IsActive ? new SolidColorBrush(AccentColor)
                 : new SolidColorBrush(Color.Parse("#8f8aa3"));

    /// <summary>Full-opacity accent brush for the left strip and badge background.</summary>
    public SolidColorBrush AccentBrush => new SolidColorBrush(AccentColor);

    public NavItem(string pageId, string label, string icon, Color accentColor)
    {
        PageId      = pageId;
        Label       = label;
        Icon        = icon;
        AccentColor = accentColor;
    }
}
