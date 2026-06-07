using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Connection tab — server management panel that lets the user view known servers,
/// switch between them, and add a new server by URL.
///
/// Uses ServerConnectionService.SwitchServer() / ConnectToUrl() which update the
/// config and reconnect automatically.
/// </summary>
public sealed partial class ConnectionTabViewModel : ViewModelBase
{
    private readonly AppConfigService        _config;
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
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

    public ConnectionTabViewModel(
        AppConfigService        config,
        ServerConnectionService server,
        ToastService            toast)
    {
        _config = config;
        _server = server;
        _toast  = toast;

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
    // Private helpers
    // ---------------------------------------------------------------------------

    private void RefreshKnownServers()
    {
        KnownServers = new ObservableCollection<ServerProfile>(_config.Config.Servers);
    }
}
