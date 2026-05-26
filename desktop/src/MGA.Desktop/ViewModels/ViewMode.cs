namespace MGA.Desktop.ViewModels;

/// <summary>
/// Active display mode for the Library page.
/// Cycles Grid → List → Timeline → Shelf on each ToggleViewMode call.
/// </summary>
public enum ViewMode
{
    /// <summary>Virtualized grid of cover-art cards.</summary>
    Grid     = 0,

    /// <summary>Tabular list rows (title, platform, developer, year).</summary>
    List     = 1,

    /// <summary>Games grouped by release year, newest group first.</summary>
    Timeline = 2,

    /// <summary>Horizontal scroll rows, one per logical section (Favorites, then per-platform).</summary>
    Shelf    = 3,
}
