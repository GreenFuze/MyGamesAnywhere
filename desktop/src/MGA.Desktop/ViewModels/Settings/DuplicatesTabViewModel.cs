using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Duplicates tab — review and resolve duplicate game entries.</summary>
public sealed class DuplicatesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public DuplicatesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
    }
}
