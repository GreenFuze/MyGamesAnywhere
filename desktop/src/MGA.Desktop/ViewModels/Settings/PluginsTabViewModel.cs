using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Display model for a single plugin row in the Plugins tab.</summary>
public sealed class PluginRowModel
{
    public string PluginId      { get; init; } = string.Empty;
    public string Version       { get; init; } = string.Empty;

    /// <summary>Comma-joined list of the plugin's Provides entries.</summary>
    public string ProvidesText  { get; init; } = string.Empty;
}

/// <summary>
/// Plugins tab — lists server-side plugins that provide game sources and enrichment.
/// Data is fetched from GET /api/plugins on construction and on manual reload.
/// </summary>
public sealed partial class PluginsTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<PluginRowModel> _plugins = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public PluginsTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        IsLoading = true;

        try
        {
            var list = await _server.Api.GetPluginsAsync().ConfigureAwait(true);

            Plugins = new ObservableCollection<PluginRowModel>(list.Select(p => new PluginRowModel
            {
                PluginId     = p.PluginId,
                Version      = p.Version,
                ProvidesText = string.Join(", ", p.Provides),
            }));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load plugins", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
