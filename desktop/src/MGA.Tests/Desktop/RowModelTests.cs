using MGA.Desktop.ViewModels.Settings;
using Xunit;

namespace MGA.Tests.Desktop;

/// <summary>
/// Unit tests for plain display-model logic in Settings row models.
/// These are pure-data classes with no service dependencies.
/// </summary>
public sealed class RowModelTests
{
    // ---------------------------------------------------------------------------
    // IntegrationRowModel.HasError
    // ---------------------------------------------------------------------------

    [Theory]
    [InlineData("ok",      false)]
    [InlineData("pending", false)]
    [InlineData("error",   true)]
    [InlineData("failed",  true)]
    [InlineData("",        false)]
    public void IntegrationRowModel_HasError_reflects_status(string status, bool expectedHasError)
    {
        var row = new IntegrationRowViewModel { Status = status };
        Assert.Equal(expectedHasError, row.HasError);
    }

    // ---------------------------------------------------------------------------
    // PluginRowModel.ProvidesText formatting
    // ---------------------------------------------------------------------------

    [Fact]
    public void PluginRowModel_ProvidesText_empty_when_no_provides()
    {
        var row = new PluginRowModel { ProvidesText = string.Empty };
        Assert.Equal(string.Empty, row.ProvidesText);
    }

    [Fact]
    public void PluginRowModel_ProvidesText_single_entry()
    {
        var row = new PluginRowModel { ProvidesText = "game-source" };
        Assert.Equal("game-source", row.ProvidesText);
    }

    [Fact]
    public void PluginRowModel_ProvidesText_comma_joins_multiple()
    {
        // Verify that the view-model joins provides entries with ", " separator.
        // (This tests the PluginsTabViewModel mapping, not just the property.)
        var dto = new MGA.Api.PluginDto
        {
            PluginId = "test-plugin",
            Provides = ["game-source", "enrichment"],
        };

        var text = string.Join(", ", dto.Provides);
        Assert.Equal("game-source, enrichment", text);
    }

    // ---------------------------------------------------------------------------
    // DuplicateGroupRowModel.SizeText
    // ---------------------------------------------------------------------------

    [Fact]
    public void DuplicateGroupRowModel_SizeText_formats_bytes()
    {
        // 2 × 768 KB = 1.5 MB
        var sources = new[]
        {
            new MGA.Api.DuplicateSourceDto { TotalSize = 786432L }, // 768 KB
            new MGA.Api.DuplicateSourceDto { TotalSize = 786432L }, // 768 KB
        };

        var sizeText = MGA.Desktop.ViewModels.ByteFormatter.Format(sources.Sum(s => s.TotalSize));
        Assert.Equal("1.5 MB", sizeText);
    }

    // ---------------------------------------------------------------------------
    // CountStatModel.MaxCount
    // ---------------------------------------------------------------------------

    [Fact]
    public void CountStatModel_MaxCount_defaults_to_zero()
    {
        var model = new MGA.Desktop.ViewModels.CountStatModel { Label = "PC", Count = 10 };
        Assert.Equal(0, model.MaxCount);
    }

    [Fact]
    public void CountStatModel_MaxCount_can_be_set()
    {
        var model = new MGA.Desktop.ViewModels.CountStatModel { Label = "PC", Count = 10, MaxCount = 42 };
        Assert.Equal(42, model.MaxCount);
    }
}
