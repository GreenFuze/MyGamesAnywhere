namespace MGA.Desktop.ViewModels;

/// <summary>
/// Shared utility for formatting unlock/completion percentages.
/// Used by StatsViewModel and AchievementsViewModel.
/// </summary>
internal static class PercentFormatter
{
    /// <summary>
    /// Returns a rounded integer percentage string such as "42%".
    /// Returns "0%" when <paramref name="total"/> is zero.
    /// </summary>
    public static string Format(int unlocked, int total)
        => total > 0
            ? $"{(int)Math.Round(unlocked * 100.0 / total)}%"
            : "0%";
}
