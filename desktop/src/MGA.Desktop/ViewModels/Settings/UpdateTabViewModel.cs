using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Update tab — shows server + client version info and lets the user
/// trigger an update check against the configured update manifest URL.
///
/// Update flow:
///   1. On load: GET /api/about (server version) + GET /api/update/status
///   2. "Check for Updates" button: POST /api/update/check → refreshes status
/// </summary>
public sealed partial class UpdateTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Loading / action state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isChecking;

    // ---------------------------------------------------------------------------
    // Server info (from GET /api/about)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _serverVersion = "–";

    [ObservableProperty]
    private string _serverBuildDate = "–";

    // ---------------------------------------------------------------------------
    // Update status (from GET /api/update/status)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _currentVersion = "–";

    [ObservableProperty]
    private string _latestVersion = "–";

    /// <summary>True when the server reports a newer version is available.</summary>
    [ObservableProperty]
    private bool _updateAvailable;

    [ObservableProperty]
    private string _statusMessage = string.Empty;

    [ObservableProperty]
    private bool _hasStatusMessage;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public UpdateTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Triggers a server-side update check and reloads the status.</summary>
    [RelayCommand]
    private async Task CheckForUpdatesAsync()
    {
        if (_server.Api is null)
            return;

        IsChecking = true;

        try
        {
            var status = await _server.Api.CheckForUpdatesAsync().ConfigureAwait(true);
            ApplyStatus(status);
            _toast.Info("Update check", UpdateAvailable ? $"Update available: {LatestVersion}" : "You are up to date.");
        }
        catch (Exception ex)
        {
            _toast.Error("Update check failed", ex.Message);
        }
        finally
        {
            IsChecking = false;
        }
    }

    /// <summary>Reloads version info and update status from the server.</summary>
    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

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
            // Fetch about info and update status in parallel.
            var aboutTask  = _server.Api.GetAboutInfoAsync();
            var statusTask = _server.Api.GetUpdateStatusAsync();

            await Task.WhenAll(aboutTask, statusTask).ConfigureAwait(true);

            // Apply server build metadata.
            var about = await aboutTask;
            ServerVersion   = about.Version;
            ServerBuildDate = about.BuildDate;

            // Apply update status.
            ApplyStatus(await statusTask);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load update info", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void ApplyStatus(MGA.Api.UpdateStatus status)
    {
        CurrentVersion   = status.CurrentVersion;
        LatestVersion    = string.IsNullOrEmpty(status.LatestVersion) ? "–" : status.LatestVersion;
        UpdateAvailable  = status.UpdateAvailable;
        StatusMessage    = status.Message ?? string.Empty;
        HasStatusMessage = !string.IsNullOrEmpty(StatusMessage);
    }
}
