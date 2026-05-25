using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Profiles tab — manage user profiles on the connected server.</summary>
public sealed class ProfilesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public ProfilesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
    }
}
