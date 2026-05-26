using MGA.Api;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Display model for a single label/count breakdown row
/// (platforms, genres, decades, etc.).
///
/// Plain class — not a ViewModel — no change notifications needed.
/// </summary>
public sealed class CountStatModel
{
    /// <summary>Human-readable label, e.g. "PC", "Action".</summary>
    public string Label    { get; init; } = string.Empty;

    /// <summary>The count for this label.</summary>
    public int    Count    { get; init; }

    /// <summary>
    /// The maximum count in the current dataset — used by the Stats bar chart
    /// to proportion the ProgressBar's Maximum so the widest bar fills 100%.
    /// </summary>
    public int    MaxCount { get; init; }

    // ---------------------------------------------------------------------------
    // Constructors
    // ---------------------------------------------------------------------------

    /// <summary>Parameterless constructor — for tests and design-time data.</summary>
    public CountStatModel() { }

    /// <summary>
    /// Production constructor: maps a <see cref="CountStat"/> API record to display properties.
    /// </summary>
    /// <param name="stat">The API breakdown row.</param>
    /// <param name="maxCount">
    /// The largest count in the same list — used to proportion the bar chart widths.
    /// </param>
    public CountStatModel(CountStat stat, int maxCount)
    {
        Label    = stat.Label;
        Count    = stat.Count;
        MaxCount = maxCount;
    }
}
