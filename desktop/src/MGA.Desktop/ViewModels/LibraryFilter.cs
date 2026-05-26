namespace MGA.Desktop.ViewModels;

/// <summary>
/// All active filter and sort parameters for the Library page.
/// Passed as a single value object to <see cref="LibraryFilter.Apply"/>.
/// </summary>
internal readonly record struct FilterCriteria
{
    public string             SearchText     { get; init; }
    public IReadOnlySet<string> Platforms    { get; init; }
    public IReadOnlySet<string> Genres       { get; init; }
    public string             Developer      { get; init; }
    public string             Publisher      { get; init; }
    public string             Integration    { get; init; }
    public int?               YearFrom       { get; init; }
    public int?               YearTo         { get; init; }
    public bool               FavoritesOnly  { get; init; }
    public int                SortIndex      { get; init; }
}

/// <summary>
/// Pure static helper that applies a <see cref="FilterCriteria"/> to a
/// sequence of <see cref="GameCardModel"/> items and returns the filtered,
/// sorted result.  Extracted from <see cref="LibraryViewModel"/> for testability.
/// </summary>
internal static class LibraryFilter
{
    internal static IEnumerable<GameCardModel> Apply(
        IEnumerable<GameCardModel> source,
        FilterCriteria             criteria)
    {
        var query = source;

        // Free-text: matches title, platform, developer (case-insensitive).
        var text = criteria.SearchText.Trim();
        if (!string.IsNullOrEmpty(text))
            query = query.Where(g =>
                g.Title.Contains(text,      StringComparison.OrdinalIgnoreCase) ||
                g.Platform.Contains(text,   StringComparison.OrdinalIgnoreCase) ||
                g.Developer.Contains(text,  StringComparison.OrdinalIgnoreCase));

        // Platform multi-select — empty set means "All Platforms".
        if (criteria.Platforms.Count > 0)
            query = query.Where(g => criteria.Platforms.Contains(g.Platform));

        // Genre multi-select — empty set means "All Genres".
        if (criteria.Genres.Count > 0)
            query = query.Where(g =>
                g.Genres.Any(genre => criteria.Genres.Contains(genre)));

        // Developer single-select.
        if (!string.IsNullOrEmpty(criteria.Developer))
            query = query.Where(g =>
                g.Developer.Equals(criteria.Developer, StringComparison.OrdinalIgnoreCase));

        // Publisher single-select.
        if (!string.IsNullOrEmpty(criteria.Publisher))
            query = query.Where(g =>
                g.Publisher.Equals(criteria.Publisher, StringComparison.OrdinalIgnoreCase));

        // Integration single-select (matched against IntegrationLabel).
        if (!string.IsNullOrEmpty(criteria.Integration))
            query = query.Where(g =>
                g.IntegrationLabel.Equals(criteria.Integration, StringComparison.OrdinalIgnoreCase));

        // Year range — only applied when a year is entered.
        if (criteria.YearFrom.HasValue)
            query = query.Where(g => g.ReleaseYear >= criteria.YearFrom.Value);

        if (criteria.YearTo.HasValue)
            query = query.Where(g => g.ReleaseYear > 0 && g.ReleaseYear <= criteria.YearTo.Value);

        // Favorites filter.
        if (criteria.FavoritesOnly)
            query = query.Where(g => g.Favorite);

        // Sort.
        query = criteria.SortIndex switch
        {
            1 => query.OrderByDescending(g => g.Title),
            2 => query.OrderBy(g => g.Platform).ThenBy(g => g.Title),
            3 => query.OrderBy(g => g.Developer).ThenBy(g => g.Title),
            4 => query.OrderByDescending(g => g.ReleaseYear).ThenBy(g => g.Title),
            5 => query.OrderBy(g => g.ReleaseYear == 0 ? int.MaxValue : g.ReleaseYear).ThenBy(g => g.Title),
            _ => query.OrderBy(g => g.Title),
        };

        return query;
    }
}
