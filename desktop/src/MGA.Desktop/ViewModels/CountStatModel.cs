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
    public string Label { get; init; } = string.Empty;

    /// <summary>The count for this label.</summary>
    public int Count { get; init; }

    /// <summary>
    /// The maximum count in the current dataset — used by the Stats bar chart
    /// to proportion the ProgressBar's Maximum so the widest bar fills 100%.
    /// </summary>
    public int MaxCount { get; init; }
}
