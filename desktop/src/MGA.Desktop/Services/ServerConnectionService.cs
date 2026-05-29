using MGA.Api;
using System.Reactive.Linq;
using System.Reactive.Subjects;

namespace MGA.Desktop.Services;

/// <summary>
/// RAII service that owns the HttpClient and SSE connection for the currently active MGA server.
///
/// Constructor reads the active server from AppConfigService and connects immediately.
/// SwitchServer disposes the old connection and creates a new one atomically.
/// Dispose cleans up the HttpClient, SseClient, and SseEventBus.
/// </summary>
public sealed class ServerConnectionService : IDisposable
{
    private readonly AppConfigService _appConfig;

    private HttpClient _http = null!;
    private SseClient? _sse;
    private MgaApiService _api = null!;
    private SseEventBus _events = null!;

    private readonly BehaviorSubject<string> _urlSubject;

    public ServerConnectionService(AppConfigService appConfig)
    {
        _appConfig = appConfig;

        _urlSubject = new BehaviorSubject<string>(appConfig.Config.ActiveServer);

        // Connect to the last active server immediately if one is configured.
        if (!string.IsNullOrWhiteSpace(appConfig.Config.ActiveServer))
            Connect(appConfig.Config.ActiveServer);
    }

    // ---------------------------------------------------------------------------
    // Public surface
    // ---------------------------------------------------------------------------

    /// <summary>The typed API client for the active server. Null if not yet connected.</summary>
    public MgaApiService? Api => _api;

    /// <summary>The SSE event bus for the active server. Null if not yet connected.</summary>
    public SseEventBus? Events => _events;

    /// <summary>The base URL of the active server, or empty if not connected.</summary>
    public string ActiveUrl => _appConfig.Config.ActiveServer;

    /// <summary>The active gamer profile ID, or empty string when none is selected.</summary>
    public string ActiveProfileId => _appConfig.Config.GamerProfileId;

    /// <summary>Fires the current URL immediately on subscribe, then on every server switch.</summary>
    public IObservable<string> UrlChanged => _urlSubject.AsObservable();

    /// <summary>
    /// Sets the active gamer profile and applies the <c>X-MGA-Profile-ID</c> header to the
    /// live HttpClient so every subsequent API call carries it automatically.
    /// Also persists the profile ID to config.
    /// </summary>
    public void SetActiveProfile(string profileId)
    {
        _appConfig.Update(cfg => cfg.GamerProfileId = profileId);

        // Update the HttpClient default header so all in-flight and future requests include it.
        if (_http is not null)
        {
            _http.DefaultRequestHeaders.Remove("X-MGA-Profile-ID");
            if (!string.IsNullOrWhiteSpace(profileId))
                _http.DefaultRequestHeaders.Add("X-MGA-Profile-ID", profileId);
        }
    }

    /// <summary>
    /// Switches to a different server: disposes the current connection, updates config,
    /// and opens a new HttpClient + SSE stream.
    ///
    /// Profile IDs are server-specific — a profile that exists on server A may not exist
    /// (or may collide with a different account) on server B. The stored GamerProfileId
    /// is therefore cleared on every switch so the caller is forced back through
    /// profile selection before issuing API requests.
    /// </summary>
    public void SwitchServer(ServerProfile profile)
    {
        DisposeConnection();

        _appConfig.Update(cfg =>
        {
            cfg.ActiveServer    = profile.Url;
            cfg.GamerProfileId  = string.Empty;   // invalidated — belongs to the old server

            // Add to server list if not already present.
            if (!cfg.Servers.Any(s => s.Url == profile.Url))
                cfg.Servers.Add(profile);
        });

        Connect(profile.Url);
    }

    /// <summary>
    /// Connects to a server URL (called on first run from OnboardingViewModel).
    /// Also saves the profile to config.
    /// </summary>
    public void ConnectToUrl(string url, string name = "")
    {
        var profile = new ServerProfile
        {
            Name = string.IsNullOrWhiteSpace(name) ? url : name,
            Url = url,
        };
        SwitchServer(profile);
    }

    // ---------------------------------------------------------------------------
    // Private — connection lifecycle
    // ---------------------------------------------------------------------------

    private void Connect(string baseUrl)
    {
        // Create a new HttpClient scoped to this server.
        _http = new HttpClient { BaseAddress = new Uri(baseUrl) };

        // Stamp the profile header so every subsequent request carries it automatically.
        // The server requires X-MGA-Profile-ID on all /api/* routes (except /api/profiles
        // and /api/setup/* which are exempt by design for the profile-selection flow).
        var profileId = _appConfig.Config.GamerProfileId;
        if (!string.IsNullOrWhiteSpace(profileId))
            _http.DefaultRequestHeaders.Add("X-MGA-Profile-ID", profileId);

        // Wrap it in the API service facade.
        _api = new MgaApiService(_http);

        // Start the SSE stream for real-time events.
        _sse = new SseClient(_http, "/api/events");
        _events = new SseEventBus(_sse);

        // Notify all URL subscribers of the new active server.
        _urlSubject.OnNext(baseUrl);
    }

    private void DisposeConnection()
    {
        _events?.Dispose();
        _sse?.DisposeAsync().AsTask().GetAwaiter().GetResult();
        _http?.Dispose();
    }

    // ---------------------------------------------------------------------------
    // IDisposable
    // ---------------------------------------------------------------------------

    public void Dispose()
    {
        DisposeConnection();
        _urlSubject.OnCompleted();
        _urlSubject.Dispose();
    }
}
