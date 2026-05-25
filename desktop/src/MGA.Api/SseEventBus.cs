using System.Reactive.Linq;
using System.Reactive.Subjects;
using System.Text.Json;

namespace MGA.Api;

/// <summary>
/// Routes SSE events from an SseClient to typed Rx subscribers.
///
/// Usage:
///   bus.Of("scan_complete").Subscribe(json => { ... });
///   bus.Messages.Subscribe(msg => Console.WriteLine(msg.EventType));
/// </summary>
public sealed class SseEventBus : IDisposable
{
    private readonly Subject<SseMessage> _subject = new();
    private readonly SseClient _client;

    /// <summary>All raw SSE messages as an observable sequence.</summary>
    public IObservable<SseMessage> Messages => _subject.AsObservable();

    public SseEventBus(SseClient client)
    {
        _client = client;
        _client.EventReceived += OnEventReceived;
    }

    // ---------------------------------------------------------------------------
    // Filtering helpers
    // ---------------------------------------------------------------------------

    /// <summary>Filters messages by event type and returns the raw JSON data string.</summary>
    public IObservable<string> Of(string eventType) =>
        _subject
            .Where(m => string.Equals(m.EventType, eventType, StringComparison.Ordinal))
            .Select(m => m.Data);

    /// <summary>Filters messages by event type and deserializes the payload to T.</summary>
    public IObservable<T> Of<T>(string eventType) =>
        Of(eventType)
            .Select(json => JsonSerializer.Deserialize<T>(json)!)
            .Where(v => v is not null);

    // ---------------------------------------------------------------------------
    // Private
    // ---------------------------------------------------------------------------

    private void OnEventReceived(string eventType, string data) =>
        _subject.OnNext(new SseMessage(eventType, data));

    public void Dispose()
    {
        _client.EventReceived -= OnEventReceived;
        _subject.Dispose();
    }
}
