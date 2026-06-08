using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Api;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Install;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Flat display model for a game card tile.
/// Constructed from a <see cref="GameDetail"/> API model so Views only bind
/// to simple string/bool properties — no API models leak into AXAML.
///
/// Inherits <see cref="ObservableObject"/> so that <see cref="IsSelected"/> can
/// be two-way bound to checkboxes in bulk-select mode.
/// </summary>
public sealed partial class GameCardModel : ObservableObject
{
    public string Id               { get; init; } = string.Empty;
    public string Title            { get; init; } = string.Empty;
    public string Platform         { get; init; } = string.Empty;

    /// <summary>Absolute cover-image URL resolved via the API service, or null.</summary>
    public string? CoverUrl        { get; init; }

    /// <summary>
    /// Landscape preview image URL (first screenshot/header in media list),
    /// optionally resolved to a local cache path via MediaCacheService.
    /// Used for the card hover overlay background.
    /// </summary>
    public string? PreviewUrl { get; init; }

    /// <summary>Explicit hover-overlay image (type="hover") — overrides screenshot when present.</summary>
    public string? HoverUrl { get; init; }

    /// <summary>Background art (type="background") — used for hero banners and card hover backgrounds.</summary>
    public string? BackgroundUrl { get; init; }

    /// <summary>
    /// Best image for the hover overlay: explicit hover override > background art >
    /// screenshot/header > cover art.
    /// </summary>
    public string? BestPreviewUrl => HoverUrl ?? PreviewUrl ?? CoverUrl;

    /// <summary>Platform-specific accent color for the platform badge chip.</summary>
    public string PlatformBadgeColor { get; init; } = "#334155";

    public bool   Favorite         { get; init; }
    public bool   CanPlay          { get; init; }
    public string Kind             { get; init; } = string.Empty;
    public string Developer        { get; init; } = string.Empty;
    public string Publisher        { get; init; } = string.Empty;
    public List<string> Genres     { get; init; } = [];

    /// <summary>Release year parsed from the API's release_date string; 0 when unknown.</summary>
    public int ReleaseYear         { get; init; }

    /// <summary>
    /// Formatted release year for display; returns "—" when year is 0 (missing) or an
    /// implausible sentinel value (≥ 3000, e.g. 9998/9999 used by some sources).
    /// </summary>
    public string ReleaseYearDisplay => (ReleaseYear > 0 && ReleaseYear < 3000) ? ReleaseYear.ToString() : "—";

    /// <summary>Label of the first source integration (e.g. "RetroAchievements", "Steam").</summary>
    public string IntegrationLabel { get; init; } = string.Empty;

    /// <summary>
    /// Short abbreviated display label for the primary source integration,
    /// shown as a badge chip on the game card (e.g. "STEAM", "XBOX", "GDRIVE").
    /// Empty string when no source is known.
    /// </summary>
    public string SourceBadge { get; init; } = string.Empty;

    /// <summary>
    /// Natural aspect ratio of the cover image (width ÷ height).
    /// Used by <see cref="Controls.JustifiedPanel"/> to size card tiles.
    /// Defaults to 2/3 (portrait game cover) when cover dimensions are unknown.
    /// </summary>
    public double CoverAspectRatio { get; init; } = 2.0 / 3.0;

    /// <summary>
    /// Full source-game info for each source attached to this canonical game.
    /// Carries PluginId + ExternalId for client-side install detection,
    /// and SourceGameId for bulk hard-delete operations.
    /// </summary>
    public IReadOnlyList<SourceGameInfo> Sources { get; init; } = [];

    /// <summary>
    /// Convenience accessor: all source-game IDs — derived from <see cref="Sources"/>.
    /// Used by bulk hard-delete to identify each source record on the server.
    /// </summary>
    public IReadOnlyList<string> SourceGameIds =>
        Sources.Select(s => s.SourceGameId).ToList();

    // ---------------------------------------------------------------------------
    // Observable (mutable) state
    // ---------------------------------------------------------------------------

    private Action? _onSelectionChanged;

    /// <summary>
    /// Whether this card is checked in bulk-select mode.
    /// Two-way bound to the card's checkbox overlay.
    /// Fires <see cref="_onSelectionChanged"/> so the parent ViewModel can
    /// refresh its bulk-count derived properties immediately.
    /// </summary>
    [ObservableProperty]
    private bool _isSelected;

    partial void OnIsSelectedChanged(bool value) => _onSelectionChanged?.Invoke();

    /// <summary>
    /// Install detection result for this game on this machine.
    /// Null until <see cref="Services.Install.InstallDetectionService"/> has run.
    /// Must be set on the UI thread (or via Dispatcher.UIThread.InvokeAsync).
    /// </summary>
    [ObservableProperty]
    private InstallStatus? _installStatus;

    // ---------------------------------------------------------------------------
    // Platform slug → display name / badge color (delegated to PlatformHelper)
    // ---------------------------------------------------------------------------

    private static string FormatPlatform(string? slug) =>
        PlatformHelper.FormatPlatform(slug);

    private static string GetPlatformBadgeColor(string? slug) =>
        PlatformHelper.GetBadgeColor(slug);

    /// <summary>
    /// Converts an integration label into a short uppercase badge string
    /// suitable for display on a game card (max ~8 characters).
    /// </summary>
    private static string FormatSourceBadge(string label)
    {
        if (string.IsNullOrWhiteSpace(label)) return string.Empty;
        var upper = label.Trim().ToUpperInvariant();
        return upper switch
        {
            _ when upper.StartsWith("STEAM",           StringComparison.Ordinal) => "STEAM",
            _ when upper.StartsWith("XBOX",            StringComparison.Ordinal) => "XBOX",
            _ when upper.StartsWith("GOOGLE DRIVE",    StringComparison.Ordinal) => "GDRIVE",
            _ when upper.StartsWith("GDRIVE",          StringComparison.Ordinal) => "GDRIVE",
            _ when upper.StartsWith("NETWORK SHARE",   StringComparison.Ordinal) => "SMB",
            _ when upper.StartsWith("SMB",             StringComparison.Ordinal) => "SMB",
            _ when upper.StartsWith("RETROACHIEVEMENTS", StringComparison.Ordinal) => "RETRO",
            _ when upper.StartsWith("ITCH",            StringComparison.Ordinal) => "ITCH.IO",
            _ when upper.StartsWith("EPIC GAMES",      StringComparison.Ordinal) => "EPIC",
            _ when upper.StartsWith("EPIC",            StringComparison.Ordinal) => "EPIC",
            _ when upper.StartsWith("GOG",             StringComparison.Ordinal) => "GOG",
            _ when upper.StartsWith("PLAYSTATION",     StringComparison.Ordinal) => "PSN",
            _ when upper.Length <= 8                                              => upper,
            _                                                                     => upper[..8],
        };
    }

    // ---------------------------------------------------------------------------
    // Constructors
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Parameterless constructor — for use in tests and AXAML design-time data.
    /// </summary>
    public GameCardModel() { }

    /// <summary>
    /// Production constructor: maps a <see cref="GameDetail"/> API response to display properties,
    /// resolving the cover URL through <paramref name="api"/> when available.
    /// The optional <paramref name="onSelectionChanged"/> callback is fired whenever
    /// <see cref="IsSelected"/> changes so the parent ViewModel can refresh bulk counters.
    /// The optional <paramref name="mediaCache"/> resolves remote URLs to local cached paths.
    /// </summary>
    public GameCardModel(GameDetail g, MgaApiService? api, Action? onSelectionChanged = null, MediaCacheService? mediaCache = null)
    {
        _onSelectionChanged = onSelectionChanged;

        // Prefer an explicit cover override, then fall back to the first cover media asset.
        var coverMedia = g.CoverOverride ?? g.Media.FirstOrDefault(m => m.Type == "cover");
        var rawCoverUrl = coverMedia is not null && api is not null
                          ? api.GetMediaUrl(coverMedia.Url)
                          : null;

        // Find explicit hover-override and background media.
        var hoverMedia  = g.Media.FirstOrDefault(m => m.Type == "hover");
        var bgMedia     = g.Media.FirstOrDefault(m => m.Type == "background");

        // Preview prefers hover > background > screenshot/header.
        var previewMedia = hoverMedia
                           ?? bgMedia
                           ?? g.Media.FirstOrDefault(m => m.Type == "screenshot" || m.Type == "header");
        var rawPreviewUrl = previewMedia is not null && api is not null
                            ? api.GetMediaUrl(previewMedia.Url)
                            : null;

        // Resolve hover and background URLs.
        var rawHoverUrl = hoverMedia is not null && api is not null
                          ? api.GetMediaUrl(hoverMedia.Url)
                          : null;
        var rawBgUrl    = bgMedia is not null && api is not null
                          ? api.GetMediaUrl(bgMedia.Url)
                          : null;

        // Cover aspect ratio from actual media dimensions (falls back to 2:3 portrait).
        var coverW = coverMedia?.Width  ?? 0;
        var coverH = coverMedia?.Height ?? 0;
        CoverAspectRatio = (coverW > 0 && coverH > 0)
                           ? (double)coverW / coverH
                           : 2.0 / 3.0;

        Id                 = g.Id;
        Title              = g.Title;
        Platform           = FormatPlatform(g.Platform);
        PlatformBadgeColor = GetPlatformBadgeColor(g.Platform);
        CoverUrl           = rawCoverUrl is not null && mediaCache is not null
                             ? mediaCache.GetLocalOrRemoteUrl(rawCoverUrl)
                             : rawCoverUrl;
        PreviewUrl         = rawPreviewUrl is not null && mediaCache is not null
                             ? mediaCache.GetLocalOrRemoteUrl(rawPreviewUrl)
                             : rawPreviewUrl;
        HoverUrl           = rawHoverUrl is not null && mediaCache is not null
                             ? mediaCache.GetLocalOrRemoteUrl(rawHoverUrl)
                             : rawHoverUrl;
        BackgroundUrl      = rawBgUrl is not null && mediaCache is not null
                             ? mediaCache.GetLocalOrRemoteUrl(rawBgUrl)
                             : rawBgUrl;
        Favorite           = g.Favorite;
        CanPlay            = g.Kind == "game";
        Kind               = g.Kind;
        Developer          = g.Developer ?? string.Empty;
        Publisher          = g.Publisher ?? string.Empty;
        Genres             = g.Genres;
        var firstSg        = g.SourceGames.FirstOrDefault();
        IntegrationLabel   = firstSg?.IntegrationLabel ?? firstSg?.IntegrationId ?? string.Empty;
        SourceBadge        = FormatSourceBadge(IntegrationLabel);

        // Build SourceGameInfo list for client-side install detection.
        Sources = g.SourceGames.Select(sg => new SourceGameInfo
        {
            SourceGameId = sg.Id,
            PluginId     = sg.PluginId,
            ExternalId   = sg.ExternalId,
            RootPath     = sg.RootPath,
            Label        = sg.IntegrationLabel,
        }).ToList();

        // Parse "YYYY-MM-DD" or "YYYY" → year int.
        if (!string.IsNullOrEmpty(g.ReleaseDate) &&
            int.TryParse(g.ReleaseDate.AsSpan(0, Math.Min(4, g.ReleaseDate.Length)), out var yr))
            ReleaseYear = yr;
    }
}
