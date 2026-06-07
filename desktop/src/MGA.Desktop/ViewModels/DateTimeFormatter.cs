using System.Globalization;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Shared date/time formatting helpers used across ViewModels.
/// All methods are pure, culture-invariant, and fall through to the raw
/// string when parsing fails — safe to call with any API response.
/// </summary>
internal static class DateTimeFormatter
{
    private static readonly CultureInfo _inv = CultureInfo.InvariantCulture;

    // ---------------------------------------------------------------------------
    // Public helpers
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Parses an ISO 8601 date/datetime string and returns a date-only string
    /// in "MMMM d, yyyy" format (e.g. "May 31, 2026").
    /// The parsed value is converted to local time before formatting.
    /// Falls through to the raw string if parsing fails.
    /// </summary>
    public static string FormatDate(string? raw)
    {
        if (string.IsNullOrWhiteSpace(raw))
            return string.Empty;

        return DateTimeOffset.TryParse(raw, _inv, DateTimeStyles.RoundtripKind, out var dt)
            ? dt.ToLocalTime().ToString("MMMM d, yyyy", _inv)
            : raw;
    }

    /// <summary>
    /// Parses an ISO 8601 datetime string and returns a human-readable
    /// "MMM d, yyyy · HH:mm" string (e.g. "May 21, 2026 · 23:24").
    /// The parsed value is converted to local time before formatting.
    /// Falls through to the raw string if parsing fails.
    /// </summary>
    public static string FormatDateTime(string? raw)
    {
        if (string.IsNullOrWhiteSpace(raw))
            return string.Empty;

        return DateTimeOffset.TryParse(raw, _inv, DateTimeStyles.RoundtripKind, out var dt)
            ? dt.ToLocalTime().ToString("MMM d, yyyy · HH:mm", _inv)
            : raw;
    }
}
