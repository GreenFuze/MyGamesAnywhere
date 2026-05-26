using System.Collections.ObjectModel;
using System.Diagnostics;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Row view-model
// ---------------------------------------------------------------------------

/// <summary>Editable row view-model for one emulator in the list.</summary>
public sealed partial class EmulatorRowViewModel : ObservableObject
{
    public string Id { get; init; } = string.Empty;

    [ObservableProperty]
    private string _name = string.Empty;

    [ObservableProperty]
    private string _executablePath = string.Empty;

    [ObservableProperty]
    private string _platforms = string.Empty;

    [ObservableProperty]
    private string _argsTemplate = "{rom}";

    /// <summary>True while this row's executable is being tested.</summary>
    [ObservableProperty]
    private bool _isTesting;

    /// <summary>Converts this row back to a plain EmulatorEntry for persistence.</summary>
    public EmulatorEntry ToEntry() => new()
    {
        Id             = Id,
        Name           = Name,
        ExecutablePath = ExecutablePath,
        Platforms      = Platforms,
        ArgsTemplate   = ArgsTemplate,
    };
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Emulators tab — purely local config for mapping platforms to emulator executables.
/// No server calls; all data is stored in AppConfigService (config.json).
/// </summary>
public sealed partial class EmulatorsTabViewModel : ViewModelBase
{
    private readonly AppConfigService _config;
    private readonly ToastService     _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<EmulatorRowViewModel> _emulators = [];

    [ObservableProperty]
    private EmulatorRowViewModel? _selectedEmulator;

    [ObservableProperty]
    private bool _isEditing;

    // Edit form fields (bound to the panel that appears below the list)
    [ObservableProperty]
    private string _editName = string.Empty;

    [ObservableProperty]
    private string _editExecutablePath = string.Empty;

    [ObservableProperty]
    private string _editPlatforms = string.Empty;

    [ObservableProperty]
    private string _editArgsTemplate = "{rom}";

    [ObservableProperty]
    private string? _editingId;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public EmulatorsTabViewModel(AppConfigService config, ToastService toast)
    {
        _config = config;
        _toast  = toast;

        LoadFromConfig();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private void AddEmulator()
    {
        // Open empty edit panel.
        EditingId          = null;
        EditName           = string.Empty;
        EditExecutablePath = string.Empty;
        EditPlatforms      = string.Empty;
        EditArgsTemplate   = "{rom}";
        SelectedEmulator   = null;
        IsEditing          = true;
    }

    [RelayCommand]
    private void EditEmulator(EmulatorRowViewModel row)
    {
        EditingId          = row.Id;
        EditName           = row.Name;
        EditExecutablePath = row.ExecutablePath;
        EditPlatforms      = row.Platforms;
        EditArgsTemplate   = row.ArgsTemplate;
        SelectedEmulator   = row;
        IsEditing          = true;
    }

    [RelayCommand]
    private void CancelEdit()
    {
        IsEditing        = false;
        SelectedEmulator = null;
        EditingId        = null;
    }

    [RelayCommand]
    private void SaveEmulator()
    {
        if (string.IsNullOrWhiteSpace(EditName))
        {
            _toast.Error("Validation", "Emulator name is required.");
            return;
        }

        if (string.IsNullOrWhiteSpace(EditExecutablePath))
        {
            _toast.Error("Validation", "Executable path is required.");
            return;
        }

        if (EditingId is null)
        {
            // New emulator.
            var row = new EmulatorRowViewModel
            {
                Id             = Guid.NewGuid().ToString(),
                Name           = EditName,
                ExecutablePath = EditExecutablePath,
                Platforms      = EditPlatforms,
                ArgsTemplate   = EditArgsTemplate,
            };
            Emulators.Add(row);
        }
        else
        {
            // Update existing.
            var existing = Emulators.FirstOrDefault(e => e.Id == EditingId);
            if (existing is not null)
            {
                existing.Name           = EditName;
                existing.ExecutablePath = EditExecutablePath;
                existing.Platforms      = EditPlatforms;
                existing.ArgsTemplate   = EditArgsTemplate;
            }
        }

        PersistToConfig();
        IsEditing        = false;
        SelectedEmulator = null;
        EditingId        = null;
        _toast.Success("Saved", "Emulator configuration saved.");
    }

    [RelayCommand]
    private void DeleteEmulator(EmulatorRowViewModel row)
    {
        Emulators.Remove(row);
        if (SelectedEmulator == row)
        {
            SelectedEmulator = null;
            IsEditing        = false;
        }
        PersistToConfig();
        _toast.Success("Deleted", $"Emulator \"{row.Name}\" removed.");
    }

    /// <summary>
    /// Verifies the emulator executable exists on disk and can be started.
    /// Launches the process with no arguments and immediately terminates it —
    /// this proves the binary is valid without opening any game UI.
    /// </summary>
    [RelayCommand]
    private async Task TestEmulatorAsync(EmulatorRowViewModel row)
    {
        if (row.IsTesting) return;

        var path = row.ExecutablePath.Trim();

        // Step 1: check the file exists on disk.
        if (!File.Exists(path))
        {
            _toast.Error("Test failed", $"File not found:\n{path}");
            return;
        }

        row.IsTesting = true;

        try
        {
            // Step 2: launch the process with no arguments.
            // We give it a short window (500 ms) to start, then kill it
            // immediately — this confirms the binary is executable without
            // waiting for full initialisation.
            var psi = new ProcessStartInfo(path)
            {
                UseShellExecute  = false,
                CreateNoWindow   = true,
                WindowStyle      = ProcessWindowStyle.Hidden,
            };

            using var proc = Process.Start(psi);
            if (proc is null)
            {
                _toast.Error("Test failed", "Could not start the process.");
                return;
            }

            // Wait briefly, then kill — we don't want to keep it open.
            await Task.Delay(500).ConfigureAwait(true);

            if (!proc.HasExited) proc.Kill(entireProcessTree: true);

            _toast.Success("Test passed", $"\"{row.Name}\" launched successfully.");
        }
        catch (Exception ex)
        {
            _toast.Error("Test failed", ex.Message);
        }
        finally
        {
            row.IsTesting = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private — config persistence
    // ---------------------------------------------------------------------------

    private void LoadFromConfig()
    {
        var entries = _config.GetEmulators();
        Emulators = new ObservableCollection<EmulatorRowViewModel>(
            entries.Select(c => new EmulatorRowViewModel
            {
                Id             = c.Id,
                Name           = c.Name,
                ExecutablePath = c.ExecutablePath,
                Platforms      = c.Platforms,
                ArgsTemplate   = c.ArgsTemplate,
            }));
    }

    private void PersistToConfig()
    {
        var entries = Emulators.Select(e => e.ToEntry()).ToList();
        _config.SetEmulators(entries);
        _config.Save();
    }
}
