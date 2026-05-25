namespace MGA.Desktop.ViewModels;

/// <summary>
/// Shared utility for formatting byte counts as human-readable strings.
/// Used by CacheTabViewModel and DuplicatesTabViewModel.
/// </summary>
internal static class ByteFormatter
{
    private const long KB = 1024L;
    private const long MB = KB * 1024;
    private const long GB = MB * 1024;

    /// <summary>
    /// Converts a raw byte count to a concise human-readable string, e.g.
    /// "2.4 GB", "450 MB", "12.3 KB", or "512 B".
    /// </summary>
    public static string Format(long bytes) => bytes switch
    {
        >= GB => $"{bytes / (double)GB:F1} GB",
        >= MB => $"{bytes / (double)MB:F1} MB",
        >= KB => $"{bytes / (double)KB:F1} KB",
        _     => $"{bytes} B",
    };
}
