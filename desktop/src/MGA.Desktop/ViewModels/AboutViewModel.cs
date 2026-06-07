using System.Collections.ObjectModel;
using System.Reflection;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// ThirdPartyCredit — one external service/library entry in the "Powered By" list
// ---------------------------------------------------------------------------

/// <summary>
/// Describes a third-party service, data provider, or runtime that powers MyGamesAnywhere.
/// Exposed as an immutable record-like class for binding in AboutView.axaml.
/// </summary>
public sealed class ThirdPartyCredit
{
    public string  Name        { get; init; } = string.Empty;
    public string  Description { get; init; } = string.Empty;
    public string? WebsiteUrl  { get; init; }
}

/// <summary>
/// About page — desktop client version, server version, open-source licenses.
/// Server info is fetched from GET /api/about on construction.
/// </summary>
public sealed partial class AboutViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;

    // ---------------------------------------------------------------------------
    // Desktop client info (from assembly)
    // ---------------------------------------------------------------------------

    public string ClientVersion { get; }
    public string ClientBuildDate { get; }

    // ---------------------------------------------------------------------------
    // Server info (loaded from API)
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private string _serverVersion = string.Empty;

    [ObservableProperty]
    private string _serverBuildDate = string.Empty;

    [ObservableProperty]
    private string _serverCommit = string.Empty;

    [ObservableProperty]
    private bool _hasServerInfo;

    [ObservableProperty]
    private ObservableCollection<string> _authorCredits = [];

    // ---------------------------------------------------------------------------
    // Third-party credits (static — never changes at runtime)
    // ---------------------------------------------------------------------------

    public IReadOnlyList<ThirdPartyCredit> ThirdPartyCredits { get; } = new ThirdPartyCredit[]
    {
        new() { Name = "Steam",             Description = "Storefront, source, metadata and achievement provider.",            WebsiteUrl = "https://store.steampowered.com/" },
        new() { Name = "Xbox",              Description = "Source integration and platform ecosystem for Xbox library records.", WebsiteUrl = "https://www.xbox.com/" },
        new() { Name = "GOG",               Description = "Storefront and metadata source for GOG-linked games.",               WebsiteUrl = "https://www.gog.com/" },
        new() { Name = "Epic Games",        Description = "Storefront/source integration for Epic-linked games.",               WebsiteUrl = "https://store.epicgames.com/" },
        new() { Name = "IGDB",              Description = "Metadata provider for descriptions, release facts, and game info.",  WebsiteUrl = "https://www.igdb.com/" },
        new() { Name = "RAWG",              Description = "Metadata provider used for game facts when available.",              WebsiteUrl = "https://rawg.io/" },
        new() { Name = "LaunchBox",         Description = "Metadata provider for cover art, descriptions, and game facts.",     WebsiteUrl = "https://www.launchbox-app.com/" },
        new() { Name = "HowLongToBeat",     Description = "Completion-time provider for story and completionist estimates.",    WebsiteUrl = "https://howlongtobeat.com/" },
        new() { Name = "RetroAchievements", Description = "Achievement provider for supported retro platforms.",                WebsiteUrl = "https://retroachievements.org/" },
        new() { Name = "MobyGames",         Description = "External reference site for game pages and release metadata.",       WebsiteUrl = "https://www.mobygames.com/" },
        new() { Name = "PCGamingWiki",      Description = "External reference for platform-specific compatibility details.",    WebsiteUrl = "https://www.pcgamingwiki.com/" },
        new() { Name = "MAME",              Description = "Arcade metadata/catalog source through MAME DAT integration.",      WebsiteUrl = "https://www.mamedev.org/" },
        new() { Name = "EmulatorJS",        Description = "Browser runtime for cartridge and console playback.",               WebsiteUrl = "https://emulatorjs.org/" },
        new() { Name = "js-dos",            Description = "Browser runtime used for DOS playback in the embedded player.",     WebsiteUrl = "https://js-dos.com/" },
        new() { Name = "DOSBox",            Description = "DOS emulation core used behind the js-dos browser runtime.",        WebsiteUrl = "https://www.dosbox.com/" },
        new() { Name = "ScummVM",           Description = "Platform mark for ScummVM-compatible adventure titles.",            WebsiteUrl = "https://www.scummvm.org/" },
        new() { Name = "SMB File Share",    Description = "Network file-share integration for local and NAS libraries.",       WebsiteUrl = null },
        new() { Name = "Google Drive",      Description = "Source and sync integration for cloud-backed libraries.",           WebsiteUrl = "https://drive.google.com/" },
    };

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public AboutViewModel(ServerConnectionService server)
    {
        _server = server;

        // Pull client version from the assembly attribute.
        var asm = Assembly.GetExecutingAssembly();
        var rawVersion  = asm.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion
                          ?? asm.GetName().Version?.ToString()
                          ?? "0.0.0";

        // Trim build metadata (the +<hash> suffix from NuGet semantic versioning).
        var plusIdx = rawVersion.IndexOf('+');
        ClientVersion   = plusIdx > 0 ? rawVersion[..plusIdx] : rawVersion;

        // Format the build date as human-readable (e.g. "June 7, 2026").
        var buildTime   = File.GetLastWriteTimeUtc(asm.Location);
        ClientBuildDate = new DateTimeOffset(buildTime, TimeSpan.Zero)
            .ToString("MMMM d, yyyy", System.Globalization.CultureInfo.InvariantCulture);

        _ = LoadServerInfoAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Opens the MGA repository in the system browser.</summary>
    [RelayCommand]
    private void OpenRepository()
    {
        System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(
            "https://github.com/GreenFuze/MyGamesAnywhere") { UseShellExecute = true });
    }

    /// <summary>Opens an arbitrary URL in the system browser (used by the credits list).</summary>
    [RelayCommand]
    private void OpenUrl(string? url)
    {
        if (string.IsNullOrEmpty(url))
            return;

        System.Diagnostics.Process.Start(
            new System.Diagnostics.ProcessStartInfo(url) { UseShellExecute = true });
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Delegates to the shared <see cref="DateTimeFormatter.FormatDate"/> helper.
    /// Kept as a private wrapper so call-sites inside this class stay concise.
    /// </summary>
    private static string FormatDate(string? raw) => DateTimeFormatter.FormatDate(raw);

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadServerInfoAsync()
    {
        if (_server.Api is null)
            return;

        IsLoading = true;

        try
        {
            var info = await _server.Api.GetAboutInfoAsync().ConfigureAwait(true);

            ServerVersion   = info.Version;
            ServerBuildDate = FormatDate(info.BuildDate);
            ServerCommit    = info.Commit.Length > 8 ? info.Commit[..8] : info.Commit;
            HasServerInfo   = true;

            AuthorCredits = new ObservableCollection<string>(info.AuthorCredits);
        }
        catch
        {
            // Non-critical — About page works without server info.
            HasServerInfo = false;
        }
        finally
        {
            IsLoading = false;
        }
    }
}
