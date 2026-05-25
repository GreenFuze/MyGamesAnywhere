using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Play page — featured games, recently played shelf, and the full game grid.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed class PlayViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;

    public PlayViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;
    }
}
