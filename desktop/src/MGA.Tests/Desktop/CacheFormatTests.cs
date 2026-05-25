using MGA.Desktop.ViewModels;
using Xunit;

namespace MGA.Tests.Desktop;

/// <summary>
/// Tests for ByteFormatter.Format (formerly CacheTabViewModel.FormatBytes).
/// </summary>
public sealed class CacheFormatTests
{
    [Theory]
    [InlineData(0,          "0 B")]
    [InlineData(500,        "500 B")]
    [InlineData(1024,       "1.0 KB")]
    [InlineData(1536,       "1.5 KB")]
    [InlineData(1048576,    "1.0 MB")]
    [InlineData(1572864,    "1.5 MB")]
    [InlineData(1073741824, "1.0 GB")]
    [InlineData(1610612736, "1.5 GB")]
    public void FormatBytes_returns_expected_string(long bytes, string expected)
    {
        Assert.Equal(expected, ByteFormatter.Format(bytes));
    }
}
