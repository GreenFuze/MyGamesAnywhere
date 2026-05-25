using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Settings page — tabbed host for all settings sub-pages.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed partial class SettingsViewModel : ViewModelBase
{
    [ObservableProperty]
    private int _selectedTabIndex;

    // One ViewModel per tab — constructed eagerly so tabs are ready immediately.
    public IntegrationsTabViewModel  Integrations  { get; }
    public UpdateTabViewModel        Update        { get; }
    public ProfilesTabViewModel      Profiles      { get; }
    public PluginsTabViewModel       Plugins       { get; }
    public CacheTabViewModel         Cache         { get; }
    public DuplicatesTabViewModel    Duplicates    { get; }
    public AppearanceTabViewModel    Appearance    { get; }
    public UndetectedGamesTabViewModel UndetectedGames { get; }

    public SettingsViewModel(
        ServerConnectionService server,
        ThemeService            theme,
        AppConfigService        config,
        ToastService            toast)
    {
        Integrations    = new IntegrationsTabViewModel(server, toast);
        Update          = new UpdateTabViewModel(toast);
        Profiles        = new ProfilesTabViewModel(server, toast);
        Plugins         = new PluginsTabViewModel(server, toast);
        Cache           = new CacheTabViewModel(server, toast);
        Duplicates      = new DuplicatesTabViewModel(server, toast);
        Appearance      = new AppearanceTabViewModel(theme, config, server, toast);
        UndetectedGames = new UndetectedGamesTabViewModel(server, toast);
    }

    public override void Dispose()
    {
        // Dispose each tab ViewModel.
        Integrations.Dispose();
        Update.Dispose();
        Profiles.Dispose();
        Plugins.Dispose();
        Cache.Dispose();
        Duplicates.Dispose();
        Appearance.Dispose();
        UndetectedGames.Dispose();
        base.Dispose();
    }
}
