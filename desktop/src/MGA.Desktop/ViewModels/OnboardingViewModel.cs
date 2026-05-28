using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

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
///   Step 2 — Select OR create a gamer profile
///   Step 3 — Done (enter the main app)
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

    public bool ShowWelcome       => CurrentStep == WizardStep.Welcome;
    public bool ShowConnect       => CurrentStep == WizardStep.Connect;
    public bool ShowProfile       => CurrentStep == WizardStep.Profile;
    public bool ShowDone          => CurrentStep == WizardStep.Done;

    /// <summary>True when the profile list is empty — show create-profile form instead.</summary>
    public bool ShowCreateProfile => ShowProfile && Profiles.Count == 0;

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

    /// <summary>
    /// Server profiles fetched after a successful server connection.
    /// Bound directly to <see cref="Profile"/> records.
    /// </summary>
    [ObservableProperty]
    private ObservableCollection<Profile> _profiles = [];

    /// <summary>Name typed into the "create first profile" form (shown when server has no profiles).</summary>
    [ObservableProperty]
    private string _newProfileName = string.Empty;

    [ObservableProperty]
    private bool _isCreatingProfile;

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

        // If the server is already connected (URL saved from a previous run)
        // but no profile is selected yet, skip straight to the profile-picker step.
        if (!string.IsNullOrWhiteSpace(config.Config.ActiveServer) && server.Api is not null)
        {
            IsConnected = true;
            ServerUrl   = config.Config.ActiveServer;
            _ = LoadProfilesAndAdvanceAsync();
        }
    }

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnCurrentStepChanged(WizardStep value)
    {
        OnPropertyChanged(nameof(ShowWelcome));
        OnPropertyChanged(nameof(ShowConnect));
        OnPropertyChanged(nameof(ShowProfile));
        OnPropertyChanged(nameof(ShowDone));
        OnPropertyChanged(nameof(ShowCreateProfile));
    }

    partial void OnProfilesChanged(ObservableCollection<Profile> value)
    {
        // Notify AXAML so the create-profile / pick-profile branches flip correctly.
        OnPropertyChanged(nameof(ShowCreateProfile));
    }

    // ---------------------------------------------------------------------------
    // Commands — Step 0: Welcome
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private void GetStarted() => CurrentStep = WizardStep.Connect;

    // ---------------------------------------------------------------------------
    // Commands — Step 1: Connect
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Probes the server URL. On success persists it, switches ServerConnectionService,
    /// and advances to the profile step.
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

            // Persist and switch the live connection.
            _config.Update(c =>
            {
                c.ActiveServer = url;
                if (!c.Servers.Any(s => s.Url == url))
                    c.Servers.Add(new ServerProfile { Name = url, Url = url });
            });

            _server.ConnectToUrl(url);

            IsConnected = true;
            _toast.Success("Connected!", $"Linked to {url}");

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
    // Commands — Step 2: Profile — pick existing
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Immediately confirms the tapped/clicked profile and advances to Done.
    /// Bound to each profile card — no separate "Continue" button required.
    /// </summary>
    [RelayCommand]
    private void SelectAndConfirm(Profile profile)
    {
        _server.SetActiveProfile(profile.Id);
        CurrentStep = WizardStep.Done;
    }

    // ---------------------------------------------------------------------------
    // Commands — Step 2: Profile — create first profile (no profiles on server yet)
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Creates the first gamer profile on the server via POST /api/setup/start-fresh,
    /// then auto-selects it and advances to Done.
    /// This path is only reachable when the server has zero profiles.
    /// </summary>
    [RelayCommand(CanExecute = nameof(CanCreateProfile))]
    private async Task CreateProfileAsync()
    {
        IsCreatingProfile = true;
        CreateProfileCommand.NotifyCanExecuteChanged();

        try
        {
            var profile = await _server.Api!
                .CreateFirstProfileAsync(NewProfileName.Trim())
                .ConfigureAwait(true);

            _server.SetActiveProfile(profile.Id);
            CurrentStep = WizardStep.Done;
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to create profile", ex.Message);
        }
        finally
        {
            IsCreatingProfile = false;
            CreateProfileCommand.NotifyCanExecuteChanged();
        }
    }

    private bool CanCreateProfile() =>
        !IsCreatingProfile && !string.IsNullOrWhiteSpace(NewProfileName);

    // ---------------------------------------------------------------------------
    // Commands — Step 3: Done
    // ---------------------------------------------------------------------------

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

            Profiles = new ObservableCollection<Profile>(profiles);

            // Always go to the Profile step so the user consciously picks or creates.
            CurrentStep = WizardStep.Profile;
        }
        catch
        {
            // Profile load failure — stay on Connect step so the user can retry.
            _toast.Error("Could not load profiles", "Check the server connection and try again.");
        }
        finally
        {
            IsLoadingProfiles = false;
        }
    }
}
