using System.Net;
using System.Text;
using MGA.Api;
using Xunit;

namespace MGA.Tests.Api;

/// <summary>
/// Tests for GetLibraryStatisticsAsync and GetGamerStatisticsAsync.
/// </summary>
public sealed class StatsApiTests
{
    private sealed class FakeHandler : HttpMessageHandler
    {
        private readonly string _json;
        public FakeHandler(string json) => _json = json;

        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
            => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(_json, Encoding.UTF8, "application/json"),
            });
    }

    private static MgaApiService Build(string json)
    {
        var http = new HttpClient(new FakeHandler(json)) { BaseAddress = new Uri("http://mga-test") };
        return new MgaApiService(http);
    }

    // ---------------------------------------------------------------------------
    // GetLibraryStatisticsAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetLibraryStatisticsAsync_parses_summary_count()
    {
        const string json = """
            {"summary":{"canonical_game_count":55},"platforms":[],"kinds":[],"genres":[],"decades":[],"coverage":[]}
            """;
        var svc = Build(json);
        var stats = await svc.GetLibraryStatisticsAsync();
        Assert.Equal(55, stats.Summary.CanonicalGameCount);
    }

    [Fact]
    public async Task GetLibraryStatisticsAsync_parses_platform_breakdown()
    {
        const string json = """
            {
              "summary":{"canonical_game_count":3},
              "platforms":[
                {"key":"pc","label":"PC","count":2},
                {"key":"ps5","label":"PlayStation 5","count":1}
              ],
              "kinds":[],"genres":[],"decades":[],"coverage":[]
            }
            """;
        var svc = Build(json);
        var stats = await svc.GetLibraryStatisticsAsync();
        Assert.Equal(2,            stats.Platforms.Count);
        Assert.Equal("pc",         stats.Platforms[0].Key);
        Assert.Equal("PC",         stats.Platforms[0].Label);
        Assert.Equal(2,            stats.Platforms[0].Count);
    }

    [Fact]
    public async Task GetLibraryStatisticsAsync_parses_coverage_with_percent()
    {
        const string json = """
            {
              "summary":{"canonical_game_count":10},
              "platforms":[],"kinds":[],"genres":[],"decades":[],
              "coverage":[{"key":"cover","label":"Cover art","count":8,"percent":80.0}]
            }
            """;
        var svc = Build(json);
        var stats = await svc.GetLibraryStatisticsAsync();
        Assert.Single(stats.Coverage);
        Assert.Equal(80.0, stats.Coverage[0].Percent);
    }

    // ---------------------------------------------------------------------------
    // GetGamerStatisticsAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetGamerStatisticsAsync_parses_totals()
    {
        const string json = """
            {
              "total_games":100,
              "favorite_games":12,
              "total_achievements":500,
              "unlocked_achievements":200,
              "achievement_unlock_percent":40.0,
              "achievement_systems":[],
              "achievement_completion_buckets":[]
            }
            """;
        var svc = Build(json);
        var gamer = await svc.GetGamerStatisticsAsync();
        Assert.Equal(100, gamer.TotalGames);
        Assert.Equal(12,  gamer.FavoriteGames);
        Assert.Equal(500, gamer.TotalAchievements);
        Assert.Equal(200, gamer.UnlockedAchievements);
        Assert.Equal(40.0, gamer.AchievementUnlockPercent);
    }

    [Fact]
    public async Task GetGamerStatisticsAsync_parses_achievement_systems()
    {
        const string json = """
            {
              "total_games":5,
              "favorite_games":1,
              "total_achievements":50,
              "unlocked_achievements":25,
              "achievement_unlock_percent":50.0,
              "achievement_systems":[
                {"source":"Steam","game_count":3,"total_count":40,"unlocked_count":20}
              ],
              "achievement_completion_buckets":[]
            }
            """;
        var svc = Build(json);
        var gamer = await svc.GetGamerStatisticsAsync();
        Assert.Single(gamer.AchievementSystems);
        Assert.Equal("Steam", gamer.AchievementSystems[0].Source);
        Assert.Equal(40,      gamer.AchievementSystems[0].TotalCount);
    }
}
