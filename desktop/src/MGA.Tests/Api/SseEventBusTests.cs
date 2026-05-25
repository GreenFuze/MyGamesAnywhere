using System.Text.Json;
using MGA.Api;
using Xunit;

namespace MGA.Tests.Api;

/// <summary>
/// Unit tests for SseEventBus using the internal test constructor + Inject helper.
/// No real HTTP connection is required.
/// </summary>
public sealed class SseEventBusTests
{
    // ---------------------------------------------------------------------------
    // Of(eventType) — raw string
    // ---------------------------------------------------------------------------

    [Fact]
    public void Of_emits_matching_event_type()
    {
        using var bus = new SseEventBus();
        var received = new List<string>();

        bus.Of("scan_complete").Subscribe(d => received.Add(d));
        bus.Inject("scan_complete", "{}");

        Assert.Single(received);
        Assert.Equal("{}", received[0]);
    }

    [Fact]
    public void Of_ignores_other_event_types()
    {
        using var bus = new SseEventBus();
        var received = new List<string>();

        bus.Of("scan_complete").Subscribe(d => received.Add(d));
        bus.Inject("scan_error", "{}");
        bus.Inject("other_event", "data");

        Assert.Empty(received);
    }

    [Fact]
    public void Of_is_case_sensitive()
    {
        using var bus = new SseEventBus();
        var received = new List<string>();

        // Subscribe to lowercase; inject uppercase — should not match.
        bus.Of("scan_complete").Subscribe(d => received.Add(d));
        bus.Inject("SCAN_COMPLETE", "{}");

        Assert.Empty(received);
    }

    [Fact]
    public void Of_delivers_raw_data_string()
    {
        using var bus = new SseEventBus();
        string? captured = null;

        bus.Of("my_event").Subscribe(d => captured = d);
        bus.Inject("my_event", "hello world");

        Assert.Equal("hello world", captured);
    }

    // ---------------------------------------------------------------------------
    // Of<T>(eventType) — deserialized payload
    // ---------------------------------------------------------------------------

    [Fact]
    public void OfT_deserializes_json_payload()
    {
        using var bus = new SseEventBus();
        ScanCompletePayload? captured = null;

        bus.Of<ScanCompletePayload>("scan_complete").Subscribe(p => captured = p);
        bus.Inject("scan_complete", JsonSerializer.Serialize(new ScanCompletePayload { Count = 42 }));

        Assert.NotNull(captured);
        Assert.Equal(42, captured.Count);
    }

    // ---------------------------------------------------------------------------
    // Multiple subscribers
    // ---------------------------------------------------------------------------

    [Fact]
    public void Multiple_subscribers_all_receive_event()
    {
        using var bus = new SseEventBus();
        var list1 = new List<string>();
        var list2 = new List<string>();

        bus.Of("evt").Subscribe(d => list1.Add(d));
        bus.Of("evt").Subscribe(d => list2.Add(d));

        bus.Inject("evt", "data");

        Assert.Single(list1);
        Assert.Single(list2);
    }

    // ---------------------------------------------------------------------------
    // Dispose — no events after disposal
    // ---------------------------------------------------------------------------

    [Fact]
    public void No_events_emitted_after_dispose()
    {
        var bus = new SseEventBus();
        var received = new List<string>();
        bus.Of("evt").Subscribe(d => received.Add(d));

        bus.Dispose();

        // Injecting after dispose should not add to received.
        // (Subject.OnNext after Dispose is a no-op / ignored.)
        try { bus.Inject("evt", "late"); } catch { /* Subject may throw — that is fine */ }

        Assert.Empty(received);
    }

    // ---------------------------------------------------------------------------
    // Messages observable
    // ---------------------------------------------------------------------------

    [Fact]
    public void Messages_observable_emits_all_event_types()
    {
        using var bus = new SseEventBus();
        var messages = new List<SseMessage>();

        bus.Messages.Subscribe(m => messages.Add(m));

        bus.Inject("type_a", "d1");
        bus.Inject("type_b", "d2");

        Assert.Equal(2, messages.Count);
        Assert.Equal("type_a", messages[0].EventType);
        Assert.Equal("type_b", messages[1].EventType);
    }

    // ---------------------------------------------------------------------------
    // Helper types
    // ---------------------------------------------------------------------------

    private sealed class ScanCompletePayload
    {
        [System.Text.Json.Serialization.JsonPropertyName("count")]
        public int Count { get; set; }
    }
}
