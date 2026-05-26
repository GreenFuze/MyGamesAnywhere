using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Source row — one entry within an expanded duplicate group
// ---------------------------------------------------------------------------

/// <summary>
/// Display row for one source entry inside a duplicate group.
/// Owns its inline merge-confirmation state.
/// </summary>
public sealed partial class DuplicateSourceRowViewModel : ObservableObject
{
    /// <summary>Source game ID — passed to the merge API endpoint.</summary>
    public string SourceGameId     { get; }

    /// <summary>Canonical game ID this source belongs to.</summary>
    public string CanonicalGameId  { get; }

    public string CanonicalTitle   { get; }
    public string IntegrationLabel { get; }
    public string RawTitle         { get; }
    public string Platform         { get; }
    public string Kind             { get; }
    public int    FileCount        { get; }
    public string SizeText         { get; }
    public bool   Cached           { get; }

    /// <summary>True when the "Merge into this" confirm strip is showing.</summary>
    [ObservableProperty]
    private bool _isMergePending;

    public DuplicateSourceRowViewModel(DuplicateSourceDto dto)
    {
        SourceGameId     = dto.Source.Id;
        CanonicalGameId  = dto.CanonicalGameId;
        CanonicalTitle   = dto.CanonicalTitle;
        IntegrationLabel = dto.Source.IntegrationLabel ?? dto.Source.IntegrationId;
        RawTitle         = dto.Source.RawTitle;
        Platform         = dto.Source.Platform;
        Kind             = dto.Source.Kind;
        FileCount        = dto.FileCount;
        SizeText         = ByteFormatter.Format(dto.TotalSize);
        Cached           = dto.Cached;
    }
}

// ---------------------------------------------------------------------------
// Group row — one duplicate group with its sources
// ---------------------------------------------------------------------------

/// <summary>
/// Display row for a duplicate group — collapsible, with per-source merge actions.
/// </summary>
public sealed partial class DuplicateGroupRowViewModel : ObservableObject
{
    public string                                              GroupId              { get; }
    public string                                              RepresentativeTitle  { get; }
    public string                                              Mode                 { get; }
    public ObservableCollection<DuplicateSourceRowViewModel>   Sources              { get; }

    /// <summary>Human-readable count: "3 entries".</summary>
    public string SourceCountText => $"{Sources.Count} entries";

    /// <summary>Total size across all sources.</summary>
    public string TotalSizeText { get; }

    /// <summary>True when the sources list is expanded.</summary>
    [ObservableProperty]
    private bool _isExpanded;

    /// <summary>True when a merge is being applied for this group (disables all buttons).</summary>
    [ObservableProperty]
    private bool _isMerging;

    public DuplicateGroupRowViewModel(DuplicateGroupDto dto)
    {
        GroupId             = dto.Id;
        RepresentativeTitle = dto.RepresentativeTitle;
        Mode                = dto.Mode;
        Sources             = new ObservableCollection<DuplicateSourceRowViewModel>(
            dto.Sources.Select(s => new DuplicateSourceRowViewModel(s)));

        var totalBytes = dto.Sources.Sum(s => s.TotalSize);
        TotalSizeText  = ByteFormatter.Format(totalBytes);
    }
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Duplicates tab — lists groups of duplicate canonical games, lets the user
/// inspect the per-source breakdown, and merge all sources into a chosen canonical.
/// </summary>
public sealed partial class DuplicatesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private int _groupCount;

    /// <summary>Current mode: "loose" or "strict".</summary>
    [ObservableProperty]
    private string _mode = "loose";

    /// <summary>True when the current mode is "loose" — drives button visibility.</summary>
    public bool IsLooseMode  => Mode == "loose";

    /// <summary>True when the current mode is "strict" — drives button visibility.</summary>
    public bool IsStrictMode => Mode == "strict";

    [ObservableProperty]
    private ObservableCollection<DuplicateGroupRowViewModel> _groups = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public DuplicatesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnModeChanged(string value)
    {
        OnPropertyChanged(nameof(IsLooseMode));
        OnPropertyChanged(nameof(IsStrictMode));
        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands — list
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    /// <summary>Switches between loose and strict duplicate detection modes.</summary>
    [RelayCommand]
    private void ToggleMode()
    {
        Mode = Mode == "loose" ? "strict" : "loose";
    }

    // ---------------------------------------------------------------------------
    // Commands — per-group
    // ---------------------------------------------------------------------------

    /// <summary>Toggles the source-list expansion for a group.</summary>
    [RelayCommand]
    private void ToggleGroup(DuplicateGroupRowViewModel group)
    {
        group.IsExpanded = !group.IsExpanded;
    }

    // ---------------------------------------------------------------------------
    // Commands — per-source
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Opens the inline merge-confirm strip on the chosen source row,
    /// closing any previously open strip in the same group.
    /// </summary>
    [RelayCommand]
    private void RequestMergeInto(DuplicateSourceRowViewModel row)
    {
        // Find the group that owns this row and close other open strips.
        var group = Groups.FirstOrDefault(g => g.Sources.Contains(row));
        if (group is null) return;

        foreach (var s in group.Sources)
            s.IsMergePending = false;

        row.IsMergePending = true;
    }

    [RelayCommand]
    private void CancelMergeInto(DuplicateSourceRowViewModel row)
    {
        row.IsMergePending = false;
    }

    /// <summary>
    /// Merges all other sources in the group into the chosen preferred canonical.
    /// Calls MergeSourceGameAsync for each non-preferred source in sequence.
    /// </summary>
    [RelayCommand]
    private async Task ConfirmMergeIntoAsync(DuplicateSourceRowViewModel preferred)
    {
        if (_server.Api is null) return;

        var group = Groups.FirstOrDefault(g => g.Sources.Contains(preferred));
        if (group is null || group.IsMerging) return;

        var others = group.Sources.Where(s => s != preferred).ToList();
        if (others.Count == 0)
        {
            preferred.IsMergePending = false;
            return;
        }

        group.IsMerging = true;
        preferred.IsMergePending = false;

        var succeeded = 0;
        var failed    = new List<string>();

        try
        {
            foreach (var other in others)
            {
                try
                {
                    await _server.Api.MergeSourceGameAsync(
                        other.CanonicalGameId,
                        other.SourceGameId,
                        preferred.CanonicalGameId).ConfigureAwait(true);

                    succeeded++;
                }
                catch (Exception ex)
                {
                    failed.Add($"{other.IntegrationLabel}: {ex.Message}");
                }
            }

            if (failed.Count == 0)
            {
                _toast.Success("Merge complete",
                    $"Merged {succeeded} source(s) into \"{preferred.CanonicalTitle}\".");
            }
            else
            {
                _toast.Error("Partial merge",
                    $"{succeeded} succeeded, {failed.Count} failed:\n{string.Join("\n", failed)}");
            }

            // Reload so the group disappears if all sources were merged.
            await LoadAsync().ConfigureAwait(true);
        }
        finally
        {
            group.IsMerging = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        IsLoading = true;

        try
        {
            var response = await _server.Api.GetDuplicatesAsync(Mode).ConfigureAwait(true);

            GroupCount = response.Groups.Count;
            Groups = new ObservableCollection<DuplicateGroupRowViewModel>(
                response.Groups.Select(g => new DuplicateGroupRowViewModel(g)));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load duplicates", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }
}
