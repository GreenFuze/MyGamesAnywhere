namespace MGA.Desktop.ViewModels;

/// <summary>
/// Flat display model for a game card tile.
/// Created from a GameDetail in the ViewModel layer so Views only bind
/// to simple string/bool properties — no API models leak into AXAML.
/// </summary>
public sealed class GameCardModel
{
    public string Id { get; init; } = string.Empty;
    public string Title { get; init; } = string.Empty;
    public string Platform { get; init; } = string.Empty;

    /// <summary>Absolute cover-image URL, or null if the game has no cover.</summary>
    public string? CoverUrl { get; init; }

    public bool Favorite { get; init; }
    public bool CanPlay { get; init; }
}
