using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Cache tab — shows cache entry count + total disk size,
/// and allows the user to clear the entire source cache.
/// </summary>
public sealed partial class CacheTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isClearing;

    [ObservableProperty]
    private int _entryCount;

    /// <summary>Human-readable total size, e.g. "2.4 GB" or "450 MB".</summary>
    [ObservableProperty]
    private string _totalSizeText = "–";

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public CacheTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Clears all cache entries via POST /api/cache/clear, then reloads the stats.</summary>
    [RelayCommand]
    private async Task ClearCacheAsync()
    {
        if (_server.Api is null)
            return;

        IsClearing = true;

        try
        {
            await _server.Api.ClearCacheAsync().ConfigureAwait(true);
            _toast.Success("Cache cleared", "All source-cache entries removed.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Cache clear failed", ex.Message);
        }
        finally
        {
            IsClearing = false;
        }
    }

    /// <summary>Reloads cache stats from the server.</summary>
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
            var response = await _server.Api.GetCacheEntriesAsync().ConfigureAwait(true);

            EntryCount   = response.Entries.Count;
            TotalSizeText = FormatBytes(response.Entries.Sum(e => e.Size));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load cache info", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private static string FormatBytes(long bytes)
    {
        const long KB = 1024;
        const long MB = KB * 1024;
        const long GB = MB * 1024;

        return bytes switch
        {
            >= GB => $"{bytes / (double)GB:F1} GB",
            >= MB => $"{bytes / (double)MB:F1} MB",
            >= KB => $"{bytes / (double)KB:F1} KB",
            _     => $"{bytes} B",
        };
    }
}
