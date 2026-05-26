using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>
/// Display model for a single profile row.
/// Maps from a <see cref="Profile"/> API record on construction.
/// </summary>
public sealed class ProfileRowModel
{
    public string Id          { get; }
    public string DisplayName { get; }
    public string Role        { get; }

    public ProfileRowModel(Profile p)
    {
        Id          = p.Id;
        DisplayName = p.DisplayName;
        Role        = p.Role;
    }
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

            // Each ProfileRowModel maps its own Profile record.
            Profiles = new ObservableCollection<ProfileRowModel>(
                list.Select(p => new ProfileRowModel(p)));
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
