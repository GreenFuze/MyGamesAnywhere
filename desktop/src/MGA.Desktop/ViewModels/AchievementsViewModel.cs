using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Achievements page — progress rings, SSE-driven refresh progress bar.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed class AchievementsViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public AchievementsViewModel(
        ServerConnectionService server,
        ToastService            toast)
    {
        _server = server;
        _toast  = toast;
    }
}
