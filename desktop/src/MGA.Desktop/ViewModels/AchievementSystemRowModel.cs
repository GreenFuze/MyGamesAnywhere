using MGA.Api;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Display model for a single achievement-source row.
/// Used in both <c>AchievementsViewModel</c> (dashboard systems list)
/// and <c>StatsViewModel</c> (gamer tab systems list).
///
/// Plain class — not a ViewModel — no change notifications needed.
/// </summary>
public sealed class AchievementSystemRowModel
{
    /// <summary>Source identifier, e.g. "RetroAchievements", "Steam".</summary>
    public string Source { get; init; } = string.Empty;

    public int    Total        { get; init; }
    public int    Unlocked     { get; init; }
    public string PercentText  { get; init; } = string.Empty;
    public int    TotalPoints  { get; init; }
    public int    EarnedPoints { get; init; }

    // ---------------------------------------------------------------------------
    // Constructors
    // ---------------------------------------------------------------------------

    /// <summary>Parameterless constructor — for tests and design-time data.</summary>
    public AchievementSystemRowModel() { }

    /// <summary>
    /// Maps an <see cref="AchievementSystemStat"/> (from the achievements dashboard).
    /// Includes points data when available.
    /// </summary>
    public AchievementSystemRowModel(AchievementSystemStat s)
    {
        Source       = s.Source;
        Total        = s.TotalCount;
        Unlocked     = s.UnlockedCount;
        PercentText  = PercentFormatter.Format(s.UnlockedCount, s.TotalCount);
        TotalPoints  = s.TotalPoints;
        EarnedPoints = s.EarnedPoints;
    }

    /// <summary>
    /// Maps an <see cref="AchievementSystem"/> (from the gamer statistics).
    /// Points fields default to 0 as the gamer-stats type does not carry them.
    /// </summary>
    public AchievementSystemRowModel(AchievementSystem s)
    {
        Source      = s.Source;
        Total       = s.TotalCount;
        Unlocked    = s.UnlockedCount;
        PercentText = PercentFormatter.Format(s.UnlockedCount, s.TotalCount);
    }
}
