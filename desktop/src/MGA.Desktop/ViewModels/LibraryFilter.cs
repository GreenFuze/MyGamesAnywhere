namespace MGA.Desktop.ViewModels;

/// <summary>
/// Pure static helper that applies text search, platform filter, favorites filter,
/// and sort order to a sequence of GameCardModel items.
///
/// Extracted from LibraryViewModel.RebuildFilteredGames for testability.
/// </summary>
internal static class LibraryFilter
{
    /// <summary>
    /// Applies all active filters and the selected sort order.
    /// </summary>
    /// <param name="source">Input sequence of game cards.</param>
    /// <param name="searchText">Free-text search matched against Title and Platform (OrdinalIgnoreCase).</param>
    /// <param name="selectedPlatform">Exact platform name, or "All Platforms" to skip platform filtering.</param>
    /// <param name="showFavoritesOnly">When true, only favorite games are returned.</param>
    /// <param name="sortIndex">0 = Title A–Z, 1 = Title Z–A, 2 = Platform then Title.</param>
    /// <returns>Filtered and sorted enumerable (not materialised).</returns>
    internal static IEnumerable<GameCardModel> Apply(
        IEnumerable<GameCardModel> source,
        string searchText,
        string selectedPlatform,
        bool showFavoritesOnly,
        int sortIndex)
    {
        var query = source;

        // Text filter — matches title or platform (case-insensitive).
        var text = searchText.Trim();
        if (!string.IsNullOrEmpty(text))
            query = query.Where(g =>
                g.Title.Contains(text, StringComparison.OrdinalIgnoreCase) ||
                g.Platform.Contains(text, StringComparison.OrdinalIgnoreCase));

        // Platform filter — skip when "All Platforms".
        if (selectedPlatform != "All Platforms")
            query = query.Where(g => g.Platform == selectedPlatform);

        // Favorites filter.
        if (showFavoritesOnly)
            query = query.Where(g => g.Favorite);

        // Sort.
        query = sortIndex switch
        {
            1 => query.OrderByDescending(g => g.Title),
            2 => query.OrderBy(g => g.Platform).ThenBy(g => g.Title),
            _ => query.OrderBy(g => g.Title),
        };

        return query;
    }
}
