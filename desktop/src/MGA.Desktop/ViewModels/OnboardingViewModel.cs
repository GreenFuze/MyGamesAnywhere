using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// First-run screen: prompt the user to enter a server URL, test connectivity,
/// then invoke the onConnected callback so MainWindowViewModel can transition
/// to the main shell.
/// </summary>
public sealed partial class OnboardingViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;
    private readonly Action                  _onConnected;

    [ObservableProperty]
    private string _serverUrl = "http://localhost:8900";

    [ObservableProperty]
    private bool _isTesting;

    [ObservableProperty]
    private string? _testError;

    public OnboardingViewModel(
        ServerConnectionService server,
        ToastService            toast,
        Action                  onConnected)
    {
        _server      = server;
        _toast       = toast;
        _onConnected = onConnected;
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand(CanExecute = nameof(CanConnect))]
    private async Task ConnectAsync()
    {
        TestError = null;
        IsTesting = true;
        ConnectCommand.NotifyCanExecuteChanged();

        try
        {
            // Probe the server with a temporary HttpClient before committing.
            using var http = new System.Net.Http.HttpClient
            {
                BaseAddress = new Uri(ServerUrl.TrimEnd('/')),
                Timeout     = TimeSpan.FromSeconds(5),
            };

            var ok = await new MGA.Api.MgaApiService(http).PingAsync();
            if (!ok)
                throw new InvalidOperationException("Server did not respond with HTTP 200.");

            // Connection works — persist the URL and notify the shell.
            _server.ConnectToUrl(ServerUrl.TrimEnd('/'));
            _toast.Success("Connected!", $"Linked to {ServerUrl.TrimEnd('/')}");
            _onConnected();
        }
        catch (Exception ex)
        {
            TestError = ex.Message;
        }
        finally
        {
            IsTesting = false;
            ConnectCommand.NotifyCanExecuteChanged();
        }
    }

    private bool CanConnect() => !IsTesting && !string.IsNullOrWhiteSpace(ServerUrl);
}
