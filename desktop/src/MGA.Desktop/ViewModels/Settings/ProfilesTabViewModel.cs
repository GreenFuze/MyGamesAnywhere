using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Profiles tab — lists gamer profiles from GET /api/profiles.
/// Loaded on construction and bound directly to <see cref="Profile"/> records —
/// no thin wrapper class is needed since the model already has the required display properties.
/// </summary>
public sealed partial class ProfilesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<Profile> _profiles = [];

    public ProfilesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
        _ = LoadAsync();
    }

    private async Task LoadAsync()
    {
        if (_server.Api is null)
            return;

        IsLoading = true;
        try
        {
            var list = await _server.Api.GetProfilesAsync().ConfigureAwait(true);

            // Bind Profile records directly — no wrapper needed.
            Profiles = new ObservableCollection<Profile>(list);
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load profiles", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
