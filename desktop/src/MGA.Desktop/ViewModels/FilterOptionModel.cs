using CommunityToolkit.Mvvm.ComponentModel;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// A single named option in a multi-select filter list (platform, genre, etc.).
/// When <see cref="IsSelected"/> changes it fires the <paramref name="onChange"/>
/// callback so the parent VM can rebuild the filtered game list immediately —
/// no explicit "Apply" button needed.
/// </summary>
public sealed partial class FilterOptionModel : ObservableObject
{
    private readonly Action _onChange;

    /// <summary>The display name and filter value (e.g. "NES", "Action").</summary>
    public string Name { get; }

    /// <summary>Whether this option is currently checked in the filter popup.</summary>
    [ObservableProperty]
    private bool _isSelected;

    public FilterOptionModel(string name, Action onChange)
    {
        Name     = name;
        _onChange = onChange;
    }

    partial void OnIsSelectedChanged(bool value) => _onChange();
}
