using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

// ---------------------------------------------------------------------------
// Row view-model for one candidate in the list
// ---------------------------------------------------------------------------

/// <summary>Display row for one manual-review candidate.</summary>
public sealed partial class ReviewCandidateRowViewModel : ObservableObject
{
    public string Id               { get; init; } = string.Empty;
    public string Title            { get; init; } = string.Empty;
    public string RawTitle         { get; init; } = string.Empty;
    public string Platform         { get; init; } = string.Empty;
    public string Kind             { get; init; } = string.Empty;
    public string IntegrationLabel { get; init; } = string.Empty;
    public string ReviewState      { get; init; } = string.Empty;
    public int    FileCount        { get; init; }
    public int    ResolverMatches  { get; init; }
    public string ReasonsText      { get; init; } = string.Empty;

    /// <summary>True when this candidate is in the archive (not_a_game or matched).</summary>
    public bool IsArchived => ReviewState is "not_a_game" or "matched";

    /// <summary>True when this candidate has at least one resolver match to offer.</summary>
    public bool HasResolverMatches => ResolverMatches > 0;
}

// ---------------------------------------------------------------------------
// Search result row
// ---------------------------------------------------------------------------

/// <summary>Display row for one metadata search result.</summary>
public sealed class ReviewSearchResultRowViewModel
{
    public string ProviderLabel    { get; init; } = string.Empty;
    public string Title            { get; init; } = string.Empty;
    public string Platform         { get; init; } = string.Empty;
    public string Kind             { get; init; } = string.Empty;
    public string ExternalId       { get; init; } = string.Empty;
    public string? ImageUrl        { get; init; }
    public string? Url             { get; init; }

    /// <summary>The original DTO, kept for passing to ApplyCandidateMatchAsync.</summary>
    internal ReviewSearchResultDto Dto { get; init; } = new();
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
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isRedetecting;

    [ObservableProperty]
    private bool _showArchive;

    [ObservableProperty]
    private ObservableCollection<ReviewCandidateRowViewModel> _candidates = [];

    // Detail / action panel
    [ObservableProperty]
    private ReviewCandidateRowViewModel? _selectedCandidate;

    [ObservableProperty]
    private bool _isSearching;

    [ObservableProperty]
    private string _searchQuery = string.Empty;

    [ObservableProperty]
    private ObservableCollection<ReviewSearchResultRowViewModel> _searchResults = [];

    [ObservableProperty]
    private bool _isApplying;

    // Status feedback
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

        IsRedetecting = true;
        StatusMessage = "Re-detecting all candidates…";
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

    /// <summary>Selects a candidate and clears any previous search results.</summary>
    [RelayCommand]
    private void SelectCandidate(ReviewCandidateRowViewModel row)
    {
        SelectedCandidate = row;
        SearchQuery       = row.Title;
        SearchResults     = [];
        StatusMessage     = string.Empty;
        HasStatusMessage  = false;
    }

    /// <summary>Searches metadata providers for the selected candidate.</summary>
    [RelayCommand]
    private async Task SearchAsync()
    {
        if (_server.Api is null || SelectedCandidate is null || IsSearching) return;

        IsSearching = true;
        SearchResults = [];

        try
        {
            var resp = await _server.Api.SearchCandidateAsync(
                SelectedCandidate.Id,
                string.IsNullOrWhiteSpace(SearchQuery) ? null : SearchQuery).ConfigureAwait(true);

            SearchResults = new ObservableCollection<ReviewSearchResultRowViewModel>(
                resp.Results.Select(r => new ReviewSearchResultRowViewModel
                {
                    ProviderLabel = r.ProviderLabel ?? r.ProviderPluginId,
                    Title         = r.Title,
                    Platform      = r.Platform ?? string.Empty,
                    Kind          = r.Kind ?? string.Empty,
                    ExternalId    = r.ExternalId,
                    ImageUrl      = r.ImageUrl,
                    Url           = r.Url,
                    Dto           = r,
                }));

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
            _toast.Success("Match applied", $"Candidate matched to “{result.Title}”.");

            // Remove the candidate from the list and clear selection.
            Candidates.Remove(SelectedCandidate);
            SelectedCandidate = null;
            SearchResults = [];
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
            var status = resp.Result.Status;

            if (status == "matched")
            {
                _toast.Success("Match found", $"Candidate auto-matched ({resp.Result.MatchCount} match(es)).");
                Candidates.Remove(row);
                if (SelectedCandidate == row) SelectedCandidate = null;
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
            Candidates.Remove(row);
            if (SelectedCandidate == row) SelectedCandidate = null;
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
            Candidates.Remove(row);
            if (SelectedCandidate == row) SelectedCandidate = null;
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
            Candidates.Remove(row);
            if (SelectedCandidate == row) SelectedCandidate = null;
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
            Candidates.Remove(row);
            if (SelectedCandidate == row) SelectedCandidate = null;
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

        IsLoading = true;
        SelectedCandidate = null;
        SearchResults     = [];

        try
        {
            var scope = ShowArchive ? "archive" : "active";
            var list  = await _server.Api.ListReviewCandidatesAsync(scope).ConfigureAwait(true);

            Candidates = new ObservableCollection<ReviewCandidateRowViewModel>(
                list.Select(c => new ReviewCandidateRowViewModel
                {
                    Id               = c.Id,
                    Title            = c.CurrentTitle,
                    RawTitle         = c.RawTitle,
                    Platform         = c.Platform,
                    Kind             = c.Kind,
                    IntegrationLabel = c.IntegrationLabel ?? c.IntegrationId,
                    ReviewState      = c.ReviewState,
                    FileCount        = c.FileCount,
                    ResolverMatches  = c.ResolverMatchCount,
                    ReasonsText      = string.Join(", ", c.ReviewReasons),
                }));
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
}
