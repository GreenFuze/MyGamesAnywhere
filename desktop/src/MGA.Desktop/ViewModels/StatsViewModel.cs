using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Stats page — Library and Gamer tabs with animated bar charts.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed class StatsViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public StatsViewModel(
        ServerConnectionService server,
        ToastService            toast)
    {
        _server = server;
        _toast  = toast;
    }
}
