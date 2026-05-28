using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;
using MGA.Desktop.ViewModels.Settings;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Settings page — tabbed host for all settings sub-pages.
/// Each tab has its own ViewModel; all are constructed eagerly so tabs are ready immediately.
/// </summary>
public sealed partial class SettingsViewModel : ViewModelBase
{
    [ObservableProperty]
    private int _selectedTabIndex;

    // One ViewModel per tab — constructed eagerly so tabs are ready immediately.
    public IntegrationsTabViewModel    Integrations    { get; }
    public UpdateTabViewModel          Update          { get; }
    public ProfilesTabViewModel        Profiles        { get; }
    public PluginsTabViewModel         Plugins         { get; }
    public CacheTabViewModel           Cache           { get; }
    public DuplicatesTabViewModel      Duplicates      { get; }
    public AppearanceTabViewModel      Appearance      { get; }
    public UndetectedGamesTabViewModel UndetectedGames { get; }
    public EmulatorsTabViewModel       Emulators       { get; }

    public SettingsViewModel(
        ServerConnectionService server,
        ThemeService            theme,
        AppConfigService        config,
        ToastService            toast)
    {
        Integrations    = new IntegrationsTabViewModel(server, toast);
        Update          = new UpdateTabViewModel(server, toast);
        Profiles        = new ProfilesTabViewModel(server, toast);
        Plugins         = new PluginsTabViewModel(server, toast);
        Cache           = new CacheTabViewModel(server, toast);
        Duplicates      = new DuplicatesTabViewModel(server, toast);
        Appearance      = new AppearanceTabViewModel(theme, config, server, toast);
        UndetectedGames = new UndetectedGamesTabViewModel(server, toast);
        Emulators       = new EmulatorsTabViewModel(config, toast);
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
        Emulators.Dispose();
        base.Dispose();
    }
}
