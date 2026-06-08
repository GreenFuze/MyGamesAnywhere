using System.Collections.ObjectModel;
using System.Globalization;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// ProfileRowViewModel — wraps the raw Profile record for the UI
// ---------------------------------------------------------------------------

/// <summary>
/// One row in the Profiles list.  Wraps the raw <see cref="Profile"/> API record
/// to add display formatting and an active-profile indicator.
/// </summary>
public sealed partial class ProfileRowViewModel : ObservableObject
{
    public string Id          { get; }
    public string DisplayName { get; }
    public string RawRole     { get; }

    /// <summary>Role formatted for display: "admin_player" → "Admin Player".</summary>
    public string DisplayRole =>
        CultureInfo.InvariantCulture.TextInfo.ToTitleCase(
            RawRole.Replace('_', ' ').ToLowerInvariant());

    /// <summary>True when this profile is the currently active one.</summary>
    [ObservableProperty]
    private bool _isActive;

    public ProfileRowViewModel(Profile profile, bool isActive)
    {
        Id          = profile.Id;
        DisplayName = profile.DisplayName;
        RawRole     = profile.Role ?? string.Empty;
        _isActive   = isActive;
    }
}

// ---------------------------------------------------------------------------
// ProfilesTabViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Profiles tab — lists gamer profiles from GET /api/profiles.
/// Supports switching the active profile via SetActiveProfile on
/// <see cref="ServerConnectionService"/>.
/// </summary>
public sealed partial class ProfilesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private ObservableCollection<ProfileRowViewModel> _profiles = [];

    public ProfilesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;
        _ = LoadAsync();
    }

    // ── Commands ──────────────────────────────────────────────────────────────

    /// <summary>Makes the given profile the active one and persists it to config.</summary>
    [RelayCommand]
    private void SwitchProfile(ProfileRowViewModel row)
    {
        // Pass the display name so the sidebar pill shows "TCs" instead of a UUID.
        _server.SetActiveProfile(row.Id, row.DisplayName);

        // Update IsActive flags in the list without a full reload.
        foreach (var p in Profiles)
            p.IsActive = p.Id == row.Id;

        _toast.Success("Profile switched", $"Now active: {row.DisplayName}");
    }

    // ── Loading ──────────────────────────────────────────────────────────────

    private async Task LoadAsync()
    {
        if (_server.Api is null)
            return;

        IsLoading = true;
        try
        {
            var list       = await _server.Api.GetProfilesAsync().ConfigureAwait(true);
            var activeId   = _server.ActiveProfileId;

            Profiles = new ObservableCollection<ProfileRowViewModel>(
                list.Select(p => new ProfileRowViewModel(p, isActive: p.Id == activeId)));
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
