using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Display model for a single integration row.</summary>
public sealed class IntegrationRowModel
{
    public string IntegrationId { get; init; } = string.Empty;
    public string Label         { get; init; } = string.Empty;
    public string PluginId      { get; init; } = string.Empty;

    /// <summary>"ok", "error", "pending", etc.</summary>
    public string Status  { get; init; } = string.Empty;
    public string Message { get; init; } = string.Empty;

    /// <summary>True when the server reports a non-ok status for this integration.</summary>
    public bool HasError => Status is "error" or "failed";
}

/// <summary>
/// Integrations tab — lists live integration status and lets the user
/// trigger a per-integration refresh.
/// </summary>
public sealed partial class IntegrationsTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<IntegrationRowModel> _integrations = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public IntegrationsTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Triggers a background refresh for the given integration via
    /// POST /api/integrations/{id}/refresh.
    /// </summary>
    [RelayCommand]
    private async Task RefreshIntegrationAsync(string integrationId)
    {
        if (_server.Api is null)
            return;

        try
        {
            await _server.Api.RefreshIntegrationAsync(integrationId).ConfigureAwait(true);
            _toast.Success("Integration refresh", "Refresh started.");
        }
        catch (Exception ex)
        {
            _toast.Error("Refresh failed", ex.Message);
        }
    }

    /// <summary>Reloads the full integration status list.</summary>
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
            var entries = await _server.Api.GetIntegrationStatusAsync().ConfigureAwait(true);

            Integrations = new ObservableCollection<IntegrationRowModel>(
                entries.Select(e => new IntegrationRowModel
                {
                    IntegrationId = e.IntegrationId,
                    Label         = e.Label,
                    PluginId      = e.PluginId,
                    Status        = e.Status,
                    Message       = e.Message,
                }));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load integrations", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
