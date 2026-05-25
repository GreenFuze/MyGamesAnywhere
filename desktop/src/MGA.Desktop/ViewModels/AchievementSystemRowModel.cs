namespace MGA.Desktop.ViewModels;

/// <summary>
/// Display model for a single achievement-source row.
/// Used in both AchievementsViewModel (dashboard systems list)
/// and StatsViewModel (gamer tab systems list).
///
/// Plain class — not a ViewModel — no change notifications needed.
/// </summary>
public sealed class AchievementSystemRowModel
{
    /// <summary>Source identifier, e.g. "RetroAchievements", "Steam".</summary>
    public string Source { get; init; } = string.Empty;

    /// <summary>Total achievement count across all games for this source.</summary>
    public int Total { get; init; }

    /// <summary>Number of unlocked achievements for this source.</summary>
    public int Unlocked { get; init; }

    /// <summary>Human-readable unlock percentage, e.g. "42%".</summary>
    public string PercentText { get; init; } = string.Empty;

    /// <summary>Total possible points for this source (0 if the source has no points).</summary>
    public int TotalPoints { get; init; }

    /// <summary>Points already earned for this source.</summary>
    public int EarnedPoints { get; init; }
}
