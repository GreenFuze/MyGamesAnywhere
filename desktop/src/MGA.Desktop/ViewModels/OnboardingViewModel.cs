using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// ProfileOptionModel
// ---------------------------------------------------------------------------

/// <summary>Display model for one gamer profile row in the wizard profile-picker.</summary>
public sealed class ProfileOptionModel
{
    public string Id          { get; }
    public string DisplayName { get; }
    public string Role        { get; }

    public ProfileOptionModel(Profile p)
    {
        Id          = p.Id;
        DisplayName = p.DisplayName;
        Role        = p.Role;
    }
}

// ---------------------------------------------------------------------------
// WizardStep enum
// ---------------------------------------------------------------------------

public enum WizardStep
{
    Welcome  = 0,
    Connect  = 1,
    Profile  = 2,
    Done     = 3,
}

// ---------------------------------------------------------------------------
// OnboardingViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// First-Run Wizard ViewModel.
/// Guides the user through:
///   Step 0 — Welcome
///   Step 1 — Connect to server (URL input + ping test)
///   Step 2 — Select active gamer profile
///   Step 3 — Done (open Library)
///
/// The wizard fires <see cref="_onConnected"/> once the user reaches step 3.
/// </summary>
public sealed partial class OnboardingViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly AppConfigService        _config;
    private readonly ToastService            _toast;
    private readonly Action                  _onConnected;

    // ---------------------------------------------------------------------------
    // Step tracking
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private WizardStep _currentStep = WizardStep.Welcome;

    public bool ShowWelcome  => CurrentStep == WizardStep.Welcome;
    public bool ShowConnect  => CurrentStep == WizardStep.Connect;
    public bool ShowProfile  => CurrentStep == WizardStep.Profile;
    public bool ShowDone     => CurrentStep == WizardStep.Done;

    // ---------------------------------------------------------------------------
    // Step 1 — Connect
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _serverUrl = "http://localhost:8900";

    [ObservableProperty]
    private bool _isTesting;

    [ObservableProperty]
    private string? _testError;

    [ObservableProperty]
    private bool _isConnected;

    // ---------------------------------------------------------------------------
    // Step 2 — Profile
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoadingProfiles;

    [ObservableProperty]
    private ObservableCollection<ProfileOptionModel> _profiles = [];

    [ObservableProperty]
    private ProfileOptionModel? _selectedProfile;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public OnboardingViewModel(
        ServerConnectionService server,
        AppConfigService        config,
        ToastService            toast,
        Action                  onConnected)
    {
        _server      = server;
        _config      = config;
        _toast       = toast;
        _onConnected = onConnected;
    }

    // ---------------------------------------------------------------------------
    // Property change hooks — keep ShowXxx derived props in sync
    // ---------------------------------------------------------------------------

    partial void OnCurrentStepChanged(WizardStep value)
    {
        OnPropertyChanged(nameof(ShowWelcome));
        OnPropertyChanged(nameof(ShowConnect));
        OnPropertyChanged(nameof(ShowProfile));
        OnPropertyChanged(nameof(ShowDone));
    }

    // ---------------------------------------------------------------------------
    // Commands — Step 0: Welcome
    // ---------------------------------------------------------------------------

    /// <summary>Advance from Welcome to the Connect step.</summary>
    [RelayCommand]
    private void GetStarted() => CurrentStep = WizardStep.Connect;

    // ---------------------------------------------------------------------------
    // Commands — Step 1: Connect
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Probe the given server URL.
    /// On success, persist the URL, switch <see cref="ServerConnectionService"/>,
    /// and advance to the Profile step.
    /// </summary>
    [RelayCommand(CanExecute = nameof(CanConnect))]
    private async Task ConnectAsync()
    {
        TestError = null;
        IsTesting = true;
        ConnectCommand.NotifyCanExecuteChanged();

        try
        {
            var url = ServerUrl.TrimEnd('/');

            // Probe with a temporary HttpClient before committing.
            using var http = new System.Net.Http.HttpClient
            {
                BaseAddress = new Uri(url),
                Timeout     = TimeSpan.FromSeconds(8),
            };

            var ok = await new MgaApiService(http).PingAsync();
            if (!ok)
                throw new InvalidOperationException("Server returned a non-OK response.");

            // Persist the server URL and switch the live connection.
            _config.Update(c =>
            {
                c.ActiveServer = url;
                // Add to servers list if not already present.
                if (!c.Servers.Any(s => s.Url == url))
                    c.Servers.Add(new ServerProfile { Name = url, Url = url });
            });

            _server.ConnectToUrl(url);

            IsConnected = true;
            _toast.Success("Connected!", $"Linked to {url}");

            // Load profiles and move on.
            await LoadProfilesAndAdvanceAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            TestError = ex.Message;
        }
        finally
        {
            IsTesting = false;
            ConnectCommand.NotifyCanExecuteChanged();
        }
    }

    private bool CanConnect() => !IsTesting && !string.IsNullOrWhiteSpace(ServerUrl);

    // ---------------------------------------------------------------------------
    // Commands — Step 2: Profile
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Confirms the selected profile (or skips selection) and advances to Done.
    /// </summary>
    [RelayCommand]
    private void ConfirmProfile()
    {
        if (SelectedProfile is not null)
        {
            _config.Update(c => c.GamerProfileId = SelectedProfile.Id);
        }

        CurrentStep = WizardStep.Done;
    }

    /// <summary>Skips the profile selection step.</summary>
    [RelayCommand]
    private void SkipProfile() => CurrentStep = WizardStep.Done;

    // ---------------------------------------------------------------------------
    // Commands — Step 3: Done
    // ---------------------------------------------------------------------------

    /// <summary>Opens the main Library — fires the onConnected callback.</summary>
    [RelayCommand]
    private void OpenLibrary() => _onConnected();

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private async Task LoadProfilesAndAdvanceAsync()
    {
        IsLoadingProfiles = true;

        try
        {
            var profiles = await _server.Api!.GetProfilesAsync().ConfigureAwait(true);

            Profiles = new ObservableCollection<ProfileOptionModel>(
                profiles.Select(p => new ProfileOptionModel(p)));

            // Auto-select if there's exactly one profile.
            if (Profiles.Count == 1)
                SelectedProfile = Profiles[0];

            // Skip the profile step if the server has no profiles.
            CurrentStep = Profiles.Count == 0 ? WizardStep.Done : WizardStep.Profile;
        }
        catch
        {
            // Profile load failure is non-fatal — just skip to Done.
            CurrentStep = WizardStep.Done;
        }
        finally
        {
            IsLoadingProfiles = false;
        }
    }
}
