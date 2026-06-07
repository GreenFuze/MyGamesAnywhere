using System.Collections.ObjectModel;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// One year-band row in the Library Timeline view.
/// Contains a display label (e.g. "2024" or "Unknown") and the games
/// released that year, already sorted by title.
/// </summary>
public sealed class TimelineYearGroupViewModel
{
    /// <summary>Year as a display string; "Unknown" when year is 0.</summary>
    public string YearLabel { get; }

    /// <summary>Games belonging to this year, title-sorted.</summary>
    public ObservableCollection<GameCardModel> Games { get; }

    public TimelineYearGroupViewModel(int year, IEnumerable<GameCardModel> games)
    {
        // Treat 0 (missing) and implausibly large sentinel values (e.g. 9998, 9999
        // used by some sources for "unknown release year") as "Unknown".
        YearLabel = (year <= 0 || year >= 3000) ? "Unknown" : year.ToString();
        Games     = new ObservableCollection<GameCardModel>(games);
    }
}
