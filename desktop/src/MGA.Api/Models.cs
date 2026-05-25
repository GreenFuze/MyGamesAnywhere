using System.Text.Json.Serialization;

namespace MGA.Api;

// ---------------------------------------------------------------------------
// Game models
// ---------------------------------------------------------------------------

/// <summary>Minimal game summary for list/grid display.</summary>
public sealed record GameSummary
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("favorite")]
    public bool Favorite { get; init; }
}

/// <summary>Media asset associated with a game.</summary>
public sealed record GameMedia
{
    [JsonPropertyName("asset_id")]
    public int AssetId { get; init; }

    [JsonPropertyName("type")]
    public string Type { get; init; } = string.Empty;

    [JsonPropertyName("url")]
    public string Url { get; init; } = string.Empty;

    [JsonPropertyName("width")]
    public int Width { get; init; }

    [JsonPropertyName("height")]
    public int Height { get; init; }
}

/// <summary>Achievement summary statistics for a game.</summary>
public sealed record AchievementSummary
{
    [JsonPropertyName("source_count")]
    public int SourceCount { get; init; }

    [JsonPropertyName("total_count")]
    public int TotalCount { get; init; }

    [JsonPropertyName("unlocked_count")]
    public int UnlockedCount { get; init; }
}

/// <summary>Full game detail object returned by GET /api/games/{id}.</summary>
public sealed record GameDetail
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("title")]
    public string Title { get; init; } = string.Empty;

    [JsonPropertyName("platform")]
    public string Platform { get; init; } = string.Empty;

    [JsonPropertyName("kind")]
    public string Kind { get; init; } = string.Empty;

    [JsonPropertyName("favorite")]
    public bool Favorite { get; init; }

    [JsonPropertyName("description")]
    public string? Description { get; init; }

    [JsonPropertyName("release_date")]
    public string? ReleaseDate { get; init; }

    [JsonPropertyName("genres")]
    public List<string> Genres { get; init; } = [];

    [JsonPropertyName("developer")]
    public string? Developer { get; init; }

    [JsonPropertyName("publisher")]
    public string? Publisher { get; init; }

    [JsonPropertyName("rating")]
    public double Rating { get; init; }

    [JsonPropertyName("media")]
    public List<GameMedia> Media { get; init; } = [];

    [JsonPropertyName("cover_override")]
    public GameMedia? CoverOverride { get; init; }

    [JsonPropertyName("achievement_summary")]
    public AchievementSummary? AchievementSummary { get; init; }
}

// ---------------------------------------------------------------------------
// Paginated game list
// ---------------------------------------------------------------------------

/// <summary>Paginated response from GET /api/games.</summary>
public sealed record ListGamesResponse
{
    [JsonPropertyName("total")]
    public int Total { get; init; }

    [JsonPropertyName("page")]
    public int Page { get; init; }

    [JsonPropertyName("page_size")]
    public int PageSize { get; init; }

    [JsonPropertyName("games")]
    public List<GameDetail> Games { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Library stats
// ---------------------------------------------------------------------------

/// <summary>High-level library statistics from GET /api/stats.</summary>
public sealed record LibraryStats
{
    [JsonPropertyName("canonical_game_count")]
    public int CanonicalGameCount { get; init; }

    [JsonPropertyName("by_platform")]
    public Dictionary<string, int> ByPlatform { get; init; } = [];
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

/// <summary>Gamer profile from GET /api/profiles.</summary>
public sealed record Profile
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("display_name")]
    public string DisplayName { get; init; } = string.Empty;

    [JsonPropertyName("avatar_key")]
    public string? AvatarKey { get; init; }

    [JsonPropertyName("role")]
    public string Role { get; init; } = string.Empty;

    [JsonPropertyName("created_at")]
    public string? CreatedAt { get; init; }
}
