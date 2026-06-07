using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Appearance tab — theme switcher only.
///
/// Server-connection management was moved to <see cref="ConnectionTabViewModel"/>.
/// </summary>
public sealed partial class AppearanceTabViewModel : ViewModelBase
{
    private readonly ThemeService _theme;

    // ---------------------------------------------------------------------------
    // Theme
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _selectedTheme;

    public IReadOnlyList<string> AvailableThemes { get; } =
        new[] { "midnight", "daylight" };

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public AppearanceTabViewModel(ThemeService theme)
    {
        _theme         = theme;
        _selectedTheme = theme.Current;

        // Sync theme selector with external changes.
        Disposables.Add(theme.Changed.Subscribe(id => SelectedTheme = id));
    }

    // ---------------------------------------------------------------------------
    // Theme change hook
    // ---------------------------------------------------------------------------

    partial void OnSelectedThemeChanged(string value)
    {
        if (value != _theme.Current)
            _theme.SetTheme(value);
    }
}
