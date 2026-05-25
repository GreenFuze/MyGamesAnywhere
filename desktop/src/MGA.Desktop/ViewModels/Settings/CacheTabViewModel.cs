using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Cache tab — view cache size and trigger a full cache clear.</summary>
public sealed class CacheTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public CacheTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
    }
}
