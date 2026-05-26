using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// File row display model
// ---------------------------------------------------------------------------

/// <summary>Display row for one file attached to a review candidate.</summary>
public sealed class CandidateFileRowViewModel
{
    public string Path     { get; }
    public string Role     { get; }
    public string FileKind { get; }
    public string SizeText { get; }
    public string FileName { get; }

    public CandidateFileRowViewModel(GameFileDto dto)
    {
        Path     = dto.Path;
        Role     = dto.Role;
        FileKind = dto.FileKind ?? string.Empty;
        SizeText = FormatSize(dto.Size);
        FileName = System.IO.Path.GetFileName(dto.Path);
    }

    private static string FormatSize(long bytes) => bytes switch
    {
        >= 1_073_741_824 => $"{bytes / 1_073_741_824.0:F1} GB",
        >= 1_048_576     => $"{bytes / 1_048_576.0:F1} MB",
        >= 1_024         => $"{bytes / 1_024.0:F1} KB",
        _                => $"{bytes} B",
    };
}

// ---------------------------------------------------------------------------
// Resolver match display model
// ---------------------------------------------------------------------------

/// <summary>Display row for one resolver match on a review candidate.</summary>
public sealed class CandidateMatchRowViewModel
{
    public string Provider   { get; }
    public string Title      { get; }
    public string Platform   { get; }
    public string ExternalId { get; }
    public string? Url       { get; }
    public string? ImageUrl  { get; }

    public CandidateMatchRowViewModel(ResolverMatchDto dto)
    {
        Provider   = dto.Provider;
        Title      = dto.Title;
        Platform   = dto.Platform;
        ExternalId = dto.ExternalId;
        Url        = dto.Url;
        ImageUrl   = dto.ImageUrl;
    }
}

// ---------------------------------------------------------------------------
// Row view-model for one candidate in the list
// ---------------------------------------------------------------------------

/// <summary>Display row for one manual-review candidate.</summary>
public sealed partial class ReviewCandidateRowViewModel : ObservableObject
{
    public string Id               { get; }
    public string Title            { get; }
    public string RawTitle         { get; }
    public string Platform         { get; }
    public string Kind             { get; }
    public string IntegrationLabel { get; }
    public string ReviewState      { get; }
    public int    FileCount        { get; }
    public int    ResolverMatches  { get; }
    public string ReasonsText      { get; }

    /// <summary>True when this candidate is archived (not_a_game, dlc, or matched).</summary>
    public bool IsArchived => ReviewState is "not_a_game" or "matched" or "dlc";

    /// <summary>True when this candidate has at least one resolver match to offer.</summary>
    public bool HasResolverMatches => ResolverMatches > 0;

    public ReviewCandidateRowViewModel(ReviewCandidateSummary dto)
    {
        Id               = dto.Id;
        Title            = dto.CurrentTitle;
        RawTitle         = dto.RawTitle;
        Platform         = dto.Platform;
        Kind             = dto.Kind;
        IntegrationLabel = dto.IntegrationLabel ?? dto.IntegrationId;
        ReviewState      = dto.ReviewState;
        FileCount        = dto.FileCount;
        ResolverMatches  = dto.ResolverMatchCount;
        ReasonsText      = string.Join(", ", dto.ReviewReasons);
    }
}

// ---------------------------------------------------------------------------
// Search result row
// ---------------------------------------------------------------------------

/// <summary>Display row for one metadata search result.</summary>
public sealed class ReviewSearchResultRowViewModel
{
    public string ProviderLabel { get; }
    public string Title         { get; }
    public string Platform      { get; }
    public string Kind          { get; }
    public string ExternalId    { get; }
    public string? ImageUrl     { get; }
    public string? Url          { get; }

    /// <summary>The original DTO, kept for passing to ApplyCandidateMatchAsync.</summary>
    internal ReviewSearchResultDto Dto { get; }

    public ReviewSearchResultRowViewModel(ReviewSearchResultDto dto)
    {
        ProviderLabel = dto.ProviderLabel ?? dto.ProviderPluginId;
        Title         = dto.Title;
        Platform      = dto.Platform ?? string.Empty;
        Kind          = dto.Kind ?? string.Empty;
        ExternalId    = dto.ExternalId;
        ImageUrl      = dto.ImageUrl;
        Url           = dto.Url;
        Dto           = dto;
    }
}

// ---------------------------------------------------------------------------
// Main view-model
// ---------------------------------------------------------------------------

/// <summary>
/// Undetected Games tab — lists manual-review candidates (unrecognised game files),
/// lets the user classify them, search for metadata matches, or delete the files.
/// </summary>
public sealed partial class UndetectedGamesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state — list
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isRedetecting;

    [ObservableProperty]
    private bool _showArchive;

    [ObservableProperty]
    private string _listFilter = string.Empty;

    /// <summary>All candidates for the current scope (active / archive).</summary>
    private List<ReviewCandidateRowViewModel> _allCandidates = [];

    [ObservableProperty]
    private ObservableCollection<ReviewCandidateRowViewModel> _candidates = [];

    // ---------------------------------------------------------------------------
    // Observable state — detail panel
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private ReviewCandidateRowViewModel? _selectedCandidate;

    /// <summary>True while the detail (files + resolver matches) is being loaded.</summary>
    [ObservableProperty]
    private bool _isLoadingDetail;

    [ObservableProperty]
    private ObservableCollection<CandidateFileRowViewModel> _selectedFiles = [];

    [ObservableProperty]
    private ObservableCollection<CandidateMatchRowViewModel> _selectedMatches = [];

    public bool HasFiles   => SelectedFiles.Count > 0;
    public bool HasMatches => SelectedMatches.Count > 0;

    // ---------------------------------------------------------------------------
    // Observable state — search
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isSearching;

    [ObservableProperty]
    private string _searchQuery = string.Empty;

    [ObservableProperty]
    private ObservableCollection<ReviewSearchResultRowViewModel> _searchResults = [];

    [ObservableProperty]
    private bool _isApplying;

    // ---------------------------------------------------------------------------
    // Observable state — status
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private string _statusMessage = string.Empty;

    [ObservableProperty]
    private bool _hasStatusMessage;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public UndetectedGamesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnSelectedCandidateChanged(ReviewCandidateRowViewModel? value)
    {
        // Clear previous detail; trigger lazy-load.
        SelectedFiles   = [];
        SelectedMatches = [];
        SearchResults   = [];
        StatusMessage   = string.Empty;
        HasStatusMessage = false;

        if (value is not null)
        {
            SearchQuery = value.Title;
            _ = LoadDetailAsync(value.Id);
        }
    }

    partial void OnListFilterChanged(string value) => ApplyFilter();

    partial void OnSelectedFilesChanged(ObservableCollection<CandidateFileRowViewModel> value)
    {
        OnPropertyChanged(nameof(HasFiles));
    }

    partial void OnSelectedMatchesChanged(ObservableCollection<CandidateMatchRowViewModel> value)
    {
        OnPropertyChanged(nameof(HasMatches));
    }

    // ---------------------------------------------------------------------------
    // Commands — list
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    /// <summary>Toggles between active and archive views.</summary>
    [RelayCommand]
    private async Task ToggleScopeAsync()
    {
        ShowArchive = !ShowArchive;
        await LoadAsync().ConfigureAwait(true);
    }

    /// <summary>Batch re-detects all pending candidates.</summary>
    [RelayCommand]
    private async Task RedetectAllAsync()
    {
        if (_server.Api is null || IsRedetecting) return;

        IsRedetecting    = true;
        StatusMessage    = "Re-detecting all candidates…";
        HasStatusMessage = true;

        try
        {
            var result = await _server.Api.RedetectAllCandidatesAsync().ConfigureAwait(true);
            StatusMessage = $"Re-detect complete: {result.Matched} matched, {result.Unidentified} unidentified out of {result.Attempted}.";

            // Refresh the list after batch redetect.
            await LoadAsync().ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            _toast.Error("Re-detect failed", ex.Message);
            HasStatusMessage = false;
        }
        finally
        {
            IsRedetecting = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Commands — per-candidate actions
    // ---------------------------------------------------------------------------

    /// <summary>Selects a candidate — drives the detail panel.</summary>
    [RelayCommand]
    private void SelectCandidate(ReviewCandidateRowViewModel row)
    {
        SelectedCandidate = row;
    }

    /// <summary>Searches metadata providers for the selected candidate.</summary>
    [RelayCommand]
    private async Task SearchAsync()
    {
        if (_server.Api is null || SelectedCandidate is null || IsSearching) return;

        IsSearching   = true;
        SearchResults = [];

        try
        {
            var resp = await _server.Api.SearchCandidateAsync(
                SelectedCandidate.Id,
                string.IsNullOrWhiteSpace(SearchQuery) ? null : SearchQuery).ConfigureAwait(true);

            SearchResults = new ObservableCollection<ReviewSearchResultRowViewModel>(
                resp.Results.Select(r => new ReviewSearchResultRowViewModel(r)));

            if (SearchResults.Count == 0)
                _toast.Info("No matches", "No metadata results found. Try a different query.");
        }
        catch (Exception ex)
        {
            _toast.Error("Search failed", ex.Message);
        }
        finally
        {
            IsSearching = false;
        }
    }

    /// <summary>Applies a search result match to the selected candidate.</summary>
    [RelayCommand]
    private async Task ApplyMatchAsync(ReviewSearchResultRowViewModel result)
    {
        if (_server.Api is null || SelectedCandidate is null || IsApplying) return;

        IsApplying = true;

        try
        {
            await _server.Api.ApplyCandidateMatchAsync(SelectedCandidate.Id, result.Dto).ConfigureAwait(true);
            _toast.Success("Match applied", $"Candidate matched to \"{result.Title}\".");

            // Remove the row from the list and clear selection.
            RemoveAndDeselectRow(SelectedCandidate);
        }
        catch (Exception ex)
        {
            _toast.Error("Apply failed", ex.Message);
        }
        finally
        {
            IsApplying = false;
        }
    }

    /// <summary>Re-detects a single candidate.</summary>
    [RelayCommand]
    private async Task RedetectOneAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            var resp = await _server.Api.RedetectCandidateAsync(row.Id).ConfigureAwait(true);

            if (resp.Result.Status == "matched")
            {
                _toast.Success("Match found", $"Candidate auto-matched ({resp.Result.MatchCount} match(es)).");
                RemoveAndDeselectRow(row);
            }
            else
            {
                _toast.Info("No match", "Candidate could not be automatically identified.");
            }
        }
        catch (Exception ex)
        {
            _toast.Error("Re-detect failed", ex.Message);
        }
    }

    /// <summary>Marks the selected candidate as "not a game" (archives it).</summary>
    [RelayCommand]
    private async Task MarkNotGameAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.MarkCandidateNotGameAsync(row.Id).ConfigureAwait(true);
            _toast.Success("Archived", "Candidate marked as Not a Game.");
            RemoveAndDeselectRow(row);
        }
        catch (Exception ex)
        {
            _toast.Error("Action failed", ex.Message);
        }
    }

    /// <summary>Marks the selected candidate as DLC (archives it).</summary>
    [RelayCommand]
    private async Task MarkDlcAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.MarkCandidateDlcAsync(row.Id).ConfigureAwait(true);
            _toast.Success("Archived", "Candidate marked as Add-on / DLC.");
            RemoveAndDeselectRow(row);
        }
        catch (Exception ex)
        {
            _toast.Error("Action failed", ex.Message);
        }
    }

    /// <summary>Marks the selected candidate as a base game (un-archives a wrongly-classified add-on).</summary>
    [RelayCommand]
    private async Task MarkBaseGameAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.MarkCandidateBaseGameAsync(row.Id).ConfigureAwait(true);
            _toast.Success("Reclassified", "Candidate reclassified as Base Game — returned to active queue.");
            RemoveAndDeselectRow(row);
        }
        catch (Exception ex)
        {
            _toast.Error("Action failed", ex.Message);
        }
    }

    /// <summary>Restores an archived candidate back to the active queue.</summary>
    [RelayCommand]
    private async Task UnarchiveAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            await _server.Api.UnarchiveCandidateAsync(row.Id).ConfigureAwait(true);
            _toast.Success("Restored", "Candidate moved back to active queue.");
            RemoveAndDeselectRow(row);
        }
        catch (Exception ex)
        {
            _toast.Error("Action failed", ex.Message);
        }
    }

    /// <summary>Permanently deletes the backing files of a candidate.</summary>
    [RelayCommand]
    private async Task DeleteFilesAsync(ReviewCandidateRowViewModel row)
    {
        if (_server.Api is null) return;

        try
        {
            var result = await _server.Api.DeleteCandidateFilesAsync(row.Id).ConfigureAwait(true);
            _toast.Success("Files deleted", result.CanonicalExists
                ? "Files removed. Game remains in library."
                : "Files removed. Entry deleted from library.");
            RemoveAndDeselectRow(row);
        }
        catch (Exception ex)
        {
            _toast.Error("Delete failed", ex.Message);
        }
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        IsLoading         = true;
        SelectedCandidate = null;
        ListFilter        = string.Empty;

        try
        {
            var scope = ShowArchive ? "archive" : "active";
            var list  = await _server.Api.ListReviewCandidatesAsync(scope).ConfigureAwait(true);

            _allCandidates = list.Select(c => new ReviewCandidateRowViewModel(c)).ToList();
            ApplyFilter();
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load undetected games", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    /// <summary>Lazily loads the full detail (files + resolver matches) for a candidate.</summary>
    private async Task LoadDetailAsync(string candidateId)
    {
        if (_server.Api is null) return;

        IsLoadingDetail = true;

        try
        {
            var detail = await _server.Api.GetReviewCandidateAsync(candidateId).ConfigureAwait(true);

            // Only apply if the selection hasn't changed while we were loading.
            if (SelectedCandidate?.Id != detail.Id) return;

            SelectedFiles = new ObservableCollection<CandidateFileRowViewModel>(
                detail.Files.Select(f => new CandidateFileRowViewModel(f)));

            SelectedMatches = new ObservableCollection<CandidateMatchRowViewModel>(
                detail.ResolverMatches.Select(m => new CandidateMatchRowViewModel(m)));
        }
        catch (Exception ex)
        {
            _toast.Error("Could not load detail", ex.Message);
        }
        finally
        {
            IsLoadingDetail = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private — helpers
    // ---------------------------------------------------------------------------

    /// <summary>Filters _allCandidates by ListFilter and republishes to Candidates.</summary>
    private void ApplyFilter()
    {
        var query = ListFilter.Trim();

        var filtered = string.IsNullOrEmpty(query)
            ? _allCandidates
            : _allCandidates.Where(c =>
                c.Title.Contains(query, StringComparison.OrdinalIgnoreCase) ||
                c.Platform.Contains(query, StringComparison.OrdinalIgnoreCase) ||
                c.IntegrationLabel.Contains(query, StringComparison.OrdinalIgnoreCase));

        Candidates = new ObservableCollection<ReviewCandidateRowViewModel>(filtered);
    }

    /// <summary>Removes a row from both _allCandidates and Candidates, clears selection if it matches.</summary>
    private void RemoveAndDeselectRow(ReviewCandidateRowViewModel row)
    {
        _allCandidates.Remove(row);
        Candidates.Remove(row);

        if (SelectedCandidate == row)
        {
            SelectedCandidate = null;
        }
    }
}
