using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>Single media item (screenshot or header) for the detail-page carousel.</summary>
public sealed class MediaItemModel
{
    public string Url { get; init; } = string.Empty;
}

/// <summary>
/// Game detail page — full-bleed hero banner, metadata panel, and action bar.
///
/// Loaded on construction via _ = LoadAsync().
/// All navigation and toast calls happen on the UI thread (ConfigureAwait(true)).
/// </summary>
public sealed partial class GameDetailViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Identity
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _gameId = string.Empty;

    // ---------------------------------------------------------------------------
    // Loading state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    // ---------------------------------------------------------------------------
    // Metadata
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _title = string.Empty;

    [ObservableProperty]
    private string _platform = string.Empty;

    [ObservableProperty]
    private string? _description;

    [ObservableProperty]
    private string? _releaseDate;

    [ObservableProperty]
    private string? _developer;

    [ObservableProperty]
    private string? _publisher;

    [ObservableProperty]
    private double _rating;

    /// <summary>Comma-separated genre list, e.g. "Action, RPG".</summary>
    [ObservableProperty]
    private string _genresText = string.Empty;

    // ---------------------------------------------------------------------------
    // Media
    // ---------------------------------------------------------------------------

    /// <summary>URL for the hero background image (background media, or cover as fallback).</summary>
    [ObservableProperty]
    private string? _heroImageUrl;

    [ObservableProperty]
    private string? _coverUrl;

    /// <summary>Screenshot and header images — shown in the media carousel strip.</summary>
    [ObservableProperty]
    private ObservableCollection<MediaItemModel> _screenshots = [];

    // ---------------------------------------------------------------------------
    // Favorite / Achievements
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _favorite;

    [ObservableProperty]
    private int _achievementUnlocked;

    [ObservableProperty]
    private int _achievementTotal;

    [ObservableProperty]
    private bool _hasAchievements;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public GameDetailViewModel(
        string                  gameId,
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        GameId  = gameId;
        _server = server;
        _nav    = nav;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Opens the game in the browser at {serverUrl}/game/{id}/play.</summary>
    [RelayCommand]
    private void PlayInBrowser()
    {
        var url = $"{_server.ActiveUrl}/game/{Uri.EscapeDataString(GameId)}/play";

        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(url) { UseShellExecute = true });
    }

    /// <summary>Stub — toggles the favorite state. Full implementation to follow.</summary>
    [RelayCommand]
    private Task ToggleFavoriteAsync()
    {
        _toast.Info("Coming soon", "Toggle favorite is not yet implemented.");
        return Task.CompletedTask;
    }

    /// <summary>Navigates back to the library.</summary>
    [RelayCommand]
    private void GoBack()
    {
        _nav.NavigateTo(new LibraryViewModel(_server, _nav, _toast));
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
            // Fetch the full game detail from the server.
            var game = await _server.Api.GetGameAsync(GameId).ConfigureAwait(true);

            // Populate scalar metadata.
            Title       = game.Title;
            Platform    = game.Platform;
            Description = game.Description;
            ReleaseDate = game.ReleaseDate;
            Developer   = game.Developer;
            Publisher   = game.Publisher;
            Rating      = game.Rating;
            Favorite    = game.Favorite;
            GenresText  = string.Join(", ", game.Genres);

            // Resolve cover URL.
            var coverMedia = game.CoverOverride
                             ?? game.Media.FirstOrDefault(m => m.Type == "cover");

            CoverUrl = coverMedia is not null && _server.Api is not null
                ? _server.Api.GetMediaUrl(coverMedia.Url)
                : null;

            // Resolve hero/background URL: prefer explicit background media,
            // fall back to the cover.
            var heroMedia = game.Media.FirstOrDefault(m => m.Type == "background");

            HeroImageUrl = heroMedia is not null && _server.Api is not null
                ? _server.Api.GetMediaUrl(heroMedia.Url)
                : CoverUrl;

            // Populate screenshot carousel.
            Screenshots = new ObservableCollection<MediaItemModel>(
                game.Media
                    .Where(m => m.Type == "screenshot" || m.Type == "header")
                    .Select(m => new MediaItemModel
                    {
                        Url = _server.Api!.GetMediaUrl(m.Url),
                    }));

            // Achievement summary.
            if (game.AchievementSummary is not null)
            {
                HasAchievements    = true;
                AchievementTotal   = game.AchievementSummary.TotalCount;
                AchievementUnlocked = game.AchievementSummary.UnlockedCount;
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load game", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
