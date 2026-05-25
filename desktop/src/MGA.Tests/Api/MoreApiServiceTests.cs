using System.Net;
using System.Text;
using MGA.Api;
using Xunit;

namespace MGA.Tests.Api;

/// <summary>
/// Additional MgaApiService tests for methods not covered by MgaApiServiceTests.cs.
/// </summary>
public sealed class MoreApiServiceTests
{
    // ---------------------------------------------------------------------------
    // Shared fake handler
    // ---------------------------------------------------------------------------

    private sealed class FakeHandler : HttpMessageHandler
    {
        private readonly string       _json;
        private readonly HttpStatusCode _status;

        public string? LastRequestUri { get; private set; }
        public string? LastMethod     { get; private set; }

        public FakeHandler(string json, HttpStatusCode status = HttpStatusCode.OK)
        {
            _json   = json;
            _status = status;
        }

        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
        {
            LastRequestUri = request.RequestUri?.ToString();
            LastMethod     = request.Method.Method;
            return Task.FromResult(new HttpResponseMessage(_status)
            {
                Content = new StringContent(_json, Encoding.UTF8, "application/json"),
            });
        }
    }

    private static (MgaApiService svc, FakeHandler handler) Build(
        string json, HttpStatusCode status = HttpStatusCode.OK)
    {
        var handler = new FakeHandler(json, status);
        var http    = new HttpClient(handler) { BaseAddress = new Uri("http://mga-test") };
        return (new MgaApiService(http), handler);
    }

    // ---------------------------------------------------------------------------
    // PingAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task PingAsync_returns_true_on_200()
    {
        var (svc, _) = Build("{}", HttpStatusCode.OK);
        Assert.True(await svc.PingAsync());
    }

    [Fact]
    public async Task PingAsync_returns_false_on_404()
    {
        var (svc, _) = Build("{}", HttpStatusCode.NotFound);
        Assert.False(await svc.PingAsync());
    }

    [Fact]
    public async Task PingAsync_returns_false_on_network_exception()
    {
        // Use a throwing handler.
        var http = new HttpClient(new ThrowingHandler());
        var svc  = new MgaApiService(http);
        Assert.False(await svc.PingAsync());
    }

    private sealed class ThrowingHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
            => throw new HttpRequestException("network down");
    }

    // ---------------------------------------------------------------------------
    // SetFavoriteAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task SetFavoriteAsync_sends_PUT_when_favoriting()
    {
        var (svc, handler) = Build("{}", HttpStatusCode.OK);
        await svc.SetFavoriteAsync("game-1", true);
        Assert.Equal("PUT",  handler.LastMethod);
        Assert.Contains("/api/games/game-1/favorite", handler.LastRequestUri!);
    }

    [Fact]
    public async Task SetFavoriteAsync_sends_DELETE_when_unfavoriting()
    {
        var (svc, handler) = Build("{}", HttpStatusCode.OK);
        await svc.SetFavoriteAsync("game-1", false);
        Assert.Equal("DELETE", handler.LastMethod);
        Assert.Contains("/api/games/game-1/favorite", handler.LastRequestUri!);
    }

    [Fact]
    public async Task SetFavoriteAsync_escapes_slash_in_id()
    {
        // Uri.EscapeDataString encodes '/' as '%2F', preventing path traversal.
        var (svc, handler) = Build("{}", HttpStatusCode.OK);
        await svc.SetFavoriteAsync("a/b", true);
        Assert.Contains("%2F", handler.LastRequestUri!);
    }

    // ---------------------------------------------------------------------------
    // GetMediaUrl
    // ---------------------------------------------------------------------------

    [Fact]
    public void GetMediaUrl_prepends_base_address()
    {
        var (svc, _) = Build("{}");
        var url = svc.GetMediaUrl("/api/media/42");
        Assert.Equal("http://mga-test/api/media/42", url);
    }

    [Fact]
    public void GetMediaUrl_no_double_slash()
    {
        var (svc, _) = Build("{}");
        var url = svc.GetMediaUrl("/api/media/42");
        Assert.DoesNotContain("//api/", url);
    }

    // ---------------------------------------------------------------------------
    // GetLibraryStatsAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetLibraryStatsAsync_parses_canonical_count()
    {
        const string json = """{"canonical_game_count":99,"by_platform":{}}""";
        var (svc, _) = Build(json);
        var stats = await svc.GetLibraryStatsAsync();
        Assert.Equal(99, stats.CanonicalGameCount);
    }

    // ---------------------------------------------------------------------------
    // GetProfilesAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetProfilesAsync_parses_profile_list()
    {
        const string json = """
            [{"id":"p1","display_name":"Alice","role":"admin"},
             {"id":"p2","display_name":"Bob",  "role":"user"}]
            """;
        var (svc, _) = Build(json);
        var profiles = await svc.GetProfilesAsync();
        Assert.Equal(2,       profiles.Count);
        Assert.Equal("Alice", profiles[0].DisplayName);
        Assert.Equal("admin", profiles[0].Role);
    }

    // ---------------------------------------------------------------------------
    // GetAchievementsDashboardAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetAchievementsDashboardAsync_parses_totals()
    {
        const string json = """
            {
              "totals":  {"total_count":200,"unlocked_count":80},
              "systems": [],
              "games":   [],
              "refresh": {"total":5,"success_count":5}
            }
            """;
        var (svc, _) = Build(json);
        var dash = await svc.GetAchievementsDashboardAsync();
        Assert.Equal(200, dash.Totals.TotalCount);
        Assert.Equal(80,  dash.Totals.UnlockedCount);
    }

    // ---------------------------------------------------------------------------
    // StartAchievementsRefreshAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task StartAchievementsRefreshAsync_accepts_202()
    {
        var (svc, handler) = Build("{}", HttpStatusCode.Accepted);
        await svc.StartAchievementsRefreshAsync(); // should not throw
        Assert.Equal("POST", handler.LastMethod);
    }

    [Fact]
    public async Task StartAchievementsRefreshAsync_throws_on_500()
    {
        var (svc, _) = Build("""{"message":"oops"}""", HttpStatusCode.InternalServerError);
        await Assert.ThrowsAsync<MgaApiException>(
            () => svc.StartAchievementsRefreshAsync());
    }
}
