using MGA.Api;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Flat display model for a game card tile.
/// Constructed from a <see cref="GameDetail"/> API model so Views only bind
/// to simple string/bool properties — no API models leak into AXAML.
/// </summary>
public sealed class GameCardModel
{
    public string Id               { get; init; } = string.Empty;
    public string Title            { get; init; } = string.Empty;
    public string Platform         { get; init; } = string.Empty;

    /// <summary>Absolute cover-image URL resolved via the API service, or null.</summary>
    public string? CoverUrl        { get; init; }

    public bool   Favorite         { get; init; }
    public bool   CanPlay          { get; init; }
    public string Kind             { get; init; } = string.Empty;
    public string Developer        { get; init; } = string.Empty;
    public string Publisher        { get; init; } = string.Empty;
    public List<string> Genres     { get; init; } = [];

    /// <summary>Release year parsed from the API's release_date string; 0 when unknown.</summary>
    public int ReleaseYear         { get; init; }

    /// <summary>Label of the first source integration (e.g. "RetroAchievements", "Steam").</summary>
    public string IntegrationLabel { get; init; } = string.Empty;

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
    /// </summary>
    public GameCardModel(GameDetail g, MgaApiService? api)
    {
        // Prefer an explicit cover override, then fall back to the first cover media asset.
        var coverMedia = g.CoverOverride
            ?? g.Media.FirstOrDefault(m => m.Type == "cover");

        Id               = g.Id;
        Title            = g.Title;
        Platform         = g.Platform;
        CoverUrl         = coverMedia is not null && api is not null
                           ? api.GetMediaUrl(coverMedia.Url)
                           : null;
        Favorite         = g.Favorite;
        CanPlay          = g.Kind == "game";
        Kind             = g.Kind;
        Developer        = g.Developer ?? string.Empty;
        Publisher        = g.Publisher ?? string.Empty;
        Genres           = g.Genres;
        IntegrationLabel = g.SourceGames.FirstOrDefault()?.IntegrationLabel
                           ?? g.SourceGames.FirstOrDefault()?.IntegrationId
                           ?? string.Empty;

        // Parse "YYYY-MM-DD" or "YYYY" → year int.
        if (!string.IsNullOrEmpty(g.ReleaseDate) &&
            int.TryParse(g.ReleaseDate.AsSpan(0, Math.Min(4, g.ReleaseDate.Length)), out var yr))
            ReleaseYear = yr;
    }
}
