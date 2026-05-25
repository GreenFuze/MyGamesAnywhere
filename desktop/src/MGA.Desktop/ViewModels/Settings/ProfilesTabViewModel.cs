using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Display model for a single profile row.</summary>
public sealed class ProfileRowModel
{
    public string Id          { get; init; } = string.Empty;
    public string DisplayName { get; init; } = string.Empty;
    public string Role        { get; init; } = string.Empty;
}

/// <summary>
/// Profiles tab — lists gamer profiles from GET /api/profiles.
/// Loaded on construction.
/// </summary>
public sealed partial class ProfilesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<ProfileRowModel> _profiles = [];

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

            Profiles = new ObservableCollection<ProfileRowModel>(
                list.Select(p => new ProfileRowModel
                {
                    Id          = p.Id,
                    DisplayName = p.DisplayName,
                    Role        = p.Role,
                }));
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
