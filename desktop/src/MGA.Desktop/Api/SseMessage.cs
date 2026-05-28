namespace MGA.Api;

/// <summary>
/// A parsed Server-Sent Event from GET /api/events.
/// EventType matches the server's "event:" field; Data is the raw JSON payload string.
/// </summary>
public sealed record SseMessage(string EventType, string Data);
