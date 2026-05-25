using System.Collections.ObjectModel;

namespace MGA.Desktop.Services;

// ---------------------------------------------------------------------------
// Toast model
// ---------------------------------------------------------------------------

public enum ToastTone { Info, Success, Error }

public sealed class ToastMessage
{
    private static int s_nextId;

    public int Id { get; } = Interlocked.Increment(ref s_nextId);
    public required ToastTone Tone { get; init; }
    public required string Title { get; init; }
    public string? Description { get; init; }
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

/// <summary>
/// Fire-and-forget toast notification queue.
/// ToastHost control observes Toasts and auto-removes items after a timeout.
/// </summary>
public sealed class ToastService
{
    private const int DefaultDurationMs = 4000;

    /// <summary>Observable collection bound to the ToastHost control.</summary>
    public ObservableCollection<ToastMessage> Toasts { get; } = [];

    public void Show(ToastTone tone, string title, string? description = null,
                     int durationMs = DefaultDurationMs)
    {
        var toast = new ToastMessage { Tone = tone, Title = title, Description = description };
        Toasts.Add(toast);

        // Auto-dismiss after the duration.
        _ = Task.Delay(durationMs).ContinueWith(_ =>
        {
            // Must update on the UI thread — Dispatcher.UIThread for Avalonia.
            Avalonia.Threading.Dispatcher.UIThread.Post(() => Toasts.Remove(toast));
        });
    }

    public void Success(string title, string? description = null) =>
        Show(ToastTone.Success, title, description);

    public void Error(string title, string? description = null) =>
        Show(ToastTone.Error, title, description);

    public void Info(string title, string? description = null) =>
        Show(ToastTone.Info, title, description);
}
