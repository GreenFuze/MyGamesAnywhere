using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Undetected Games tab — manually add games that weren't auto-detected.</summary>
public sealed class UndetectedGamesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public UndetectedGamesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
    }
}
