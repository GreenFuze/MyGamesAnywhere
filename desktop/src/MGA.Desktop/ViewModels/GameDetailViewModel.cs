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

/// <summary>Display model for one source game row.</summary>
public sealed class SourceGameRowViewModel
{
    public string IntegrationLabel { get; init; } = string.Empty;
    public string Platform         { get; init; } = string.Empty;
    public string Kind             { get; init; } = string.Empty;
    public string RawTitle         { get; init; } = string.Empty;
    public string Status           { get; init; } = string.Empty;
    public int    FileCount        { get; init; }
    public string FileSummary      { get; init; } = string.Empty;

    /// <summary>Absolute file paths for ROM launch. First entry is the primary ROM.</summary>
    public List<string> FilePaths  { get; init; } = [];
}

/// <summary>Display model for one external link.</summary>
public sealed class ExternalLinkViewModel
{
    public string Source     { get; init; } = string.Empty;
    public string ExternalId { get; init; } = string.Empty;
    public string? Url       { get; init; }
    public bool HasUrl => !string.IsNullOrEmpty(Url);
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
    private readonly AppConfigService        _config;

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

    /// <summary>True when at least one screenshot/header image is available.</summary>
    [ObservableProperty]
    private bool _hasScreenshots;

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
    // Source games + external IDs
    // ---------------------------------------------------------------------------

    /// <summary>Source games attached to this canonical entry.</summary>
    [ObservableProperty]
    private ObservableCollection<SourceGameRowViewModel> _sourceGames = [];

    [ObservableProperty]
    private bool _hasSourceGames;

    /// <summary>External ID links (IGDB, Steam, etc.).</summary>
    [ObservableProperty]
    private ObservableCollection<ExternalLinkViewModel> _externalLinks = [];

    [ObservableProperty]
    private bool _hasExternalLinks;

    /// <summary>True when the game has a non-zero rating value.</summary>
    [ObservableProperty]
    private bool _hasRating;

    // ---------------------------------------------------------------------------
    // Emulator launch
    // ---------------------------------------------------------------------------

    /// <summary>True when an emulator is configured for this game's platform.</summary>
    [ObservableProperty]
    private bool _canLaunchWithEmulator;

    /// <summary>Name of the matched emulator (shown in button tooltip).</summary>
    [ObservableProperty]
    private string _emulatorName = string.Empty;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public GameDetailViewModel(
        string                  gameId,
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast,
        AppConfigService        config)
    {
        GameId  = gameId;
        _server = server;
        _nav    = nav;
        _toast  = toast;
        _config = config;

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

    /// <summary>Toggles the favorite flag via PUT/DELETE /api/games/{id}/favorite.</summary>
    [RelayCommand]
    private async Task ToggleFavoriteAsync()
    {
        if (_server.Api is null)
            return;

        bool newValue = !Favorite;
        try
        {
            await _server.Api.SetFavoriteAsync(GameId, newValue).ConfigureAwait(true);
            Favorite = newValue;
            _toast.Success(
                newValue ? "Added to favorites" : "Removed from favorites",
                Title);
        }
        catch (Exception ex)
        {
            _toast.Error("Could not update favorite", ex.Message);
        }
    }

    /// <summary>Navigates back to the library.</summary>
    [RelayCommand]
    private void GoBack()
    {
        _nav.NavigateTo(new LibraryViewModel(_server, _nav, _toast, _config));
    }

    /// <summary>Launches the game using the configured emulator for its platform.</summary>
    [RelayCommand]
    private void LaunchWithEmulator()
    {
        var emulators = _config.GetEmulators();
        var emulator  = FindEmulatorForPlatform(Platform, emulators);

        if (emulator is null)
        {
            _toast.Error("No emulator", $"No emulator configured for platform \"{Platform}\". Add one in Settings → Emulators.");
            return;
        }

        // Find the primary ROM file path.
        var romPath = SourceGames
            .SelectMany(sg => sg.FilePaths)
            .FirstOrDefault();

        if (string.IsNullOrEmpty(romPath))
        {
            _toast.Error("No files", "This game has no local files available for launch.");
            return;
        }

        try
        {
            // Build args: replace {rom} with the quoted ROM path.
            var args = emulator.ArgsTemplate.Replace("{rom}", $"\"{romPath}\"");

            System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
            {
                FileName        = emulator.ExecutablePath,
                Arguments       = args,
                UseShellExecute = true,
            });

            _toast.Success("Launched", $"Started \"{Title}\" with {emulator.Name}.");
        }
        catch (Exception ex)
        {
            _toast.Error("Launch failed", ex.Message);
        }
    }

    /// <summary>Opens an external link in the system browser.</summary>
    [RelayCommand]
    private void OpenExternalLink(ExternalLinkViewModel link)
    {
        if (string.IsNullOrEmpty(link.Url)) return;
        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(link.Url) { UseShellExecute = true });
    }

    // ---------------------------------------------------------------------------
    // Private — emulator helpers
    // ---------------------------------------------------------------------------

    private void CheckEmulatorAvailability()
    {
        var emulators = _config.GetEmulators();
        var matched   = FindEmulatorForPlatform(Platform, emulators);

        CanLaunchWithEmulator = matched is not null;
        EmulatorName          = matched?.Name ?? string.Empty;
    }

    private static EmulatorEntry? FindEmulatorForPlatform(string platform, List<EmulatorEntry> emulators)
    {
        if (string.IsNullOrEmpty(platform))
            return null;

        return emulators.FirstOrDefault(e =>
            e.Platforms
             .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)
             .Any(p => p.Equals(platform, StringComparison.OrdinalIgnoreCase)));
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
            Title    = game.Title;
            Platform = game.Platform;
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
            HasScreenshots = Screenshots.Count > 0;

            // Set rating visibility flag.
            HasRating = Rating > 0;

            // Check whether a configured emulator matches this game's platform.
            CheckEmulatorAvailability();

            // Achievement summary.
            if (game.AchievementSummary is not null)
            {
                HasAchievements    = true;
                AchievementTotal   = game.AchievementSummary.TotalCount;
                AchievementUnlocked = game.AchievementSummary.UnlockedCount;
            }

            // Populate source games.
            SourceGames = new ObservableCollection<SourceGameRowViewModel>(
                game.SourceGames.Select(sg => new SourceGameRowViewModel
                {
                    IntegrationLabel = sg.IntegrationLabel ?? sg.IntegrationId,
                    Platform         = sg.Platform,
                    Kind             = sg.Kind,
                    RawTitle         = sg.RawTitle,
                    Status           = sg.Status,
                    FileCount        = sg.Files.Count,
                    FileSummary      = sg.RootPath is not null
                        ? $"{sg.Files.Count} file(s) in {sg.RootPath}"
                        : $"{sg.Files.Count} file(s)",
                    FilePaths        = sg.Files.Select(f => f.Path).ToList(),
                }));
            HasSourceGames = SourceGames.Count > 0;

            // Populate external links.
            ExternalLinks = new ObservableCollection<ExternalLinkViewModel>(
                game.ExternalIds
                    .Where(e => !string.IsNullOrEmpty(e.Url))
                    .Select(e => new ExternalLinkViewModel
                    {
                        Source     = e.Source,
                        ExternalId = e.ExternalId,
                        Url        = e.Url,
                    }));
            HasExternalLinks = ExternalLinks.Count > 0;
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
