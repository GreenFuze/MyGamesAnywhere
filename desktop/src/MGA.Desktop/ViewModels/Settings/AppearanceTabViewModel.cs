using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Appearance tab — theme switcher + live server management.
///
/// Server management lets the user view known servers, switch between them,
/// and add a new server by URL. Uses ServerConnectionService.SwitchServer()
/// which updates the config and reconnects automatically.
/// </summary>
public sealed partial class AppearanceTabViewModel : ViewModelBase
{
    private readonly ThemeService            _theme;
    private readonly AppConfigService        _config;
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Theme
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _selectedTheme;

    public IReadOnlyList<string> AvailableThemes { get; } =
        new[] { "midnight", "daylight" };

    // ---------------------------------------------------------------------------
    // Server management
    // ---------------------------------------------------------------------------

    /// <summary>Known servers from config. Refreshed after add/switch.</summary>
    [ObservableProperty]
    private ObservableCollection<ServerProfile> _knownServers = [];

    /// <summary>The currently active server URL — reflects UrlChanged observable.</summary>
    [ObservableProperty]
    private string _activeServerUrl = string.Empty;

    [ObservableProperty]
    private string _newServerUrl = string.Empty;

    [ObservableProperty]
    private string _newServerName = string.Empty;

    [ObservableProperty]
    private bool _isConnecting;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public AppearanceTabViewModel(
        ThemeService             theme,
        AppConfigService         config,
        ServerConnectionService  server,
        ToastService             toast)
    {
        _theme          = theme;
        _config         = config;
        _server         = server;
        _toast          = toast;
        _selectedTheme  = theme.Current;

        // Sync theme selector with external changes.
        Disposables.Add(theme.Changed.Subscribe(id => SelectedTheme = id));

        // Sync active server URL display reactively.
        Disposables.Add(server.UrlChanged.Subscribe(url =>
        {
            ActiveServerUrl = url;
            RefreshKnownServers();
        }));

        RefreshKnownServers();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private void SwitchToServer(ServerProfile profile)
    {
        if (profile.Url == _server.ActiveUrl)
            return;

        _server.SwitchServer(profile);
        _toast.Info("Server switched", $"Now connected to {profile.Url}");
    }

    [RelayCommand(CanExecute = nameof(CanAddServer))]
    private async Task AddServerAsync()
    {
        var url  = NewServerUrl.Trim();
        var name = NewServerName.Trim();

        if (!Uri.TryCreate(url, UriKind.Absolute, out _))
        {
            _toast.Error("Invalid URL", "Enter a full URL such as http://hostname:8900");
            return;
        }

        IsConnecting = true;
        AddServerCommand.NotifyCanExecuteChanged();

        try
        {
            // Ping first to validate reachability.
            using var http = new System.Net.Http.HttpClient { BaseAddress = new Uri(url) };
            var api  = new MGA.Api.MgaApiService(http);
            var ok   = await api.PingAsync().ConfigureAwait(true);

            if (!ok)
            {
                _toast.Error("Cannot reach server", $"No response from {url}");
                return;
            }

            _server.ConnectToUrl(url, name);
            _toast.Info("Connected", $"Now connected to {url}");

            NewServerUrl  = string.Empty;
            NewServerName = string.Empty;
        }
        catch (Exception ex)
        {
            _toast.Error("Connection failed", ex.Message);
        }
        finally
        {
            IsConnecting = false;
            AddServerCommand.NotifyCanExecuteChanged();
        }
    }

    private bool CanAddServer() => !IsConnecting && !string.IsNullOrWhiteSpace(NewServerUrl);

    // ---------------------------------------------------------------------------
    // Theme change hook
    // ---------------------------------------------------------------------------

    partial void OnSelectedThemeChanged(string value)
    {
        if (value != _theme.Current)
            _theme.SetTheme(value);
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void RefreshKnownServers()
    {
        KnownServers = new ObservableCollection<ServerProfile>(_config.Config.Servers);
    }
}
