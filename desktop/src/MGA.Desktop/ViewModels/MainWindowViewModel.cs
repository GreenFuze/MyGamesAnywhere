using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
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
    private bool _sidebarCollapsed;

    /// <summary>Active theme ID ("midnight" | "daylight").</summary>
    [ObservableProperty]
    private string _currentTheme = "midnight";

    /// <summary>
    /// Base URL of the currently connected server (e.g. "http://tv2:8900").
    /// Shown in the title bar. Empty string when not connected.
    /// </summary>
    [ObservableProperty]
    private string _activeServerUrl = string.Empty;

    /// <summary>Text bound to the TitleBar search box. Press Enter to search.</summary>
    [ObservableProperty]
    private string _globalSearchText = string.Empty;

    // ---------------------------------------------------------------------------
    // Nav items
    // ---------------------------------------------------------------------------

    public IReadOnlyList<NavItem> NavItems { get; }

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
        InstallDetectionService?  installDetector = null,
        RecentPlayedService?      recentPlayed    = null)
    {
        _config          = config;
        _serverConn      = serverConn;
        _theme           = theme;
        _nav             = nav;
        _toast           = toast;
        _installDetector = installDetector;
        _recentPlayed    = recentPlayed;

        // Restore persisted shell state.
        SidebarCollapsed = config.Config.SidebarCollapsed;
        CurrentTheme     = theme.Current;

        // Build the sidebar nav list.
        NavItems = new NavItem[]
        {
            new("play",         "Play",         "▶"),
            new("library",      "Library",      "▤"),
            new("achievements", "Achievements", "★"),
            new("stats",        "Stats",        "╪"),
            new("settings",     "Settings",     "⚙"),
            new("about",        "About",        "ℹ"),
        };

        // Cache the Library item so badge updates don't scan the list each time.
        _libraryNavItem = NavItems.First(n => n.PageId == "library");

        // Mirror NavigationService → CurrentPage (with old-VM disposal).
        Disposables.Add(
            nav.CurrentPage.Subscribe(vm =>
            {
                var old = CurrentPage;
                CurrentPage = vm;
                if (!ReferenceEquals(old, vm))
                    old?.Dispose();
            }));

        // Mirror ThemeService → CurrentTheme.
        Disposables.Add(
            theme.Changed.Subscribe(id => CurrentTheme = id));

        // Reflect the currently connected server URL — BehaviorSubject fires immediately
        // on subscribe (initial value) and again on every future switch.
        Disposables.Add(
            serverConn.UrlChanged.Subscribe(url => ActiveServerUrl = url));

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
    /// Executes a global search: navigates to Library and pre-fills the search text.
    /// Called when the user presses Enter in the TitleBar search box.
    /// </summary>
    [RelayCommand]
    private void GlobalSearch()
    {
        var query = GlobalSearchText.Trim();
        if (string.IsNullOrEmpty(query))
            return;

        // Mark Library as active in the sidebar.
        foreach (var item in NavItems)
            item.IsActive = item.PageId == "library";

        // Navigate to Library with the search query pre-filled.
        _nav.NavigateTo(new LibraryViewModel(
            _serverConn, _nav, _toast, _config,
            initialSearch: query, installDetector: _installDetector, recentPlayed: _recentPlayed));

        // Clear the search box after navigation.
        GlobalSearchText = string.Empty;
    }

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
        "play"         => new PlayViewModel(_serverConn, _nav, _toast, _config, _installDetector, _recentPlayed),
        "library"      => new LibraryViewModel(_serverConn, _nav, _toast, _config, installDetector: _installDetector, recentPlayed: _recentPlayed),
        "achievements" => new AchievementsViewModel(_serverConn, _toast),
        "stats"        => new StatsViewModel(_serverConn, _toast),
        "settings"     => new SettingsViewModel(_serverConn, _theme, _config, _toast),
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
        base.Dispose();
    }
}

// ---------------------------------------------------------------------------
// NavItem — one entry in the sidebar navigation list
// ---------------------------------------------------------------------------

/// <summary>
/// Represents a single navigation entry in the sidebar.
/// IsActive is observable so the sidebar style can highlight the selected item.
/// </summary>
public sealed partial class NavItem : ObservableObject
{
    public string PageId { get; }
    public string Label  { get; }

    /// <summary>Icon character — Unicode symbol or Segoe MDL2 codepoint.</summary>
    public string Icon   { get; }

    [ObservableProperty]
    private bool _isActive;

    /// <summary>Numeric badge shown beside the label (e.g. total game count). 0 = hidden.</summary>
    [ObservableProperty]
    private int _badgeCount;

    /// <summary>True when <see cref="BadgeCount"/> is greater than zero.</summary>
    public bool HasBadge => BadgeCount > 0;

    partial void OnBadgeCountChanged(int value) => OnPropertyChanged(nameof(HasBadge));

    public NavItem(string pageId, string label, string icon)
    {
        PageId = pageId;
        Label  = label;
        Icon   = icon;
    }
}
