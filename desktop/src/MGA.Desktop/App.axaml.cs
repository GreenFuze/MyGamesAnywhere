using Avalonia;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Markup.Xaml;
using MGA.Desktop.Services;
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
            _config     = new AppConfigService();
            _serverConn = new ServerConnectionService(_config);
            _theme      = new ThemeService(_config, this);
            _nav        = new NavigationService();
            _toast      = new ToastService();

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
                _config, _serverConn, _theme, _nav, _toast, _installDetector);

            var window = new MainWindow { DataContext = _mainVm };

            // Dispose services when the window closes (RAII cleanup).
            window.Closed += (_, _) => DisposeServices();

            desktop.MainWindow = window;
        }

        base.OnFrameworkInitializationCompleted();
    }

    // ---------------------------------------------------------------------------
    // Cleanup
    // ---------------------------------------------------------------------------

    private void DisposeServices()
    {
        _mainVm?.Dispose();
        _installDetector?.Dispose();
        _nav?.Dispose();
        _theme?.Dispose();
        _serverConn?.Dispose();
        // AppConfigService, ToastService, and InstallBindingService have no unmanaged resources.
    }
}
