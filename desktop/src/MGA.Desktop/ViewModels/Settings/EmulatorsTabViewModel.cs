using System.Collections.ObjectModel;
using System.Diagnostics;
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
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Emulators settings tab — shows installed emulators and allows manual add/edit/remove.
///
/// Phase 1 scope: install record management (add by locating an executable,
/// remove, test launch). The full catalog-browsing and BIOS management UI
/// is a separate Phase 2/3 deliverable.
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
