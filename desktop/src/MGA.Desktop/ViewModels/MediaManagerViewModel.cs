using System.Collections.ObjectModel;
using System.Diagnostics;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// MediaAssetModel
// ---------------------------------------------------------------------------

/// <summary>
/// Display model for a single media asset tile in the Media Manager.
/// Carries resolved URL, dimensions, type badge, and the raw asset_id
/// needed by override commands.
/// </summary>
public sealed class MediaAssetModel
{
    public int    AssetId      { get; }
    public string Type         { get; }

    /// <summary>Badge text shown on the tile, e.g. "cover", "background", "screenshot".</summary>
    public string TypeBadge    { get; }

    public string  ImageUrl     { get; } = string.Empty;
    public string  Dimensions   { get; } = string.Empty;

    /// <summary>True when this asset is the current cover override.</summary>
    public bool   IsCoverNow   { get; }

    /// <summary>True when this asset is used as cover by any source (not necessarily overridden).</summary>
    public bool   IsDefaultCover { get; }

    /// <summary>Local file path if available (null for remote-only assets).</summary>
    public string? LocalPath  { get; }

    /// <summary>True when this asset is a video type.</summary>
    public bool    IsVideo    { get; }

    /// <summary>True when this asset is an image type.</summary>
    public bool    IsImage    { get; }

    /// <summary>True when this asset is hosted on YouTube.</summary>
    public bool    IsYouTube  { get; }

    public MediaAssetModel(GameMedia m, MgaApiService? api, GameDetail game)
    {
        AssetId      = m.AssetId;
        Type         = m.Type;
        TypeBadge    = m.Type.Length > 0
                       ? char.ToUpperInvariant(m.Type[0]) + m.Type[1..]
                       : "Media";
        ImageUrl     = api?.GetMediaUrl(m.Url) ?? m.Url;
        Dimensions   = m.Width > 0 && m.Height > 0 ? $"{m.Width}×{m.Height}" : string.Empty;
        IsCoverNow   = game.CoverOverride?.AssetId == m.AssetId;
        IsDefaultCover = m.Type == "cover" && game.CoverOverride is null;

        // Derive media-kind flags from type and URL.
        var url = ImageUrl ?? string.Empty;
        IsVideo   = m.Type.Equals("video", StringComparison.OrdinalIgnoreCase)
                    || url.EndsWith(".mp4", StringComparison.OrdinalIgnoreCase)
                    || url.EndsWith(".mkv", StringComparison.OrdinalIgnoreCase)
                    || url.EndsWith(".webm", StringComparison.OrdinalIgnoreCase);
        IsYouTube = url.Contains("youtube.com", StringComparison.OrdinalIgnoreCase)
                    || url.Contains("youtu.be", StringComparison.OrdinalIgnoreCase);
        IsImage   = !IsVideo && !IsYouTube;

        // LocalPath: not provided by the API; may be set externally if needed.
        LocalPath = null;
    }
}

// ---------------------------------------------------------------------------
// MediaManagerViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Media Manager page — shows all media assets for a game and lets the user
/// set cover / background / hover overrides from the existing asset set.
/// Navigated to from Game Detail via "Manage Media" button.
/// </summary>
public sealed partial class MediaManagerViewModel : ViewModelBase
{
    private readonly string                   _gameId;
    private readonly ServerConnectionService  _server;
    private readonly NavigationService        _nav;
    private readonly ToastService             _toast;
    private readonly AppConfigService         _config;
    private readonly InstallDetectionService? _installDetector;
    private readonly GameStateService?        _gameStateService;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private string _gameTitle = string.Empty;

    /// <summary>All media assets returned by the game detail, grouped display-order.</summary>
    [ObservableProperty]
    private ObservableCollection<MediaAssetModel> _assets = [];

    /// <summary>True when there is at least one asset to show.</summary>
    [ObservableProperty]
    private bool _hasAssets;

    /// <summary>True when a cover override is currently set (enables the Clear button).</summary>
    [ObservableProperty]
    private bool _hasCoverOverride;

    /// <summary>The currently selected asset for the preview panel; null when none.</summary>
    [ObservableProperty]
    private MediaAssetModel? _selectedAsset;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public MediaManagerViewModel(
        string                   gameId,
        ServerConnectionService  server,
        NavigationService        nav,
        ToastService             toast,
        AppConfigService         config,
        InstallDetectionService? installDetector  = null,
        GameStateService?        gameStateService = null)
    {
        _gameId          = gameId;
        _server          = server;
        _nav             = nav;
        _toast           = toast;
        _config          = config;
        _installDetector  = installDetector;
        _gameStateService = gameStateService;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Navigates back to Game Detail for this game.</summary>
    [RelayCommand]
    private void Back() =>
        _nav.NavigateTo(new GameDetailViewModel(
            _gameId, _server, _nav, _toast, _config,
            _installDetector, gameStateService: _gameStateService));

    /// <summary>Selects an asset and shows it in the preview panel.</summary>
    [RelayCommand]
    private void SelectAsset(MediaAssetModel asset)
    {
        SelectedAsset = asset;
    }

    /// <summary>Opens the asset URL in the system browser.</summary>
    [RelayCommand]
    private void OpenInBrowser(MediaAssetModel asset)
    {
        var url = asset.ImageUrl;
        if (string.IsNullOrEmpty(url)) return;

        try
        {
            Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Cannot open URL", ex.Message);
        }
    }

    /// <summary>Opens the asset's local file in the default system player.</summary>
    [RelayCommand]
    private void OpenInPlayer(MediaAssetModel asset)
    {
        var path = asset.LocalPath;
        if (string.IsNullOrEmpty(path)) return;

        try
        {
            Process.Start(new ProcessStartInfo(path) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Cannot open file", ex.Message);
        }
    }

    /// <summary>Sets the given asset as the cover override for this game.</summary>
    [RelayCommand]
    private async Task SetCoverAsync(MediaAssetModel asset)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.SetCoverOverrideAsync(_gameId, asset.AssetId).ConfigureAwait(true);
            _toast.Success("Cover updated", $"Cover set to {asset.TypeBadge} asset #{asset.AssetId}.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to set cover", ex.Message);
        }
    }

    /// <summary>Clears the cover override, restoring automatic cover selection.</summary>
    [RelayCommand]
    private async Task ClearCoverAsync()
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.DeleteCoverOverrideAsync(_gameId).ConfigureAwait(true);
            _toast.Success("Cover cleared", "Cover override removed; auto-selection restored.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to clear cover", ex.Message);
        }
    }

    /// <summary>Sets the given asset as the background/hero override.</summary>
    [RelayCommand]
    private async Task SetBackgroundAsync(MediaAssetModel asset)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.SetBackgroundOverrideAsync(_gameId, asset.AssetId).ConfigureAwait(true);
            _toast.Success("Background updated", $"Background set to asset #{asset.AssetId}.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to set background", ex.Message);
        }
    }

    /// <summary>Sets the given asset as the hover/card image override.</summary>
    [RelayCommand]
    private async Task SetHoverAsync(MediaAssetModel asset)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.SetHoverOverrideAsync(_gameId, asset.AssetId).ConfigureAwait(true);
            _toast.Success("Hover image updated", $"Hover image set to asset #{asset.AssetId}.");
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to set hover image", ex.Message);
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
            // Game detail already includes the full media array.
            var detail = await _server.Api.GetGameAsync(_gameId).ConfigureAwait(true);

            GameTitle      = detail.Title;
            HasCoverOverride = detail.CoverOverride is not null;

            // Sort by type priority: covers first, then backgrounds, then others.
            var sorted = detail.Media
                .OrderBy(m => m.Type switch
                {
                    "cover"      => 0,
                    "background" => 1,
                    "hero"       => 2,
                    "screenshot" => 3,
                    _            => 4,
                })
                .ThenBy(m => m.AssetId);

            Assets = new ObservableCollection<MediaAssetModel>(
                sorted.Select(m => new MediaAssetModel(m, _server.Api, detail)));

            HasAssets = Assets.Count > 0;
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load media", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
