using System.Collections.ObjectModel;
using System.Diagnostics;
using Avalonia.Platform.Storage;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;
using MGA.Desktop.Services.Emulation;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Row view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Displays one <see cref="EmulatorInstallRecord"/> in the emulator list.
/// </summary>
public sealed partial class EmulatorRowViewModel : ObservableObject
{
    public string Id { get; init; } = string.Empty;

    /// <summary>Catalog ID, or null for user-defined installs.</summary>
    public string? CatalogId { get; init; }

    [ObservableProperty]
    private string _name = string.Empty;

    [ObservableProperty]
    private string _executablePath = string.Empty;

    [ObservableProperty]
    private bool _mgaManaged;

    [ObservableProperty]
    private string _version = string.Empty;

    /// <summary>
    /// Comma-separated platform IDs covered by this install's configs, e.g. "gba, nes, snes".
    /// Derived at load time from the EmulatorService; empty when no config has been created yet.
    /// </summary>
    [ObservableProperty]
    private string _platforms = string.Empty;

    /// <summary>True while this row's executable is being test-launched.</summary>
    [ObservableProperty]
    private bool _isTesting;

    /// <summary>Maps this row back to a config-compatible record.</summary>
    internal EmulatorInstallRecord ToRecord() => new()
    {
        Id             = Id,
        CatalogId      = CatalogId,
        Name           = Name,
        ExecutablePath = ExecutablePath,
        MgaManaged     = MgaManaged,
        Version        = Version,
    };
}

// ---------------------------------------------------------------------------
// BIOS status row view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Displays the BIOS check result for one (emulator install, platform) pair
/// in the Settings BIOS status list.
/// </summary>
public sealed class BiosStatusRowViewModel
{
    public string InstallId    { get; init; } = string.Empty;
    public string EmulatorName { get; init; } = string.Empty;
    public string Platform     { get; init; } = string.Empty;
    public bool   AllPresent   { get; init; }

    /// <summary>Filenames of required BIOS files that are absent or hash-mismatched.</summary>
    public IReadOnlyList<string> MissingFilenames { get; init; } = [];

    public string StatusIcon => AllPresent ? "✓" : "✗";

    public string StatusText => AllPresent
        ? "All required BIOS files are present."
        : $"Missing: {string.Join(", ", MissingFilenames)}";
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Emulators settings tab — shows installed emulators, manages BIOS and library
/// directories, and displays per-emulator BIOS status with drag-and-drop import.
/// </summary>
public sealed partial class EmulatorsTabViewModel : ViewModelBase
{
    private readonly EmulatorService _emulatorService;
    private readonly ToastService    _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ObservableCollection<EmulatorRowViewModel> _emulators = [];

    [ObservableProperty]
    private EmulatorRowViewModel? _selectedEmulator;

    [ObservableProperty]
    private bool _isEditing;

    // Edit form fields
    [ObservableProperty]
    private string _editName = string.Empty;

    [ObservableProperty]
    private string _editExecutablePath = string.Empty;

    /// <summary>Comma-separated platform IDs for the emulator being added/edited, e.g. "nes,snes,gba".</summary>
    [ObservableProperty]
    private string _editPlatforms = string.Empty;

    /// <summary>Optional launch arguments template for the emulator being added/edited, e.g. "{rom}".</summary>
    [ObservableProperty]
    private string _editArgsTemplate = string.Empty;

    [ObservableProperty]
    private string? _editingId;

    // ── Directory paths ──────────────────────────────────────────────────────

    /// <summary>
    /// Current BIOS directory path — delegates to EmulatorService.
    /// Re-read after SetBiosDirectory() is called to reflect the persisted value.
    /// </summary>
    public string BiosDirectory    => _emulatorService.BiosDirectory;

    /// <summary>Current game library directory path — delegates to EmulatorService.</summary>
    public string LibraryDirectory => _emulatorService.LibraryDirectory;

    // ── BIOS status ──────────────────────────────────────────────────────────

    /// <summary>Per-(emulator,platform) BIOS check results, rebuilt by RefreshBiosStatusAsync.</summary>
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasBiosStatusRows))]
    private ObservableCollection<BiosStatusRowViewModel> _biosStatusRows = [];

    /// <summary>True when at least one BIOS status row is present.</summary>
    public bool HasBiosStatusRows => BiosStatusRows.Count > 0;

    /// <summary>True while the BIOS check is running.</summary>
    [ObservableProperty]
    private bool _isBiosCheckInProgress;

    // ── Drag-and-drop feedback ────────────────────────────────────────────────

    /// <summary>Feedback message shown below the drop zone after a file is processed.</summary>
    [ObservableProperty]
    private string _biosDropMessage = string.Empty;

    /// <summary>True when the last drop operation succeeded (drives green styling).</summary>
    [ObservableProperty]
    private bool _biosDropSuccess;

    /// <summary>True when the last drop operation failed (drives red styling).</summary>
    [ObservableProperty]
    private bool _biosDropError;

    // ── StorageProvider (set by View code-behind after attachment) ───────────

    /// <summary>
    /// Injected by the View after the control attaches to the visual tree.
    /// Required for the Browse Directory commands.
    /// </summary>
    public IStorageProvider? StorageProvider { private get; set; }

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public EmulatorsTabViewModel(EmulatorService emulatorService, ToastService toast)
    {
        _emulatorService = emulatorService;
        _toast           = toast;

        LoadFromService();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private void AddEmulator()
    {
        EditingId          = null;
        EditName           = string.Empty;
        EditExecutablePath = string.Empty;
        EditPlatforms      = string.Empty;
        EditArgsTemplate   = string.Empty;
        SelectedEmulator   = null;
        IsEditing          = true;
    }

    [RelayCommand]
    private void EditEmulator(EmulatorRowViewModel row)
    {
        // Load existing config data for this install so the user can edit platforms + args.
        var configs       = _emulatorService.Configs
                               .Where(c => string.Equals(c.InstallId, row.Id, StringComparison.Ordinal))
                               .OrderBy(c => c.Priority)
                               .ToList();
        var primaryConfig = configs.FirstOrDefault();

        EditingId          = row.Id;
        EditName           = row.Name;
        EditExecutablePath = row.ExecutablePath;
        EditPlatforms      = string.Join(", ",
                                 configs.SelectMany(c => c.Platforms)
                                        .Distinct(StringComparer.OrdinalIgnoreCase)
                                        .OrderBy(p => p));
        EditArgsTemplate   = primaryConfig?.ArgsTemplate ?? string.Empty;
        SelectedEmulator   = row;
        IsEditing          = true;
    }

    [RelayCommand]
    private void CancelEdit()
    {
        IsEditing          = false;
        SelectedEmulator   = null;
        EditingId          = null;
        EditPlatforms      = string.Empty;
        EditArgsTemplate   = string.Empty;
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

        // Parse the comma-separated platform list from the form field.
        var platforms = EditPlatforms
            .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)
            .ToList();

        var argsTemplate = string.IsNullOrWhiteSpace(EditArgsTemplate)
            ? null
            : EditArgsTemplate.Trim();

        if (EditingId is null)
        {
            // ── Add new user-located install ─────────────────────────────────
            var newId = _emulatorService.AddInstall(
                catalogId:      null,
                displayName:    EditName.Trim(),
                executablePath: EditExecutablePath.Trim(),
                mgaManaged:     false);

            // Create a companion launch config when the user specified platforms.
            if (platforms.Count > 0)
            {
                _emulatorService.AddConfig(
                    installId:    newId,
                    displayName:  EditName.Trim(),
                    platforms:    platforms,
                    argsTemplate: argsTemplate);
            }

            var record = _emulatorService.GetInstall(newId)!;
            Emulators.Add(MapRow(record, _emulatorService));
        }
        else
        {
            // ── Update existing install ──────────────────────────────────────
            // EmulatorInstallRecord is not directly mutable; remove and re-add.
            var existing = Emulators.FirstOrDefault(e => e.Id == EditingId);
            if (existing is not null)
            {
                var idx = Emulators.IndexOf(existing);

                // Drop any configs that referenced the old install ID before removing the install.
                foreach (var cfg in _emulatorService.Configs
                             .Where(c => string.Equals(c.InstallId, EditingId, StringComparison.Ordinal))
                             .Select(c => c.Id)
                             .ToList())
                {
                    _emulatorService.RemoveConfig(cfg);
                }

                _emulatorService.RemoveInstall(EditingId);

                var newId = _emulatorService.AddInstall(
                    catalogId:      existing.CatalogId,
                    displayName:    EditName.Trim(),
                    executablePath: EditExecutablePath.Trim(),
                    mgaManaged:     existing.MgaManaged);

                if (platforms.Count > 0)
                {
                    _emulatorService.AddConfig(
                        installId:    newId,
                        displayName:  EditName.Trim(),
                        platforms:    platforms,
                        argsTemplate: argsTemplate);
                }

                var updated = _emulatorService.GetInstall(newId)!;
                Emulators[idx] = MapRow(updated, _emulatorService);
            }
        }

        IsEditing          = false;
        SelectedEmulator   = null;
        EditingId          = null;
        EditPlatforms      = string.Empty;
        EditArgsTemplate   = string.Empty;
        _toast.Success("Saved", "Emulator configuration saved.");
    }

    [RelayCommand]
    private void DeleteEmulator(EmulatorRowViewModel row)
    {
        _emulatorService.RemoveInstall(row.Id);
        Emulators.Remove(row);

        if (SelectedEmulator == row)
        {
            SelectedEmulator = null;
            IsEditing        = false;
        }

        _toast.Success("Deleted", $"Emulator \"{row.Name}\" removed.");
    }

    /// <summary>
    /// Verifies the emulator executable exists and can be launched.
    /// Starts the process with no arguments, waits briefly, then kills it.
    /// </summary>
    [RelayCommand]
    private async Task TestEmulatorAsync(EmulatorRowViewModel row)
    {
        if (row.IsTesting) return;

        var path = row.ExecutablePath.Trim();

        if (!File.Exists(path))
        {
            _toast.Error("Test failed", $"File not found:\n{path}");
            return;
        }

        row.IsTesting = true;

        try
        {
            var psi = new ProcessStartInfo(path)
            {
                UseShellExecute = false,
                CreateNoWindow  = true,
                WindowStyle     = ProcessWindowStyle.Hidden,
            };

            using var proc = Process.Start(psi);
            if (proc is null)
            {
                _toast.Error("Test failed", "Could not start the process.");
                return;
            }

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
    // Browse directory commands
    // ---------------------------------------------------------------------------

    /// <summary>Opens a folder picker to choose a custom BIOS directory.</summary>
    [RelayCommand]
    private async Task BrowseBiosDirectoryAsync()
    {
        if (StorageProvider is null)
        {
            _toast.Error("Browse unavailable", "Storage provider not ready.");
            return;
        }

        var folders = await StorageProvider.OpenFolderPickerAsync(new FolderPickerOpenOptions
        {
            Title                = "Select BIOS Directory",
            AllowMultiple        = false,
            SuggestedStartLocation = await StorageProvider.TryGetFolderFromPathAsync(BiosDirectory)
                                       .ConfigureAwait(true),
        }).ConfigureAwait(true);

        if (folders is not { Count: > 0 }) return;

        var path = folders[0].Path.LocalPath;
        _emulatorService.SetBiosDirectory(path);
        OnPropertyChanged(nameof(BiosDirectory));
        _toast.Success("BIOS Directory", $"BIOS directory set to:\n{path}");
    }

    /// <summary>Opens a folder picker to choose a custom game library directory.</summary>
    [RelayCommand]
    private async Task BrowseLibraryDirectoryAsync()
    {
        if (StorageProvider is null)
        {
            _toast.Error("Browse unavailable", "Storage provider not ready.");
            return;
        }

        var folders = await StorageProvider.OpenFolderPickerAsync(new FolderPickerOpenOptions
        {
            Title                = "Select Library Directory",
            AllowMultiple        = false,
            SuggestedStartLocation = await StorageProvider.TryGetFolderFromPathAsync(LibraryDirectory)
                                       .ConfigureAwait(true),
        }).ConfigureAwait(true);

        if (folders is not { Count: > 0 }) return;

        var path = folders[0].Path.LocalPath;
        _emulatorService.SetLibraryDirectory(path);
        OnPropertyChanged(nameof(LibraryDirectory));
        _toast.Success("Library Directory", $"Library directory set to:\n{path}");
    }

    /// <summary>Opens the current BIOS directory in Explorer.</summary>
    [RelayCommand]
    private void OpenBiosDirectory()
    {
        try
        {
            Directory.CreateDirectory(BiosDirectory);
            Process.Start(new ProcessStartInfo(BiosDirectory) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Cannot open directory", ex.Message);
        }
    }

    /// <summary>Opens the current library directory in Explorer.</summary>
    [RelayCommand]
    private void OpenLibraryDirectory()
    {
        try
        {
            Directory.CreateDirectory(LibraryDirectory);
            Process.Start(new ProcessStartInfo(LibraryDirectory) { UseShellExecute = true });
        }
        catch (Exception ex)
        {
            _toast.Error("Cannot open directory", ex.Message);
        }
    }

    // ---------------------------------------------------------------------------
    // BIOS status refresh
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Runs BIOS checks for all installed emulators and rebuilds BiosStatusRows.
    /// Runs off the UI thread; updates the collection on return.
    /// </summary>
    [RelayCommand]
    private async Task RefreshBiosStatusAsync()
    {
        if (IsBiosCheckInProgress) return;

        IsBiosCheckInProgress = true;
        BiosDropMessage       = string.Empty;
        BiosDropSuccess       = false;
        BiosDropError         = false;

        try
        {
            // Run the actual file checks on a thread-pool thread.
            var results = await Task.Run(
                () => _emulatorService.CheckBiosForAllInstallsAsync())
                .ConfigureAwait(true);

            // Rebuild the observable collection on the UI thread.
            var rows = results.Select(r => new BiosStatusRowViewModel
            {
                InstallId        = r.InstallId,
                EmulatorName     = r.EmulatorName,
                Platform         = r.Platform,
                AllPresent       = r.Check.AllRequiredPresent,
                MissingFilenames = r.Check.Missing.Select(m => m.Filename).ToList(),
            }).ToList();

            BiosStatusRows = new ObservableCollection<BiosStatusRowViewModel>(rows);
        }
        catch (Exception ex)
        {
            _toast.Error("BIOS check failed", ex.Message);
        }
        finally
        {
            IsBiosCheckInProgress = false;
        }
    }

    // ---------------------------------------------------------------------------
    // BIOS drag-and-drop (called from View code-behind)
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Called by the View's DragDrop handler when a file is dropped onto the BIOS zone.
    /// Identifies the file by SHA-256, copies it to the BIOS directory if recognised,
    /// and refreshes the BIOS status list.
    /// </summary>
    public async Task HandleBiosDropAsync(string filePath)
    {
        BiosDropMessage = "Verifying…";
        BiosDropSuccess = false;
        BiosDropError   = false;

        try
        {
            // Hash the file and compare against all catalog entries.
            await using var stream = File.OpenRead(filePath);
            var match = await _emulatorService.TryIdentifyBiosFileAsync(stream)
                                              .ConfigureAwait(true);

            if (match is null)
            {
                // Reopen stream to compute hash for the user-facing message.
                await using var s2 = File.OpenRead(filePath);
                using var sha = System.Security.Cryptography.SHA256.Create();
                var hashBytes = await sha.ComputeHashAsync(s2).ConfigureAwait(true);
                var hash = Convert.ToHexString(hashBytes).ToLowerInvariant();

                BiosDropMessage = $"Unrecognised file — SHA-256: {hash[..12]}…";
                BiosDropError   = true;
                return;
            }

            // Copy the file into the BIOS directory under its canonical filename.
            stream.Seek(0, SeekOrigin.Begin);
            await _emulatorService.PlaceBiosFileAsync(stream, match.BiosEntry.Filename)
                                  .ConfigureAwait(true);

            BiosDropMessage = $"Placed: {match.BiosEntry.Filename} ({match.EmulatorName} · {match.Platform})";
            BiosDropSuccess = true;

            // Refresh the status list so the newly placed file shows as present.
            await RefreshBiosStatusAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            BiosDropMessage = $"Error: {ex.Message}";
            BiosDropError   = true;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void LoadFromService()
    {
        Emulators = new ObservableCollection<EmulatorRowViewModel>(
            _emulatorService.Installs.Select(r => MapRow(r, _emulatorService)));
    }

    /// <summary>
    /// Maps an install record to a row view-model, deriving the Platforms display string
    /// from any launch configs that reference this install.
    /// </summary>
    private static EmulatorRowViewModel MapRow(EmulatorInstallRecord record, EmulatorService svc)
    {
        // Collect all unique platforms across configs for this install, sorted alphabetically.
        var allPlatforms = svc.Configs
            .Where(c => string.Equals(c.InstallId, record.Id, StringComparison.Ordinal))
            .SelectMany(c => c.Platforms)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(p => p)
            .ToList();

        return new EmulatorRowViewModel
        {
            Id             = record.Id,
            CatalogId      = record.CatalogId,
            Name           = record.Name,
            ExecutablePath = record.ExecutablePath,
            MgaManaged     = record.MgaManaged,
            Version        = record.Version,
            Platforms      = allPlatforms.Count > 0 ? string.Join(", ", allPlatforms) : string.Empty,
        };
    }
}
