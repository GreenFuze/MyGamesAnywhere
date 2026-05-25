using MGA.Desktop.ViewModels;
using Xunit;

namespace MGA.Tests.Desktop;

/// <summary>
/// Tests for PercentFormatter.Format.
/// </summary>
public sealed class PercentFormatterTests
{
    [Theory]
    [InlineData(0,   0,   "0%")]   // zero total → always 0%
    [InlineData(0,   100, "0%")]   // none unlocked
    [InlineData(50,  100, "50%")]  // half
    [InlineData(100, 100, "100%")] // full completion
    [InlineData(1,   3,   "33%")]  // rounds down
    [InlineData(2,   3,   "67%")]  // rounds up
    public void Format_returns_expected_percentage(int unlocked, int total, string expected)
    {
        Assert.Equal(expected, PercentFormatter.Format(unlocked, total));
    }
}
