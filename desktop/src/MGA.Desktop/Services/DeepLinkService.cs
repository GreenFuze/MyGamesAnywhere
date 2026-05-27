using System.IO.Pipes;
using System.Text;
using Avalonia.Threading;

namespace MGA.Desktop.Services;

/// <summary>
/// Handles <c>mga://</c> URI deep links for the desktop client.
///
/// On first-instance launch: starts a named-pipe server to receive URIs
/// forwarded by subsequent instances.
///
/// On duplicate-instance launch (enforced by caller via mutex): sends the
/// startup URI to the running instance via the same named pipe, then exits.
/// </summary>
public sealed class DeepLinkService : IDisposable
{
    // -----------------------------------------------------------------------
    // Constants
    // -----------------------------------------------------------------------

    public const  string MutexName     = "MGA.Desktop.SingleInstance";
    private const string PipeName      = "MGA.Desktop.DeepLink";
    private const int    PipeTimeoutMs = 3_000;

    // -----------------------------------------------------------------------
    // Fields
    // -----------------------------------------------------------------------

    private readonly NavigationService      _nav;
    private readonly ServerConnectionService _server;
    private readonly ToastService           _toast;
    private readonly AppConfigService       _config;
    private readonly RecentPlayedService?   _recentPlayed;

    private CancellationTokenSource? _cts;
    private Task?                    _pipeServerTask;

    // -----------------------------------------------------------------------
    // Constructor / Dispose
    // -----------------------------------------------------------------------

    public DeepLinkService(
        NavigationService       nav,
        ServerConnectionService server,
        ToastService            toast,
        AppConfigService        config,
        RecentPlayedService?    recentPlayed = null)
    {
        _nav          = nav;
        _server       = server;
        _toast        = toast;
        _config       = config;
        _recentPlayed = recentPlayed;
    }

    public void Dispose()
    {
        _cts?.Cancel();
        _cts?.Dispose();
        _cts = null;
    }

    // -----------------------------------------------------------------------
    // Public API — called by App
    // -----------------------------------------------------------------------

    /// <summary>
    /// Starts the background named-pipe server that receives forwarded URIs.
    /// Call once per session after the main window is shown.
    /// </summary>
    public void StartServer()
    {
        _cts = new CancellationTokenSource();
        _pipeServerTask = Task.Run(() => RunPipeServerAsync(_cts.Token));
    }

    /// <summary>
    /// Processes a startup URI (e.g. <c>mga://game/abc123</c>) immediately on
    /// first launch. If the URI is empty or not an <c>mga://</c> link, does nothing.
    /// </summary>
    public void HandleStartupUri(string? uri)
    {
        if (!string.IsNullOrWhiteSpace(uri))
            HandleUri(uri);
    }

    // -----------------------------------------------------------------------
    // Static helper — called from Program.Main before App starts
    // -----------------------------------------------------------------------

    /// <summary>
    /// Tries to forward <paramref name="uri"/> to the already-running instance
    /// via named pipe. Returns <c>true</c> if the URI was delivered successfully
    /// (caller should exit). Returns <c>false</c> if no server is listening.
    /// </summary>
    public static bool TryForwardToRunningInstance(string uri)
    {
        try
        {
            using var client = new NamedPipeClientStream(".", PipeName, PipeDirection.Out);
            client.Connect(PipeTimeoutMs);
            var bytes = Encoding.UTF8.GetBytes(uri);
            client.Write(bytes, 0, bytes.Length);
            return true;
        }
        catch
        {
            return false;
        }
    }

    // -----------------------------------------------------------------------
    // Private — pipe server loop
    // -----------------------------------------------------------------------

    private async Task RunPipeServerAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var server = new NamedPipeServerStream(
                    PipeName, PipeDirection.In, maxNumberOfServerInstances: 1,
                    PipeTransmissionMode.Byte, PipeOptions.Asynchronous);

                await server.WaitForConnectionAsync(ct).ConfigureAwait(false);

                // Read the URI (max 4 KB).
                var buf = new byte[4096];
                int read = await server.ReadAsync(buf, 0, buf.Length, ct).ConfigureAwait(false);
                if (read > 0)
                {
                    var uri = Encoding.UTF8.GetString(buf, 0, read);
                    Dispatcher.UIThread.Post(() => HandleUri(uri));
                }
            }
            catch (OperationCanceledException)
            {
                break;
            }
            catch
            {
                // Restart server on transient errors.
                await Task.Delay(500, ct).ConfigureAwait(false);
            }
        }
    }

    // -----------------------------------------------------------------------
    // Private — URI dispatch
    // -----------------------------------------------------------------------

    /// <summary>Parses and dispatches a <c>mga://</c> URI on the UI thread.</summary>
    private void HandleUri(string uri)
    {
        if (!Uri.TryCreate(uri.Trim(), UriKind.Absolute, out var parsed))
            return;

        if (!parsed.Scheme.Equals("mga", StringComparison.OrdinalIgnoreCase))
            return;

        // mga://game/{id}
        if (parsed.Host.Equals("game", StringComparison.OrdinalIgnoreCase))
        {
            var gameId = parsed.AbsolutePath.TrimStart('/');
            if (!string.IsNullOrEmpty(gameId))
            {
                _nav.NavigateTo(new ViewModels.GameDetailViewModel(
                    gameId, _server, _nav, _toast, _config, recentPlayed: _recentPlayed));
            }
        }
    }
}
