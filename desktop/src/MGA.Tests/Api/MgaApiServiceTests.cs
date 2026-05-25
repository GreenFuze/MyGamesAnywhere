using System.Net;
using System.Text;
using System.Text.Json;
using MGA.Api;
using Xunit;

namespace MGA.Tests.Api;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

file sealed class FakeHandler : HttpMessageHandler
{
    private readonly Func<HttpRequestMessage, HttpResponseMessage> _fn;

    public FakeHandler(Func<HttpRequestMessage, HttpResponseMessage> fn) => _fn = fn;

    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage req,
        CancellationToken ct)
        => Task.FromResult(_fn(req));
}

file static class MgaApiServiceFactory
{
    public static MgaApiService Build(
        Func<HttpRequestMessage, HttpResponseMessage> fn,
        string baseUrl = "http://localhost")
    {
        var http = new HttpClient(new FakeHandler(fn)) { BaseAddress = new Uri(baseUrl) };
        return new MgaApiService(http);
    }

    public static HttpResponseMessage Json(object payload, int status = 200)
    {
        var json = JsonSerializer.Serialize(payload);
        return new HttpResponseMessage((HttpStatusCode)status)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json"),
        };
    }

    public static HttpResponseMessage Empty(int status)
        => new HttpResponseMessage((HttpStatusCode)status);
}

// ---------------------------------------------------------------------------
// PingAsync
// ---------------------------------------------------------------------------

public sealed class PingAsyncTests
{
    [Fact]
    public async Task Returns_true_on_200()
    {
        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Empty(200));
        Assert.True(await svc.PingAsync());
    }

    [Fact]
    public async Task Returns_false_on_exception()
    {
        // Simulate network failure by using a handler that throws.
        var http = new HttpClient(new ThrowingHandler()) { BaseAddress = new Uri("http://localhost") };
        var svc = new MgaApiService(http);
        Assert.False(await svc.PingAsync());
    }

    private sealed class ThrowingHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage req, CancellationToken ct)
            => throw new HttpRequestException("network error");
    }
}

// ---------------------------------------------------------------------------
// ListGamesAsync
// ---------------------------------------------------------------------------

public sealed class ListGamesAsyncTests
{
    [Fact]
    public async Task Parses_response_correctly()
    {
        var payload = new
        {
            total = 2,
            page = 0,
            page_size = 100,
            games = new[]
            {
                new { id = "g1", title = "Alpha", platform = "PC", kind = "game", favorite = false,
                      description = (string?)null, release_date = (string?)null, genres = Array.Empty<string>(),
                      developer = (string?)null, publisher = (string?)null, rating = 0.0,
                      media = Array.Empty<object>(), cover_override = (object?)null, achievement_summary = (object?)null },
                new { id = "g2", title = "Beta",  platform = "PS5", kind = "game", favorite = true,
                      description = (string?)null, release_date = (string?)null, genres = Array.Empty<string>(),
                      developer = (string?)null, publisher = (string?)null, rating = 0.0,
                      media = Array.Empty<object>(), cover_override = (object?)null, achievement_summary = (object?)null },
            },
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.ListGamesAsync();

        Assert.Equal(2, result.Total);
        Assert.Equal("g1", result.Games[0].Id);
        Assert.Equal("Beta", result.Games[1].Title);
        Assert.True(result.Games[1].Favorite);
    }
}

// ---------------------------------------------------------------------------
// GetGameAsync
// ---------------------------------------------------------------------------

public sealed class GetGameAsyncTests
{
    [Fact]
    public async Task Parses_game_detail_correctly()
    {
        var payload = new
        {
            id = "g99",
            title = "My Game",
            platform = "PC",
            kind = "game",
            favorite = false,
            description = "A great game.",
            release_date = (string?)null,
            genres = new[] { "Action" },
            developer = "Dev Co",
            publisher = (string?)null,
            rating = 8.5,
            media = Array.Empty<object>(),
            cover_override = (object?)null,
            achievement_summary = (object?)null,
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.GetGameAsync("g99");

        Assert.Equal("g99", result.Id);
        Assert.Equal("My Game", result.Title);
        Assert.Equal("Dev Co", result.Developer);
        Assert.Equal(8.5, result.Rating);
        Assert.Contains("Action", result.Genres);
    }
}

// ---------------------------------------------------------------------------
// SetFavoriteAsync
// ---------------------------------------------------------------------------

public sealed class SetFavoriteAsyncTests
{
    [Fact]
    public async Task Sends_PUT_when_setting_favorite()
    {
        HttpMethod? capturedMethod = null;
        var svc = MgaApiServiceFactory.Build(req =>
        {
            capturedMethod = req.Method;
            return MgaApiServiceFactory.Empty(200);
        });

        await svc.SetFavoriteAsync("g1", true);

        Assert.Equal(HttpMethod.Put, capturedMethod);
    }

    [Fact]
    public async Task Sends_DELETE_when_clearing_favorite()
    {
        HttpMethod? capturedMethod = null;
        var svc = MgaApiServiceFactory.Build(req =>
        {
            capturedMethod = req.Method;
            return MgaApiServiceFactory.Empty(200);
        });

        await svc.SetFavoriteAsync("g1", false);

        Assert.Equal(HttpMethod.Delete, capturedMethod);
    }
}

// ---------------------------------------------------------------------------
// TriggerScanAsync
// ---------------------------------------------------------------------------

public sealed class TriggerScanAsyncTests
{
    [Fact]
    public async Task Sends_POST_to_scan_endpoint()
    {
        string? capturedPath = null;
        HttpMethod? capturedMethod = null;

        var svc = MgaApiServiceFactory.Build(req =>
        {
            capturedPath   = req.RequestUri!.AbsolutePath;
            capturedMethod = req.Method;
            return MgaApiServiceFactory.Empty(200);
        });

        await svc.TriggerScanAsync();

        Assert.Equal("/api/scan", capturedPath);
        Assert.Equal(HttpMethod.Post, capturedMethod);
    }

    [Fact]
    public async Task Throws_MgaApiException_on_non_2xx()
    {
        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(new { code = "ERR", message = "fail" }, 500));
        await Assert.ThrowsAsync<MgaApiException>(() => svc.TriggerScanAsync());
    }
}

// ---------------------------------------------------------------------------
// GetIntegrationStatusAsync
// ---------------------------------------------------------------------------

public sealed class GetIntegrationStatusAsyncTests
{
    [Fact]
    public async Task Parses_integration_status_list()
    {
        var payload = new[]
        {
            new { integration_id = "i1", plugin_id = "p1", label = "Steam", status = "ok", message = "" },
            new { integration_id = "i2", plugin_id = "p2", label = "GOG",   status = "error", message = "fail" },
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.GetIntegrationStatusAsync();

        Assert.Equal(2, result.Count);
        Assert.Equal("Steam", result[0].Label);
        Assert.Equal("error", result[1].Status);
    }
}

// ---------------------------------------------------------------------------
// RefreshIntegrationAsync
// ---------------------------------------------------------------------------

public sealed class RefreshIntegrationAsyncTests
{
    [Fact]
    public async Task Accepts_202_response()
    {
        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Empty(202));
        // Should not throw.
        await svc.RefreshIntegrationAsync("i1");
    }

    [Fact]
    public async Task Throws_on_500()
    {
        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(new { code = "ERR", message = "server error" }, 500));
        await Assert.ThrowsAsync<MgaApiException>(() => svc.RefreshIntegrationAsync("i1"));
    }
}

// ---------------------------------------------------------------------------
// GetCacheEntriesAsync
// ---------------------------------------------------------------------------

public sealed class GetCacheEntriesAsyncTests
{
    [Fact]
    public async Task Parses_cache_entries_response()
    {
        var payload = new
        {
            entries = new[]
            {
                new { id = "c1", canonical_title = "Game A", source_title = "Game A", integration_label = "Steam",
                      plugin_id = "steam", status = "active", size = 1024L, file_count = 3 },
            },
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.GetCacheEntriesAsync();

        Assert.Single(result.Entries);
        Assert.Equal("c1", result.Entries[0].Id);
        Assert.Equal(1024L, result.Entries[0].Size);
    }
}

// ---------------------------------------------------------------------------
// ClearCacheAsync
// ---------------------------------------------------------------------------

public sealed class ClearCacheAsyncTests
{
    [Fact]
    public async Task Sends_POST_to_cache_clear()
    {
        string? capturedPath   = null;
        HttpMethod? capturedMethod = null;

        var svc = MgaApiServiceFactory.Build(req =>
        {
            capturedPath   = req.RequestUri!.AbsolutePath;
            capturedMethod = req.Method;
            return MgaApiServiceFactory.Empty(200);
        });

        await svc.ClearCacheAsync();

        Assert.Equal("/api/cache/clear", capturedPath);
        Assert.Equal(HttpMethod.Post, capturedMethod);
    }
}

// ---------------------------------------------------------------------------
// GetPluginsAsync
// ---------------------------------------------------------------------------

public sealed class GetPluginsAsyncTests
{
    [Fact]
    public async Task Parses_plugin_list()
    {
        var payload = new[]
        {
            new { plugin_id = "steam-plugin", plugin_version = "1.2.3",
                  provides = new[] { "games", "achievements" }, capabilities = new[] { "scan" } },
            new { plugin_id = "gog-plugin",   plugin_version = "0.9.0",
                  provides = new[] { "games" },                capabilities = Array.Empty<string>() },
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.GetPluginsAsync();

        Assert.Equal(2, result.Count);
        Assert.Equal("steam-plugin", result[0].PluginId);
        Assert.Equal("1.2.3", result[0].Version);
        Assert.Contains("achievements", result[0].Provides);
        Assert.Equal("gog-plugin", result[1].PluginId);
    }
}

// ---------------------------------------------------------------------------
// GetDuplicatesAsync
// ---------------------------------------------------------------------------

public sealed class GetDuplicatesAsyncTests
{
    [Fact]
    public async Task Parses_duplicates_response()
    {
        var payload = new
        {
            mode = "exact",
            groups = new[]
            {
                new
                {
                    id = "grp1",
                    representative_title = "Portal",
                    sources = new[]
                    {
                        new { canonical_game_id = "g1", canonical_title = "Portal", file_count = 2, total_size = 2048L },
                        new { canonical_game_id = "g2", canonical_title = "Portal 2", file_count = 1, total_size = 1024L },
                    },
                },
            },
        };

        var svc = MgaApiServiceFactory.Build(_ => MgaApiServiceFactory.Json(payload));
        var result = await svc.GetDuplicatesAsync();

        Assert.Equal("exact", result.Mode);
        Assert.Single(result.Groups);
        Assert.Equal("Portal", result.Groups[0].RepresentativeTitle);
        Assert.Equal(2, result.Groups[0].Sources.Count);
        Assert.Equal(2048L, result.Groups[0].Sources[0].TotalSize);
    }
}

// ---------------------------------------------------------------------------
// Error path — non-2xx throws MgaApiException with correct StatusCode
// ---------------------------------------------------------------------------

public sealed class ErrorPathTests
{
    [Fact]
    public async Task Non_2xx_throws_MgaApiException_with_status_code()
    {
        var svc = MgaApiServiceFactory.Build(_ =>
            MgaApiServiceFactory.Json(new { code = "NOT_FOUND", message = "Game not found" }, 404));

        var ex = await Assert.ThrowsAsync<MgaApiException>(() => svc.GetGameAsync("missing"));

        Assert.Equal(404, ex.StatusCode);
    }

    [Fact]
    public async Task Exception_message_comes_from_body()
    {
        var svc = MgaApiServiceFactory.Build(_ =>
            MgaApiServiceFactory.Json(new { code = "BAD_REQUEST", message = "Invalid ID format" }, 400));

        var ex = await Assert.ThrowsAsync<MgaApiException>(() => svc.GetGameAsync("bad-id"));

        Assert.Contains("Invalid ID format", ex.Message);
    }
}
