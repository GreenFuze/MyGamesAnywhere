using System.Net;
using System.Text;
using MGA.Api;
using Xunit;

namespace MGA.Tests.Api;

/// <summary>
/// Tests for the About and Update API methods added in Phase 10.
/// </summary>
public sealed class UpdateApiTests
{
    // ---------------------------------------------------------------------------
    // Shared fake handler
    // ---------------------------------------------------------------------------

    private sealed class FakeHandler : HttpMessageHandler
    {
        private readonly string       _json;
        private readonly HttpStatusCode _status;

        public string? LastMethod { get; private set; }
        public string? LastUri    { get; private set; }

        public FakeHandler(string json, HttpStatusCode status = HttpStatusCode.OK)
        {
            _json   = json;
            _status = status;
        }

        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
        {
            LastMethod = request.Method.Method;
            LastUri    = request.RequestUri?.ToString();
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
    // GetAboutInfoAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetAboutInfoAsync_parses_version_and_build_date()
    {
        const string json = """
            {"version":"1.2.3","commit":"abc123","build_date":"2026-01-15","author_credits":[]}
            """;
        var (svc, _) = Build(json);
        var about = await svc.GetAboutInfoAsync();
        Assert.Equal("1.2.3",      about.Version);
        Assert.Equal("abc123",     about.Commit);
        Assert.Equal("2026-01-15", about.BuildDate);
    }

    [Fact]
    public async Task GetAboutInfoAsync_sends_GET_to_api_about()
    {
        var (svc, handler) = Build("""{"version":"1","commit":"","build_date":""}""");
        await svc.GetAboutInfoAsync();
        Assert.Equal("GET", handler.LastMethod);
        Assert.Contains("/api/about", handler.LastUri!);
    }

    [Fact]
    public async Task GetAboutInfoAsync_parses_author_credits()
    {
        const string json = """
            {"version":"1","commit":"","build_date":"","author_credits":["Alice","Bob"]}
            """;
        var (svc, _) = Build(json);
        var about = await svc.GetAboutInfoAsync();
        Assert.Equal(2, about.AuthorCredits.Count);
        Assert.Contains("Alice", about.AuthorCredits);
    }

    // ---------------------------------------------------------------------------
    // GetUpdateStatusAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task GetUpdateStatusAsync_parses_up_to_date_response()
    {
        const string json = """
            {"current_version":"1.0.0","update_available":false,"install_type":"manual"}
            """;
        var (svc, _) = Build(json);
        var status = await svc.GetUpdateStatusAsync();
        Assert.Equal("1.0.0", status.CurrentVersion);
        Assert.False(status.UpdateAvailable);
    }

    [Fact]
    public async Task GetUpdateStatusAsync_parses_update_available_response()
    {
        const string json = """
            {
              "current_version":"1.0.0",
              "latest_version":"1.1.0",
              "update_available":true,
              "release_notes_url":"https://example.com/notes",
              "install_type":"auto",
              "message":"New features available"
            }
            """;
        var (svc, _) = Build(json);
        var status = await svc.GetUpdateStatusAsync();
        Assert.True(status.UpdateAvailable);
        Assert.Equal("1.1.0",                   status.LatestVersion);
        Assert.Equal("New features available",  status.Message);
        Assert.Equal("https://example.com/notes", status.ReleaseNotesUrl);
    }

    // ---------------------------------------------------------------------------
    // CheckForUpdatesAsync
    // ---------------------------------------------------------------------------

    [Fact]
    public async Task CheckForUpdatesAsync_sends_POST_to_update_check()
    {
        const string json = """{"current_version":"1.0.0","update_available":false,"install_type":"manual"}""";
        var (svc, handler) = Build(json);
        await svc.CheckForUpdatesAsync();
        Assert.Equal("POST", handler.LastMethod);
        Assert.Contains("/api/update/check", handler.LastUri!);
    }

    [Fact]
    public async Task CheckForUpdatesAsync_returns_parsed_status()
    {
        const string json = """
            {"current_version":"1.0.0","latest_version":"2.0.0","update_available":true,"install_type":"auto"}
            """;
        var (svc, _) = Build(json);
        var status = await svc.CheckForUpdatesAsync();
        Assert.True(status.UpdateAvailable);
        Assert.Equal("2.0.0", status.LatestVersion);
    }

    [Fact]
    public async Task CheckForUpdatesAsync_throws_on_500()
    {
        var (svc, _) = Build("""{"message":"server error"}""", HttpStatusCode.InternalServerError);
        await Assert.ThrowsAsync<MgaApiException>(() => svc.CheckForUpdatesAsync());
    }
}
