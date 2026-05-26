using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Cache entry row display model
// ---------------------------------------------------------------------------

/// <summary>Display row for one source-cache entry.</summary>
public sealed partial class CacheEntryRowViewModel : ObservableObject
{
    public string CanonicalTitle   { get; }
    public string SourceTitle      { get; }
    public string IntegrationLabel { get; }
    public string Status           { get; }
    public string SizeText         { get; }
    public int    FileCount        { get; }

    /// <summary>The canonical game ID — used by the Prepare Cache command.</summary>
    public string CanonicalGameId  { get; }

    /// <summary>True while a prepare-cache call is in flight for this row.</summary>
    [ObservableProperty]
    private bool _isPreparing;

    public CacheEntryRowViewModel(CacheEntryDto dto)
    {
        CanonicalTitle   = dto.CanonicalTitle;
        SourceTitle      = dto.SourceTitle;
        IntegrationLabel = dto.IntegrationLabel;
        Status           = dto.Status;
        SizeText         = ByteFormatter.Format(dto.Size);
        FileCount        = dto.FileCount;
        CanonicalGameId  = dto.Id;  // cache entry id is the canonical game id per API
    }
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Cache tab — shows the list of source-cache entries (canonical title, integration,
/// status, size, file count) and allows clearing the entire cache or triggering
/// preparation for individual entries.
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
    private string _totalSizeText = string.Empty;

    [ObservableProperty]
    private ObservableCollection<CacheEntryRowViewModel> _entries = [];

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

    /// <summary>Reloads cache stats and entry list from the server.</summary>
    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    /// <summary>Clears all cache entries via POST /api/cache/clear, then reloads.</summary>
    [RelayCommand]
    private async Task ClearCacheAsync()
    {
        if (_server.Api is null) return;

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

    /// <summary>Triggers cache preparation for a single entry's canonical game.</summary>
    [RelayCommand]
    private async Task PrepareCacheAsync(CacheEntryRowViewModel entry)
    {
        if (_server.Api is null || entry.IsPreparing) return;

        entry.IsPreparing = true;

        try
        {
            await _server.Api.PrepareCacheAsync(entry.CanonicalGameId).ConfigureAwait(true);
            _toast.Success("Preparation triggered", $"Cache preparation started for \"{entry.CanonicalTitle}\".");

            // Reload so the status reflects the new state.
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Prepare failed", ex.Message);
        }
        finally
        {
            entry.IsPreparing = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        IsLoading = true;

        try
        {
            var response = await _server.Api.GetCacheEntriesAsync().ConfigureAwait(true);

            EntryCount    = response.Entries.Count;
            TotalSizeText = ByteFormatter.Format(response.Entries.Sum(e => e.Size));

            Entries = new ObservableCollection<CacheEntryRowViewModel>(
                response.Entries.Select(e => new CacheEntryRowViewModel(e)));
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
}
