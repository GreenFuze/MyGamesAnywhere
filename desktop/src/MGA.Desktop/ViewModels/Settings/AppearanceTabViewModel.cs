using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Appearance tab — Midnight / Daylight theme switcher and other visual preferences.
/// </summary>
public sealed partial class AppearanceTabViewModel : ViewModelBase
{
    private readonly ThemeService    _theme;
    private readonly AppConfigService _config;
    private readonly ToastService    _toast;

    [ObservableProperty]
    private string _selectedTheme;

    public IReadOnlyList<string> AvailableThemes { get; } =
        new[] { "midnight", "daylight" };

    public AppearanceTabViewModel(ThemeService theme, AppConfigService config, ToastService toast)
    {
        _theme          = theme;
        _config         = config;
        _toast          = toast;
        _selectedTheme  = theme.Current;

        // Keep the selected theme in sync when it changes externally.
        Disposables.Add(theme.Changed.Subscribe(id => SelectedTheme = id));
    }

    partial void OnSelectedThemeChanged(string value)
    {
        // Apply immediately when the user picks a different theme.
        if (value != _theme.Current)
            _theme.SetTheme(value);
    }
}
