using MGA.Desktop.ViewModels;
using Xunit;

namespace MGA.Tests.Desktop;

public sealed class LibraryFilterTests
{
    // ---------------------------------------------------------------------------
    // Helpers
    // ---------------------------------------------------------------------------

    private static GameCardModel Card(
        string id,
        string title,
        string platform,
        bool favorite = false)
        => new() { Id = id, Title = title, Platform = platform, Favorite = favorite };

    /// <summary>
    /// Convenience wrapper that translates legacy positional test arguments into
    /// a <see cref="FilterCriteria"/> so individual test cases stay concise.
    /// </summary>
    private static IEnumerable<GameCardModel> Apply(
        IEnumerable<GameCardModel> source,
        string searchText       = "",
        string selectedPlatform = "All Platforms",
        string selectedGenre    = "All Genres",
        bool   showFavs         = false,
        int    sortIndex        = 0)
    {
        // "All Platforms" / "All Genres" → empty set → LibraryFilter treats as "no filter".
        var platforms = selectedPlatform == "All Platforms" || string.IsNullOrEmpty(selectedPlatform)
            ? (IReadOnlySet<string>)new HashSet<string>()
            : new HashSet<string>(StringComparer.OrdinalIgnoreCase) { selectedPlatform };

        var genres = selectedGenre == "All Genres" || string.IsNullOrEmpty(selectedGenre)
            ? (IReadOnlySet<string>)new HashSet<string>()
            : new HashSet<string>(StringComparer.OrdinalIgnoreCase) { selectedGenre };

        var criteria = new FilterCriteria
        {
            SearchText    = searchText,
            Platforms     = platforms,
            Genres        = genres,
            Developer     = string.Empty,
            Publisher     = string.Empty,
            Integration   = string.Empty,
            FavoritesOnly = showFavs,
            SortIndex     = sortIndex,
        };

        return LibraryFilter.Apply(source, criteria);
    }

    // ---------------------------------------------------------------------------
    // Source data
    // ---------------------------------------------------------------------------

    private static readonly GameCardModel[] s_games =
    [
        Card("g1", "Alpha",   "PC",   favorite: false),
        Card("g2", "Bravo",   "PS5",  favorite: true),
        Card("g3", "Charlie", "PC",   favorite: false),
        Card("g4", "Delta",   "Xbox", favorite: true),
        Card("g5", "Echo",    "PS5",  favorite: false),
    ];

    // ---------------------------------------------------------------------------
    // Basic filter tests
    // ---------------------------------------------------------------------------

    [Fact]
    public void Empty_search_returns_all()
    {
        var result = Apply(s_games).ToList();
        // All 5 games, sorted A-Z by default.
        Assert.Equal(5, result.Count);
    }

    [Fact]
    public void Text_search_filters_by_title_case_insensitive()
    {
        var result = Apply(s_games, searchText: "alpha").ToList();
        Assert.Single(result);
        Assert.Equal("Alpha", result[0].Title);
    }

    [Fact]
    public void Text_search_matches_platform()
    {
        // "Xbox" matches only Delta (platform = Xbox).
        var result = Apply(s_games, searchText: "xbox").ToList();
        Assert.Single(result);
        Assert.Equal("Delta", result[0].Title);
    }

    [Fact]
    public void Platform_filter_all_platforms_returns_all()
    {
        var result = Apply(s_games, selectedPlatform: "All Platforms").ToList();
        Assert.Equal(5, result.Count);
    }

    [Fact]
    public void Platform_filter_specific_platform_filters_correctly()
    {
        var result = Apply(s_games, selectedPlatform: "PS5").ToList();
        Assert.Equal(2, result.Count);
        Assert.All(result, g => Assert.Equal("PS5", g.Platform));
    }

    [Fact]
    public void Favorites_only_returns_only_favorites()
    {
        var result = Apply(s_games, showFavs: true).ToList();
        Assert.Equal(2, result.Count);
        Assert.All(result, g => Assert.True(g.Favorite));
    }

    [Fact]
    public void Combined_text_and_platform_and_favorites()
    {
        // Search "b" on PS5 with favorites only → Bravo matches.
        var result = Apply(s_games,
            searchText: "b",
            selectedPlatform: "PS5",
            showFavs: true).ToList();

        Assert.Single(result);
        Assert.Equal("Bravo", result[0].Title);
    }

    // ---------------------------------------------------------------------------
    // Sort tests
    // ---------------------------------------------------------------------------

    [Fact]
    public void Sort_0_orders_title_ascending()
    {
        var result = Apply(s_games, sortIndex: 0).Select(g => g.Title).ToList();
        Assert.Equal(["Alpha", "Bravo", "Charlie", "Delta", "Echo"], result);
    }

    [Fact]
    public void Sort_1_orders_title_descending()
    {
        var result = Apply(s_games, sortIndex: 1).Select(g => g.Title).ToList();
        Assert.Equal(["Echo", "Delta", "Charlie", "Bravo", "Alpha"], result);
    }

    [Fact]
    public void Sort_2_orders_by_platform_then_title()
    {
        var result = Apply(s_games, sortIndex: 2).Select(g => (g.Platform, g.Title)).ToList();

        // Expected: PC(Alpha, Charlie), PS5(Bravo, Echo), Xbox(Delta).
        var expected = new[]
        {
            ("PC",   "Alpha"),
            ("PC",   "Charlie"),
            ("PS5",  "Bravo"),
            ("PS5",  "Echo"),
            ("Xbox", "Delta"),
        };

        Assert.Equal(expected, result);
    }

    // ---------------------------------------------------------------------------
    // Edge case
    // ---------------------------------------------------------------------------

    [Fact]
    public void Empty_source_returns_empty()
    {
        var result = Apply(Array.Empty<GameCardModel>()).ToList();
        Assert.Empty(result);
    }

    [Fact]
    public void Text_search_no_match_returns_empty()
    {
        var result = Apply(s_games, searchText: "zzznomatch").ToList();
        Assert.Empty(result);
    }
}
