using System.Collections.ObjectModel;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// One horizontal shelf row in the Library Shelf view.
/// Each row has a section title (e.g. "Favorites" or a platform name)
/// and an ordered list of game cards to display in a horizontal scroll strip.
/// </summary>
public sealed class ShelfRowViewModel
{
    /// <summary>Display heading for this shelf row (e.g. "Favorites", "PC", "Nintendo DS").</summary>
    public string Title { get; }

    /// <summary>Games shown in this row, title-sorted.</summary>
    public ObservableCollection<GameCardModel> Games { get; }

    public ShelfRowViewModel(string title, IEnumerable<GameCardModel> games)
    {
        Title = title;
        Games = new ObservableCollection<GameCardModel>(games);
    }
}
