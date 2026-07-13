using MGA.Desktop.Services;
using Xunit;

namespace MGA.Tests.Desktop;

/// <summary>
/// Tests for AppConfigService using the internal constructor that accepts
/// an explicit path, avoiding any writes to %APPDATA%.
/// Each test uses a unique temp file so tests can run in parallel.
/// </summary>
public sealed class AppConfigServiceTests : IDisposable
{
    // Each test instance gets its own temp path.
    private readonly string _tempPath = Path.Combine(
        Path.GetTempPath(),
        $"mga-test-{Guid.NewGuid():N}.json");

    public void Dispose()
    {
        if (File.Exists(_tempPath))
            File.Delete(_tempPath);
    }

    // ---------------------------------------------------------------------------
    // Fresh file
    // ---------------------------------------------------------------------------

    [Fact]
    public void Fresh_file_IsFirstRun_true_and_empty_servers()
    {
        // File does not exist yet.
        var svc = new AppConfigService(_tempPath);

        Assert.True(svc.Config.IsFirstRun);
        Assert.Empty(svc.Config.Servers);
    }

    // ---------------------------------------------------------------------------
    // Update + persist
    // ---------------------------------------------------------------------------

    [Fact]
    public void Update_mutates_and_persists()
    {
        var svc = new AppConfigService(_tempPath);

        svc.Update(c =>
        {
            c.Servers.Add(new ServerProfile { Name = "Home", Url = "http://localhost:8900" });
            c.ActiveServer = "http://localhost:8900";
        });

        // File should now exist.
        Assert.True(File.Exists(_tempPath));

        // In-memory state should reflect mutation.
        Assert.Single(svc.Config.Servers);
        Assert.Equal("http://localhost:8900", svc.Config.ActiveServer);
    }

    // ---------------------------------------------------------------------------
    // Reload from existing file
    // ---------------------------------------------------------------------------

    [Fact]
    public void Reload_restores_saved_values()
    {
        // Write via one instance.
        var svc1 = new AppConfigService(_tempPath);
        svc1.Update(c =>
        {
            c.Servers.Add(new ServerProfile { Name = "Remote", Url = "http://192.168.1.100:8900" });
            c.ActiveServer = "http://192.168.1.100:8900";
            c.ThemeId      = "daylight";
        });

        // Load via a second instance from the same path.
        var svc2 = new AppConfigService(_tempPath);

        Assert.Single(svc2.Config.Servers);
        Assert.Equal("Remote", svc2.Config.Servers[0].Name);
        Assert.Equal("daylight", svc2.Config.ThemeId);
        Assert.Equal("http://192.168.1.100:8900", svc2.Config.ActiveServer);
    }

    // ---------------------------------------------------------------------------
    // Multiple updates accumulate state
    // ---------------------------------------------------------------------------

    [Fact]
    public void Multiple_updates_accumulate_state()
    {
        var svc = new AppConfigService(_tempPath);

        svc.Update(c => c.Servers.Add(new ServerProfile { Name = "A", Url = "http://a" }));
        svc.Update(c => c.Servers.Add(new ServerProfile { Name = "B", Url = "http://b" }));

        Assert.Equal(2, svc.Config.Servers.Count);
    }

    // ---------------------------------------------------------------------------
    // Corrupt JSON — graceful fallback
    // ---------------------------------------------------------------------------

    [Fact]
    public void Corrupt_json_falls_back_to_empty_config()
    {
        // Write garbage JSON.
        File.WriteAllText(_tempPath, "{ this is not valid json !!! }");

        // Should not throw; should silently recover.
        var svc = new AppConfigService(_tempPath);

        Assert.True(svc.Config.IsFirstRun);
        Assert.Empty(svc.Config.Servers);
    }

    // ---------------------------------------------------------------------------
    // IsFirstRun false when fully configured
    // ---------------------------------------------------------------------------

    [Fact]
    public void IsFirstRun_false_when_server_and_gamer_profile_are_set()
    {
        var svc = new AppConfigService(_tempPath);

        svc.Update(c =>
        {
            c.Servers.Add(new ServerProfile { Name = "Main", Url = "http://server" });
            c.ActiveServer = "http://server";
            c.GamerProfileId = "profile-1";
        });

        Assert.False(svc.Config.IsFirstRun);
    }
}
