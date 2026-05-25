using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Update tab — check for server/plugin updates and view changelog.</summary>
public sealed class UpdateTabViewModel : ViewModelBase
{
    private readonly ToastService _toast;

    public UpdateTabViewModel(ToastService toast) => _toast = toast;
}
