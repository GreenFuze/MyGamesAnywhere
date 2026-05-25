using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Integrations tab — manage game-source plugins (Steam, GOG, Epic, etc.).</summary>
public sealed class IntegrationsTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    public IntegrationsTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
    }
}
