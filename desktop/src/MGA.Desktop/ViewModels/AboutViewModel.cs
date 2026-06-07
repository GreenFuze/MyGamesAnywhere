using System.Collections.ObjectModel;
using System.Reflection;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

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
