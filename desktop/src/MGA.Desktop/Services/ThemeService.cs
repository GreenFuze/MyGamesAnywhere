using System.Reactive.Subjects;
using Avalonia;
using Avalonia.Markup.Xaml.Styling;

namespace MGA.Desktop.Services;

/// <summary>
/// Manages the active UI theme at runtime.
///
/// Swaps Avalonia ResourceDictionaries so DynamicResource references repaint the whole tree
/// immediately. Theme is persisted to AppConfigService.
///
/// Only two themes are supported in v1: "midnight" (dark) and "daylight" (light).
/// </summary>
public sealed class ThemeService : IDisposable
{
    private static readonly Uri s_baseUri = new("avares://MGA.Desktop/");

    private static readonly Dictionary<string, Uri> s_themeUris = new(StringComparer.OrdinalIgnoreCase)
    {
        ["midnight"] = new Uri("avares://MGA.Desktop/Themes/Midnight.axaml"),
        ["daylight"] = new Uri("avares://MGA.Desktop/Themes/Daylight.axaml"),
    };

    private readonly AppConfigService _appConfig;
    private readonly Application _app;
    private readonly BehaviorSubject<string> _current;
    private ResourceInclude? _activeDict;

    public ThemeService(AppConfigService appConfig, Application app)
    {
        _appConfig = appConfig;
        _app = app;

        var initial = Normalize(appConfig.Config.ThemeId);
        _current = new BehaviorSubject<string>(initial);

        // Apply theme immediately on construction (RAII).
        ApplyTheme(initial);
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>The ID of the currently active theme ("midnight" or "daylight").</summary>
    public string Current => _current.Value;

    /// <summary>Observable that emits whenever the theme changes.</summary>
    public IObservable<string> Changed => _current;

    public void SetTheme(string themeId)
    {
        var id = Normalize(themeId);
        if (id == _current.Value) return;

        ApplyTheme(id);
        _current.OnNext(id);

        // Persist choice.
        _appConfig.Update(cfg => cfg.ThemeId = id);
    }

    // ---------------------------------------------------------------------------
    // Private
    // ---------------------------------------------------------------------------

    private void ApplyTheme(string themeId)
    {
        if (!s_themeUris.TryGetValue(themeId, out var uri))
            uri = s_themeUris["midnight"]; // fallback

        var newDict = new ResourceInclude(s_baseUri) { Source = uri };

        // Replace the existing theme dictionary (index 0 by convention) or add it.
        var merged = _app.Resources.MergedDictionaries;
        if (_activeDict is not null && merged.Contains(_activeDict))
            merged[merged.IndexOf(_activeDict)] = newDict;
        else
            merged.Insert(0, newDict);

        _activeDict = newDict;
    }

    private static string Normalize(string id) =>
        s_themeUris.ContainsKey(id) ? id.ToLowerInvariant() : "midnight";

    public void Dispose() => _current.Dispose();
}
