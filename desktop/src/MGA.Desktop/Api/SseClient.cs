using System.Net.Http.Headers;

namespace MGA.Api;

/// <summary>
/// RAII SSE stream client for GET /api/events.
///
/// Constructor connects immediately and starts a background reader loop.
/// On disconnect, reconnects automatically with exponential backoff (1 → 30 s).
/// Dispose cancels the stream and waits for the reader task to finish.
/// </summary>
public sealed class SseClient : IAsyncDisposable
{
    private readonly HttpClient _http;
    private readonly string _endpoint;
    private readonly CancellationTokenSource _cts = new();
    private readonly Task _readerTask;

    /// <summary>Fired on the thread-pool for each received SSE event.</summary>
    public event Action<string, string>? EventReceived; // (eventType, jsonData)

    public SseClient(HttpClient http, string endpoint)
    {
        _http = http;
        _endpoint = endpoint;

        // Start the background read loop immediately (RAII).
        _readerTask = RunLoop(_cts.Token);
    }

    // ---------------------------------------------------------------------------
    // Background reader loop
    // ---------------------------------------------------------------------------

    private async Task RunLoop(CancellationToken ct)
    {
        var backoff = TimeSpan.FromSeconds(1);

        while (!ct.IsCancellationRequested)
        {
            try
            {
                await ConnectAndStream(ct).ConfigureAwait(false);

                // Clean disconnect — reset backoff.
                backoff = TimeSpan.FromSeconds(1);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                // Shutdown requested — exit cleanly.
                return;
            }
            catch
            {
                // Any other error: connection refused, network drop, server restart, etc.
                // Wait with backoff then reconnect.
                try { await Task.Delay(backoff, ct).ConfigureAwait(false); }
                catch (OperationCanceledException) { return; }

                backoff = TimeSpan.FromSeconds(Math.Min(backoff.TotalSeconds * 2, 30));
            }
        }
    }

    private async Task ConnectAndStream(CancellationToken ct)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, _endpoint);
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("text/event-stream"));

        // ResponseHeadersRead: start reading as soon as headers arrive; don't buffer the body.
        using var response = await _http
            .SendAsync(request, HttpCompletionOption.ResponseHeadersRead, ct)
            .ConfigureAwait(false);

        response.EnsureSuccessStatusCode();

        await using var stream = await response.Content.ReadAsStreamAsync(ct).ConfigureAwait(false);
        using var reader = new StreamReader(stream);

        // Parse SSE wire format: "event: <type>\ndata: <json>\n\n"
        string? pendingEventType = null;

        while (!ct.IsCancellationRequested)
        {
            var line = await reader.ReadLineAsync(ct).ConfigureAwait(false);
            if (line is null)
                break; // server closed the stream

            if (line.StartsWith("event:", StringComparison.Ordinal))
            {
                pendingEventType = line["event:".Length..].Trim();
            }
            else if (line.StartsWith("data:", StringComparison.Ordinal))
            {
                var data = line["data:".Length..].Trim();

                if (pendingEventType is not null)
                    EventReceived?.Invoke(pendingEventType, data);

                pendingEventType = null; // reset after dispatching
            }
            else if (line.Length == 0)
            {
                // Empty line = event boundary; reset pending type if not already dispatched.
                pendingEventType = null;
            }
        }
    }

    // ---------------------------------------------------------------------------
    // IAsyncDisposable — cancel and await the reader task
    // ---------------------------------------------------------------------------

    public async ValueTask DisposeAsync()
    {
        _cts.Cancel();

        try
        {
            await _readerTask.ConfigureAwait(false);
        }
        catch
        {
            // Ignore — the task may have thrown before the cancel arrived.
        }

        _cts.Dispose();
    }
}
