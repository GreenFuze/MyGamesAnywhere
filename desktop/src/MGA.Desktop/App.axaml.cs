using Avalonia;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Markup.Xaml;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;
using MGA.Desktop.Services.Install;
using MGA.Desktop.ViewModels;
using MGA.Desktop.Views;

namespace MGA.Desktop;

/// <summary>
/// Application entry point — manually constructs and wires the service graph.
/// No DI container; every object receives exactly what it needs.
/// </summary>
public partial class App : Application
{
    // Top-level services owned by App; disposed on shutdown.
    private AppConfigService?          _config;
    private ServerConnectionService?   _serverConn;
    private ThemeService?              _theme;
    private NavigationService?         _nav;
    private ToastService?              _toast;
    private InstallDetectionService?   _installDetector;
    private RecentPlayedService?       _recentPlayed;
    private GameCacheService?          _gameCache;
    private MediaCacheService?         _mediaCache;
    private EmulatorCatalogService?    _emulatorCatalog;
    private EmulatorService?           _emulatorService;
    private GameStateService?          _gameStateService;
    private DeepLinkService?           _deepLink;
    private MainWindowViewModel?       _mainVm;

    /// <summary>Exposed so MainWindow code-behind can bind the toast overlay.</summary>
    public ToastService? ToastService => _toast;

    public override void Initialize()
    {
        AvaloniaXamlLoader.Load(this);
    }

    public override void OnFrameworkInitializationCompleted()
    {
        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime desktop)
        {
            // Build service graph bottom-up (RAII order: dependencies first).
            _config       = new AppConfigService();
            _serverConn   = new ServerConnectionService(_config);
            _theme        = new ThemeService(_config, this);
            _nav          = new NavigationService();
            _toast        = new ToastService();
            _recentPlayed = new RecentPlayedService(_config);
            _gameCache    = new GameCacheService();
            _mediaCache   = new MediaCacheService();

            // Emulation services — catalog is loaded from embedded JSON (fail-fast).
            _emulatorCatalog  = new EmulatorCatalogService();
            _emulatorService  = new EmulatorService(_config, _emulatorCatalog);
            _gameStateService = new GameStateService(_emulatorService, _emulatorCatalog);

            // Install detection — wires all storefront + ARP detectors.
            var bindings = new InstallBindingService();
            _installDetector = new InstallDetectionService(
                detectors: new IInstallDetector[]
                {
                    new SteamInstallDetector(),
                    new ArpInstallDetector(),
                },
                bindings: bindings);

            // Root ViewModel drives the whole shell (also creates OnboardingViewModel if needed).
            _mainVm = new MainWindowViewModel(
                _config, _serverConn, _theme, _nav, _toast,
                _installDetector, _recentPlayed, _gameCache, _mediaCache,
                _emulatorService);

            // Deep link service — single-instance pipe server + mga:// URI handler.
            _deepLink = new DeepLinkService(_nav, _serverConn, _toast, _config, _recentPlayed);

            var window = new MainWindow { DataContext = _mainVm };

            // Start deep-link pipe server once the window is shown, then
            // process any startup URI passed on the command line.
            window.Opened += (_, _) =>
            {
                _deepLink.StartServer();
                _deepLink.HandleStartupUri(Program.StartupUri);

                // Eagerly warm the game cache in the background so Play/Library
                // are instant on first navigation — no spinner, no wait.
                _ = WarmGameCacheAsync();
            };

            // Dispose services when the window closes (RAII cleanup).
            window.Closed += (_, _) => DisposeServices();

            desktop.MainWindow = window;
        }

        base.OnFrameworkInitializationCompleted();
    }

    // ---------------------------------------------------------------------------
    // Eager cache warmup
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Proactively warms both the game list cache and the media (cover art) cache
    /// immediately after the window opens, so Play/Library render instantly on first
    /// navigation — no spinner, no remote image round-trips.
    ///
    /// Two phases:
    ///   1. Game list — skip if already warm (disk cache survives restarts);
    ///      otherwise fetch from server and persist via GameCacheService.
    ///   2. Media — always run regardless of game-cache state so cover art is
    ///      pre-fetched even when the game list came from disk.  Downloads all
    ///      cover images plus the first 30 preview (screenshot/header) images
    ///      used by the recently-played shelf.
    ///
    /// Silently swallows any network error — pages fall back to their own
    /// cold-load paths if the cache is empty when they open.
    /// </summary>
    private async Task WarmGameCacheAsync()
    {
        if (_serverConn?.Api is null || _gameCache is null)
            return;

        var serverUrl = _serverConn.ActiveUrl;
        if (string.IsNullOrWhiteSpace(serverUrl))
            return;

        var api = _serverConn.Api;

        try
        {
            // ── Phase 1: game list ────────────────────────────────────────────
            IReadOnlyList<MGA.Api.GameDetail> games;

            if (_gameCache.TryGet(serverUrl, out var cached))
            {
                // Disk / memory cache is warm — use it directly.
                games = cached;
            }
            else
            {
                // Cold start — fetch and persist (must update cache on UI thread).
                var response = await api.ListGamesAsync(page: 0, pageSize: 500)
                                        .ConfigureAwait(false);

                await Avalonia.Threading.Dispatcher.UIThread
                    .InvokeAsync(() => _gameCache.Update(serverUrl, response.Games));

                games = response.Games;
            }

            // ── Phase 2: media — always run ──────────────────────────────────
            if (_mediaCache is null || games.Count == 0)
                return;

            // All cover images (small, essential for every grid/list tile).
            var coverUrls = games
                .SelectMany(g => g.Media.Where(m => m.Type == "cover")
                                        .Select(m => api.GetMediaUrl(m.Url)))
                .Where(u => !string.IsNullOrEmpty(u));

            // First 30 preview (screenshot / header) images — used by the hero
            // banner and card hover overlays on the Play page.
            var previewUrls = games
                .SelectMany(g => g.Media.Where(m => m.Type == "screenshot" || m.Type == "header")
                                        .Take(1)              // one preview per game
                                        .Select(m => api.GetMediaUrl(m.Url)))
                .Where(u => !string.IsNullOrEmpty(u))
                .Take(30);

            // Fire both warm jobs concurrently; they share the semaphore throttle.
            await Task.WhenAll(
                _mediaCache.WarmAsync(coverUrls),
                _mediaCache.WarmAsync(previewUrls)
            ).ConfigureAwait(false);
        }
        catch
        {
            // Background warmup — swallow failures silently.
            // The individual page VMs handle their own error toasts on cold load.
        }
    }

    // ---------------------------------------------------------------------------
    // Cleanup
    // ---------------------------------------------------------------------------

    private void DisposeServices()
    {
        _mainVm?.Dispose();
        _deepLink?.Dispose();
        _installDetector?.Dispose();
        _nav?.Dispose();
        _theme?.Dispose();
        _serverConn?.Dispose();
        _mediaCache?.Dispose();
        // AppConfigService, ToastService, RecentPlayedService, and InstallBindingService
        // have no unmanaged resources.
    }
}
