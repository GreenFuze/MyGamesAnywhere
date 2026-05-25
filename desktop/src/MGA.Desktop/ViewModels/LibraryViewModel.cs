using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Library page — full game collection with sort/filter and shelf/grid/list toggle.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed class LibraryViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;

    public LibraryViewModel(
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        _server = server;
        _nav    = nav;
        _toast  = toast;
    }
}
