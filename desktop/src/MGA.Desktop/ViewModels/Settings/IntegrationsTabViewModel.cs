using System.Collections.ObjectModel;
using System.Diagnostics;
using System.Text.Json;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Row view-model for a configured integration
// ---------------------------------------------------------------------------

/// <summary>Display model for a single configured integration row.</summary>
public sealed partial class IntegrationRowViewModel : ObservableObject
{
    public string IntegrationId    { get; init; } = string.Empty;
    public string PluginId         { get; init; } = string.Empty;
    public string Label            { get; init; } = string.Empty;
    public string IntegrationType  { get; init; } = string.Empty;

    /// <summary>Double-encoded JSON string from the API.</summary>
    public string ConfigJson { get; init; } = string.Empty;

    [ObservableProperty]
    private string _status = string.Empty;

    [ObservableProperty]
    private string _message = string.Empty;

    /// <summary>True when the server reports an error or failed state.</summary>
    public bool HasError => Status is "error" or "failed";
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Integrations tab — full CRUD for configured integrations, with a
/// plugin-selection wizard, dynamic config fields, OAuth flow support,
/// and scan-job progress polling.
/// </summary>
public sealed partial class IntegrationsTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // Poll intervals
    private static readonly TimeSpan ScanPollInterval    = TimeSpan.FromSeconds(2);
    private static readonly TimeSpan OAuthPollInterval   = TimeSpan.FromSeconds(3);
    private static readonly TimeSpan OAuthPollTimeout    = TimeSpan.FromSeconds(60);

    // ---------------------------------------------------------------------------
    // Observable state — loading / list
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<IntegrationRowViewModel> _integrations = [];

    // ---------------------------------------------------------------------------
    // Observable state — add/edit wizard
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isAddingIntegration;

    [ObservableProperty]
    private ObservableCollection<PluginRowModel> _availablePlugins = [];

    [ObservableProperty]
    private PluginRowModel? _selectedPlugin;

    [ObservableProperty]
    private ObservableCollection<ConfigFieldModel> _configFields = [];

    [ObservableProperty]
    private string _newIntegrationLabel = string.Empty;

    [ObservableProperty]
    private string _newIntegrationType = "source";

    [ObservableProperty]
    private bool _isSaving;

    /// <summary>The integration currently being edited; null when creating a new one.</summary>
    [ObservableProperty]
    private IntegrationRowViewModel? _editingIntegration;

    // ---------------------------------------------------------------------------
    // Observable state — scan
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isScanning;

    [ObservableProperty]
    private string _scanStatus = string.Empty;

    [ObservableProperty]
    private string? _scanJobId;

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
    // Commands — list
    // ---------------------------------------------------------------------------

    /// <summary>Reloads both the integration list and their live status.</summary>
    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    // ---------------------------------------------------------------------------
    // Commands — add / edit wizard
    // ---------------------------------------------------------------------------

    /// <summary>Opens the add-integration wizard and loads the available plugins.</summary>
    [RelayCommand]
    private async Task AddIntegrationAsync()
    {
        if (_server.Api is null)
            return;

        // Reset wizard state.
        EditingIntegration    = null;
        SelectedPlugin        = null;
        NewIntegrationLabel   = string.Empty;
        NewIntegrationType    = "source";
        ConfigFields          = [];
        IsAddingIntegration   = true;

        try
        {
            var plugins = await _server.Api.GetPluginsAsync().ConfigureAwait(true);
            AvailablePlugins = new ObservableCollection<PluginRowModel>(
                plugins.Select(p => new PluginRowModel
                {
                    PluginId     = p.PluginId,
                    Version      = p.Version,
                    ProvidesText = string.Join(", ", p.Provides),
                }));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load plugins", ex.Message);
            IsAddingIntegration = false;
        }
    }

    /// <summary>Cancels the wizard without saving.</summary>
    [RelayCommand]
    private void CancelAdd()
    {
        IsAddingIntegration = false;
        EditingIntegration  = null;
        SelectedPlugin      = null;
        ConfigFields        = [];
    }

    /// <summary>
    /// Step 1 of the wizard: user picks a plugin.
    /// Loads the plugin's config schema and populates ConfigFields.
    /// </summary>
    [RelayCommand]
    private async Task SelectPluginAsync(PluginRowModel plugin)
    {
        if (_server.Api is null)
            return;

        SelectedPlugin = plugin;
        ConfigFields   = [];

        try
        {
            var pluginDetail = await _server.Api.GetPluginAsync(plugin.PluginId).ConfigureAwait(true);
            ConfigFields     = BuildConfigFields(pluginDetail.ConfigSchema, existingConfigJson: null);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load plugin schema", ex.Message);
            SelectedPlugin = null;
        }
    }

    /// <summary>Goes back to the plugin-selection step in the wizard.</summary>
    [RelayCommand]
    private void BackToPluginSelection()
    {
        SelectedPlugin = null;
        ConfigFields   = [];
    }

    /// <summary>
    /// Creates or updates an integration.
    /// Handles HTTP 202 OAuth flow: opens browser, then polls until authorized.
    /// </summary>
    [RelayCommand]
    private async Task SaveIntegrationAsync()
    {
        if (_server.Api is null || SelectedPlugin is null)
            return;

        IsSaving = true;

        try
        {
            var config = BuildConfigDict();

            if (EditingIntegration is null)
            {
                // Create new integration.
                var (dto, oauth) = await _server.Api.CreateIntegrationAsync(
                    SelectedPlugin.PluginId,
                    NewIntegrationLabel,
                    NewIntegrationType,
                    config).ConfigureAwait(true);

                if (oauth is not null)
                {
                    await HandleOAuthFlowAsync(oauth, integrationId: null).ConfigureAwait(true);
                }
                else
                {
                    _toast.Success("Integration created", dto!.Label);
                }
            }
            else
            {
                // Update existing integration.
                var (dto, oauth) = await _server.Api.UpdateIntegrationAsync(
                    EditingIntegration.IntegrationId,
                    NewIntegrationLabel,
                    NewIntegrationType,
                    config).ConfigureAwait(true);

                if (oauth is not null)
                {
                    await HandleOAuthFlowAsync(oauth, EditingIntegration.IntegrationId).ConfigureAwait(true);
                }
                else
                {
                    _toast.Success("Integration updated", dto!.Label);
                }
            }

            // Reload and close wizard.
            IsAddingIntegration = false;
            EditingIntegration  = null;
            SelectedPlugin      = null;
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Save failed", ex.Message);
        }
        finally
        {
            IsSaving = false;
        }
    }

    /// <summary>
    /// Opens the edit wizard pre-filled with the integration's existing config.
    /// </summary>
    [RelayCommand]
    private async Task EditIntegrationAsync(IntegrationRowViewModel row)
    {
        if (_server.Api is null)
            return;

        EditingIntegration  = row;
        NewIntegrationLabel = row.Label;
        NewIntegrationType  = row.IntegrationType;
        SelectedPlugin      = null;
        ConfigFields        = [];
        IsAddingIntegration = true;

        // Load plugins list so the wizard has them for display, then select the current plugin.
        try
        {
            var plugins = await _server.Api.GetPluginsAsync().ConfigureAwait(true);
            AvailablePlugins = new ObservableCollection<PluginRowModel>(
                plugins.Select(p => new PluginRowModel
                {
                    PluginId     = p.PluginId,
                    Version      = p.Version,
                    ProvidesText = string.Join(", ", p.Provides),
                }));

            // Auto-select the current plugin.
            SelectedPlugin = AvailablePlugins.FirstOrDefault(p => p.PluginId == row.PluginId);

            if (SelectedPlugin is not null)
            {
                // Load its schema and pre-fill with existing values.
                var pluginDetail = await _server.Api.GetPluginAsync(row.PluginId).ConfigureAwait(true);
                ConfigFields     = BuildConfigFields(pluginDetail.ConfigSchema, row.ConfigJson);
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load integration config", ex.Message);
            IsAddingIntegration = false;
        }
    }

    /// <summary>Deletes an integration after confirmation (confirmed by command invocation).</summary>
    [RelayCommand]
    private async Task DeleteIntegrationAsync(string integrationId)
    {
        if (_server.Api is null)
            return;

        try
        {
            await _server.Api.DeleteIntegrationAsync(integrationId).ConfigureAwait(true);
            _toast.Success("Integration deleted", "The integration has been removed.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Delete failed", ex.Message);
        }
    }

    /// <summary>Triggers a background refresh for a specific integration.</summary>
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

    // ---------------------------------------------------------------------------
    // Commands — scan
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Starts a scan job for all integrations, then polls for progress every 2 s
    /// until the job reaches a terminal state.
    /// </summary>
    [RelayCommand]
    private async Task ScanAsync()
    {
        if (_server.Api is null || IsScanning)
            return;

        IsScanning = true;
        ScanStatus = "Starting scan…";

        try
        {
            var job = await _server.Api.StartScanAsync().ConfigureAwait(true);
            ScanJobId  = job.JobId;
            ScanStatus = FormatScanStatus(job);

            // Poll until terminal state.
            while (!IsTerminalScanStatus(job.Status))
            {
                await Task.Delay(ScanPollInterval).ConfigureAwait(true);

                if (_server.Api is null)
                    break;

                job        = await _server.Api.GetScanJobAsync(ScanJobId!).ConfigureAwait(true);
                ScanStatus = FormatScanStatus(job);
            }

            // Report final result.
            if (job.Status == "completed")
            {
                _toast.Success("Scan complete", $"Scanned {job.IntegrationCount} integration(s).");
                await LoadAsync().ConfigureAwait(true);
            }
            else if (job.Status == "failed")
            {
                _toast.Error("Scan failed", job.Error ?? "Unknown error.");
            }
            else
            {
                _toast.Info("Scan stopped", $"Status: {job.Status}.");
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Scan error", ex.Message);
        }
        finally
        {
            IsScanning = false;
            ScanJobId  = null;
            ScanStatus = string.Empty;
        }
    }

    /// <summary>Cancels the currently running scan job.</summary>
    [RelayCommand]
    private async Task CancelScanAsync()
    {
        if (_server.Api is null || ScanJobId is null)
            return;

        try
        {
            await _server.Api.CancelScanJobAsync(ScanJobId).ConfigureAwait(true);
            ScanStatus = "Cancelling…";
        }
        catch (Exception ex)
        {
            _toast.Error("Cancel scan failed", ex.Message);
        }
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
            // Load both the integration list and their live status in parallel.
            var integrationsTask = _server.Api.ListIntegrationsAsync();
            var statusTask       = _server.Api.GetIntegrationStatusAsync();

            await Task.WhenAll(integrationsTask, statusTask).ConfigureAwait(true);

            var integrations = integrationsTask.Result;
            var statusMap    = statusTask.Result
                .ToDictionary(s => s.IntegrationId, s => s);

            Integrations = new ObservableCollection<IntegrationRowViewModel>(
                integrations.Select(dto =>
                {
                    statusMap.TryGetValue(dto.Id, out var statusEntry);
                    return new IntegrationRowViewModel
                    {
                        IntegrationId   = dto.Id,
                        PluginId        = dto.PluginId,
                        Label           = dto.Label,
                        IntegrationType = dto.IntegrationType,
                        ConfigJson      = dto.ConfigJson,
                        Status          = statusEntry?.Status  ?? "pending",
                        Message         = statusEntry?.Message ?? string.Empty,
                    };
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

    // ---------------------------------------------------------------------------
    // Private — OAuth handling
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Opens the browser for OAuth authorization and polls AuthorizeIntegration
    /// every 3 s until success or 60 s timeout.
    /// </summary>
    private async Task HandleOAuthFlowAsync(OAuthRequiredResponse oauth, string? integrationId)
    {
        // Open the browser.
        try
        {
            Process.Start(new ProcessStartInfo(oauth.AuthorizeUrl) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Could not open browser", ex.Message);
            return;
        }

        _toast.Info("Authorizing…", "Check your browser to complete authorization.");

        if (integrationId is null)
            return; // New integration: can't poll without an id yet.

        // Poll for up to 60 s.
        var deadline = DateTime.UtcNow.Add(OAuthPollTimeout);
        while (DateTime.UtcNow < deadline)
        {
            await Task.Delay(OAuthPollInterval).ConfigureAwait(true);

            if (_server.Api is null)
                return;

            try
            {
                var (status, _) = await _server.Api.AuthorizeIntegrationAsync(integrationId).ConfigureAwait(true);
                if (status?.Status == "ok")
                {
                    _toast.Success("Authorization complete", "Integration is now authorized.");
                    return;
                }
            }
            catch
            {
                // Ignore transient poll errors; keep trying until deadline.
            }
        }

        _toast.Error("Authorization timed out", "Please try again or check your credentials.");
    }

    // ---------------------------------------------------------------------------
    // Private — config field helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Builds observable ConfigFieldModel list from a plugin's schema dict
    /// and optionally pre-populates values from a double-encoded config JSON string.
    /// </summary>
    private static ObservableCollection<ConfigFieldModel> BuildConfigFields(
        Dictionary<string, System.Text.Json.JsonElement>? schema,
        string? existingConfigJson)
    {
        if (schema is null or { Count: 0 })
            return [];

        // Parse existing values if available.
        Dictionary<string, JsonElement>? existingValues = null;
        if (!string.IsNullOrEmpty(existingConfigJson))
        {
            try
            {
                existingValues = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(existingConfigJson);
            }
            catch
            {
                // Ignore malformed config JSON; fields will be empty.
            }
        }

        var fields = schema.Select(kvp =>
        {
            JsonElement? currentValue = null;

            if (existingValues is not null && existingValues.TryGetValue(kvp.Key, out var foundValue))
                currentValue = foundValue;

            return ConfigFieldModel.FromSchema(kvp.Key, kvp.Value, currentValue);
        });

        return new ObservableCollection<ConfigFieldModel>(fields);
    }

    /// <summary>
    /// Collects values from ConfigFields into a dictionary suitable for the API body.
    /// </summary>
    private Dictionary<string, object> BuildConfigDict()
    {
        var dict = new Dictionary<string, object>();

        foreach (var field in ConfigFields)
        {
            object value = field.Type == ConfigFieldType.Boolean
                ? (object)field.BoolValue
                : field.StringValue;

            dict[field.Key] = value;
        }

        return dict;
    }

    // ---------------------------------------------------------------------------
    // Private — scan helpers
    // ---------------------------------------------------------------------------

    private static string FormatScanStatus(ScanJobStatus job)
    {
        if (job.IntegrationCount > 0)
        {
            var label = job.CurrentIntegrationLabel is not null
                ? $" ({job.CurrentIntegrationLabel})"
                : string.Empty;

            return $"Scanning… {job.IntegrationsCompleted}/{job.IntegrationCount} integrations{label}";
        }

        return $"Scanning… [{job.Status}]";
    }

    private static bool IsTerminalScanStatus(string status) =>
        status is "completed" or "failed" or "cancelled";
}
