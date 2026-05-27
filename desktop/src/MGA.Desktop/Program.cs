using Avalonia;
using MGA.Desktop.Services;
using System;
using System.Threading;

namespace MGA.Desktop;

sealed class Program
{
    // Initialization code. Don't use any Avalonia, third-party APIs or any
    // SynchronizationContext-reliant code before AppMain is called: things aren't initialized
    // yet and stuff might break.
    [STAThread]
    public static void Main(string[] args)
    {
        // ── Single-instance check ──────────────────────────────────────────
        // Find the first mga:// URI in args (registered via Inno Setup).
        var mgaUri = Array.Find(args, a =>
            a.StartsWith("mga://", StringComparison.OrdinalIgnoreCase));

        // Try to acquire a per-user named mutex. If another instance already
        // holds it, forward the URI to that instance and exit.
        bool createdNew;
        using var mutex = new Mutex(
            initiallyOwned: true,
            name: $"Local\\{DeepLinkService.MutexName}",
            createdNew: out createdNew);

        if (!createdNew)
        {
            // Another instance is running. Forward the URI if present.
            if (!string.IsNullOrEmpty(mgaUri))
                DeepLinkService.TryForwardToRunningInstance(mgaUri);
            return;
        }

        // ── First instance — start the app normally ────────────────────────
        // Store startup URI so App can process it after the window is shown.
        StartupUri = mgaUri;

        try
        {
            BuildAvaloniaApp().StartWithClassicDesktopLifetime(args);
        }
        finally
        {
            mutex.ReleaseMutex();
        }
    }

    /// <summary>
    /// The <c>mga://</c> URI passed on the command line (if any).
    /// Consumed by <see cref="App"/> after the window is ready.
    /// </summary>
    public static string? StartupUri { get; private set; }

    // Avalonia configuration, don't remove; also used by visual designer.
    public static AppBuilder BuildAvaloniaApp()
        => AppBuilder.Configure<App>()
            .UsePlatformDetect()
#if DEBUG
            .WithDeveloperTools()
#endif
            .WithInterFont()
            .LogToTrace();
}
