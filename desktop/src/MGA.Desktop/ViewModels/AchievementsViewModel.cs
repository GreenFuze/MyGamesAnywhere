using System.Collections.ObjectModel;
using System.Text.Json;
using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Api;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

// ---------------------------------------------------------------------------
// Display models — each handles its own mapping from API types
// ---------------------------------------------------------------------------

/// <summary>
/// Display model for a single game row in the achievements game list.
/// Maps from an <see cref="AchievementGameEntry"/> API record on construction,
/// aggregating totals across all systems and resolving the cover URL.
/// </summary>
public sealed class AchievementGameRowModel
{
    public string  GameId      { get; }
    public string  Title       { get; }
    public string? CoverUrl    { get; }
    public int     Total       { get; }
    public int     Unlocked    { get; }
    public string  PercentText { get; }

    public AchievementGameRowModel(AchievementGameEntry entry, MgaApiService? api)
    {
        // Aggregate totals across all sources for this game.
        int total    = entry.Systems.Sum(s => s.TotalCount);
        int unlocked = entry.Systems.Sum(s => s.UnlockedCount);

        // Resolve cover URL.
        var coverMedia = entry.Game.CoverOverride
                         ?? entry.Game.Media.FirstOrDefault(m => m.Type == "cover");

        GameId      = entry.Game.Id;
        Title       = entry.Game.Title;
        CoverUrl    = coverMedia is not null && api is not null
                      ? api.GetMediaUrl(coverMedia.Url)
                      : null;
        Total       = total;
        Unlocked    = unlocked;
        PercentText = PercentFormatter.Format(unlocked, total);
    }
}

/// <summary>
/// Display model for a single individual achievement.
/// Resolves icon URL, formats unlock date, and computes rarity text on construction.
/// </summary>
public sealed class AchievementRowModel
{
    public string  ExternalId  { get; }
    public string  Title       { get; }
    public string  Description { get; }

    /// <summary>Best icon URL: unlocked icon when achieved, locked icon otherwise.</summary>
    public string? IconUrl     { get; }

    public int     Points      { get; }
    public bool    Unlocked    { get; }

    /// <summary>Formatted local unlock date (e.g. "2024-03-15") or empty string.</summary>
    public string  UnlockedAt  { get; }

    /// <summary>Rarity percentage (e.g. "12.4%") or empty string when not available.</summary>
    public string  RarityText  { get; }

    public AchievementRowModel(AchievementDto dto)
    {
        // Prefer the unlocked icon when achieved; fall back to locked icon.
        string? iconUrl = dto.Unlocked
            ? (dto.UnlockedIcon ?? dto.LockedIcon)
            : (dto.LockedIcon   ?? dto.UnlockedIcon);

        // Format the unlock timestamp to a local date string.
        string unlockedAt = string.Empty;
        if (dto.Unlocked && !string.IsNullOrEmpty(dto.UnlockedAt) &&
            DateTimeOffset.TryParse(dto.UnlockedAt, out var dt))
        {
            unlockedAt = dt.ToLocalTime().ToString("yyyy-MM-dd");
        }

        ExternalId  = dto.ExternalId;
        Title       = dto.Title;
        Description = dto.Description;
        IconUrl     = iconUrl;
        Points      = dto.Points;
        Unlocked    = dto.Unlocked;
        UnlockedAt  = unlockedAt;
        RarityText  = dto.Rarity > 0 ? $"{dto.Rarity:F1}%" : string.Empty;
    }
}

// ---------------------------------------------------------------------------
// AchievementSetRowViewModel — one source/integration set within a game
// ---------------------------------------------------------------------------

/// <summary>
/// Represents one achievement provider set within a game.
/// Maps from an <see cref="AchievementSetDto"/> on construction, building the
/// full <see cref="AllAchievements"/> list and performing an initial filter pass.
///
/// The <see cref="Achievements"/> collection is rebuilt on every search/filter
/// change via <see cref="RebuildFilter"/>.
/// </summary>
public sealed partial class AchievementSetRowViewModel : ObservableObject
{
    public string Source           { get; }
    public string IntegrationLabel { get; }
    public string Platform         { get; }
    public int    TotalCount       { get; }
    public int    UnlockedCount    { get; }
    public string PercentText      { get; }
    public int    TotalPoints      { get; }
    public int    EarnedPoints     { get; }

    /// <summary>Display label: source name + "N / M" counts.</summary>
    public string HeaderLabel =>
        $"{(string.IsNullOrEmpty(IntegrationLabel) ? Source : IntegrationLabel)}  —  {UnlockedCount} / {TotalCount}";

    /// <summary>All achievements before any filtering — immutable once constructed.</summary>
    public IReadOnlyList<AchievementRowModel> AllAchievements { get; }

    /// <summary>Filtered/searched subset — bound to the AXAML ItemsControl.</summary>
    public ObservableCollection<AchievementRowModel> Achievements { get; } = [];

    public AchievementSetRowViewModel(AchievementSetDto dto)
    {
        Source           = dto.Source;
        IntegrationLabel = dto.IntegrationLabel ?? dto.Source;
        Platform         = dto.Platform ?? string.Empty;
        TotalCount       = dto.TotalCount;
        UnlockedCount    = dto.UnlockedCount;
        PercentText      = PercentFormatter.Format(dto.UnlockedCount, dto.TotalCount);
        TotalPoints      = dto.TotalPoints;
        EarnedPoints     = dto.EarnedPoints;
        AllAchievements  = dto.Achievements.Select(a => new AchievementRowModel(a)).ToList();

        // Populate Achievements with the full unfiltered list on construction.
        RebuildFilter(string.Empty, 0);
    }

    /// <summary>
    /// Rebuilds <see cref="Achievements"/> applying a search query and filter mode.
    /// </summary>
    /// <param name="searchText">Case-insensitive substring matched against title and description.</param>
    /// <param name="filterIndex">0 = All, 1 = Unlocked only, 2 = Locked only.</param>
    public void RebuildFilter(string searchText, int filterIndex)
    {
        Achievements.Clear();

        foreach (var a in AllAchievements)
        {
            if (filterIndex == 1 && !a.Unlocked) continue;
            if (filterIndex == 2 &&  a.Unlocked) continue;

            if (!string.IsNullOrEmpty(searchText) &&
                !a.Title.Contains(searchText, StringComparison.OrdinalIgnoreCase) &&
                !a.Description.Contains(searchText, StringComparison.OrdinalIgnoreCase))
                continue;

            Achievements.Add(a);
        }
    }
}

// ---------------------------------------------------------------------------
// AchievementsViewModel
// ---------------------------------------------------------------------------

/// <summary>
/// Achievements page — overview dashboard + per-game explorer drill-down.
///
/// Dashboard mode: overall summary, per-source breakdown, per-game progress rows.
/// Explorer mode: selected game, per-source achievement sets with search + filter.
/// </summary>
public sealed partial class AchievementsViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    /// <summary>Cached explorer response — fetched once, reused on subsequent game selections.</summary>
    private AchievementsExplorerResponse? _explorerCache;

    // ---------------------------------------------------------------------------
    // Loading state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private bool _isLoadingExplorer;

    // ---------------------------------------------------------------------------
    // Refresh progress state (SSE-driven)
    // ---------------------------------------------------------------------------

    /// <summary>True while a server-side achievement refresh job is in progress.</summary>
    [ObservableProperty]
    private bool _isRefreshing;

    /// <summary>Human-readable status line, e.g. "Refreshing RetroAchievements (45/120)".</summary>
    [ObservableProperty]
    private string _refreshProgressText = string.Empty;

    /// <summary>Completed item count — bound to the progress bar Value.</summary>
    [ObservableProperty]
    private int _refreshProgressValue;

    /// <summary>Total item count — bound to the progress bar Maximum. Defaults to 100.</summary>
    [ObservableProperty]
    private int _refreshProgressMax = 100;

    /// <summary>True when the provider is rate-limited and waiting before retrying.</summary>
    [ObservableProperty]
    private bool _isRefreshWaiting;

    /// <summary>Rate-limit message, e.g. "Rate limited (RetroAchievements). Waiting until 14:32."</summary>
    [ObservableProperty]
    private string _refreshWaitingMessage = string.Empty;

    /// <summary>Accumulated warning messages emitted during the current refresh run.</summary>
    [ObservableProperty]
    private ObservableCollection<string> _refreshWarnings = [];

    /// <summary>True when at least one warning has been collected.</summary>
    [ObservableProperty]
    private bool _hasRefreshWarnings;

    /// <summary>Count of collected warnings — updated alongside <see cref="RefreshWarnings"/>.</summary>
    [ObservableProperty]
    private int _refreshWarningCount;

    // ---------------------------------------------------------------------------
    // Dashboard state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private int _totalAchievements;

    [ObservableProperty]
    private int _unlockedAchievements;

    [ObservableProperty]
    private string _unlockPercent = "0%";

    [ObservableProperty]
    private ObservableCollection<AchievementSystemRowModel> _systems = [];

    [ObservableProperty]
    private ObservableCollection<AchievementGameRowModel> _games = [];

    // ---------------------------------------------------------------------------
    // Explorer state (active when SelectedGame != null)
    // ---------------------------------------------------------------------------

    /// <summary>The game currently being drilled into. Null → show dashboard.</summary>
    [ObservableProperty]
    private AchievementGameRowModel? _selectedGame;

    [ObservableProperty]
    private ObservableCollection<AchievementSetRowViewModel> _explorerSets = [];

    [ObservableProperty]
    private string _achievementSearchText = string.Empty;

    /// <summary>0 = All, 1 = Unlocked, 2 = Locked.</summary>
    [ObservableProperty]
    private int _achievementFilterIndex;

    public string[] AchievementFilterOptions { get; } = ["All", "Unlocked", "Locked"];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public AchievementsViewModel(
        ServerConnectionService server,
        ToastService            toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();

        // Subscribe to SSE events so the progress bar tracks a running refresh job.
        WireAchievementRefreshEvents();
    }

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnAchievementSearchTextChanged(string value) => RebuildAchievementFilter();
    partial void OnAchievementFilterIndexChanged(int value)   => RebuildAchievementFilter();

    // ---------------------------------------------------------------------------
    // Commands — dashboard
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Triggers a server-side achievement refresh job.
    /// SSE events (<c>achievement_refresh_started/progress/completed/failed</c>) drive
    /// the progress bar and trigger a dashboard reload — no eager <c>LoadAsync</c> here.
    /// </summary>
    [RelayCommand]
    private async Task RefreshAsync()
    {
        if (_server.Api is null || IsRefreshing)
            return;

        // Reset warning state from any prior run before starting.
        RefreshWarnings.Clear();
        HasRefreshWarnings   = false;
        RefreshWarningCount  = 0;
        RefreshProgressValue = 0;
        RefreshProgressMax   = 100;
        RefreshProgressText  = "Starting refresh…";

        try
        {
            await _server.Api.StartAchievementsRefreshAsync().ConfigureAwait(true);
            // SSE achievement_refresh_started will set IsRefreshing = true.
            // SSE achievement_refresh_completed/failed will reload the dashboard.
            _explorerCache = null;
        }
        catch (Exception ex)
        {
            _toast.Error("Refresh failed", ex.Message);
            RefreshProgressText = string.Empty;
        }
    }

    // ---------------------------------------------------------------------------
    // Commands — explorer
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Opens the explorer detail view for the given game.
    /// Uses the cached explorer response if available; otherwise fetches from the server.
    /// </summary>
    [RelayCommand]
    private async Task SelectGameAsync(AchievementGameRowModel game)
    {
        if (_server.Api is null)
            return;

        SelectedGame           = game;
        AchievementSearchText  = string.Empty;
        AchievementFilterIndex = 0;
        ExplorerSets.Clear();

        IsLoadingExplorer = true;
        try
        {
            // Fetch once; reuse for all subsequent game selections without re-fetching.
            _explorerCache ??= await _server.Api.GetAchievementsExplorerAsync().ConfigureAwait(true);

            var entry = _explorerCache.Games.FirstOrDefault(g => g.Game.Id == game.GameId);
            if (entry is null)
            {
                _toast.Info("No data", "No stored achievement sets found for this game.");
                return;
            }

            // Each AchievementSetRowViewModel maps its own AchievementSetDto.
            foreach (var setDto in entry.Systems)
                ExplorerSets.Add(new AchievementSetRowViewModel(setDto));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load achievements", ex.Message);
            SelectedGame = null;
        }
        finally
        {
            IsLoadingExplorer = false;
        }
    }

    /// <summary>Dismisses the explorer detail view and returns to the dashboard.</summary>
    [RelayCommand]
    private void CloseDetail()
    {
        SelectedGame           = null;
        AchievementSearchText  = string.Empty;
        AchievementFilterIndex = 0;
        ExplorerSets.Clear();
    }

    // ---------------------------------------------------------------------------
    // Private — SSE achievement-refresh event wiring
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Subscribes to all <c>achievement_refresh_*</c> SSE events.
    /// Each callback posts back to the Avalonia UI thread.
    /// </summary>
    private void WireAchievementRefreshEvents()
    {
        if (_server.Events is null)
            return;

        Disposables.Add(_server.Events.Of("achievement_refresh_started")
            .Subscribe(_ => Dispatcher.UIThread.Post(OnAchievementRefreshStarted)));

        Disposables.Add(_server.Events.Of("achievement_refresh_progress")
            .Subscribe(json => Dispatcher.UIThread.Post(() => OnAchievementRefreshProgress(json))));

        Disposables.Add(_server.Events.Of("achievement_refresh_waiting")
            .Subscribe(json => Dispatcher.UIThread.Post(() => OnAchievementRefreshWaiting(json))));

        Disposables.Add(_server.Events.Of("achievement_refresh_warning")
            .Subscribe(json => Dispatcher.UIThread.Post(() => OnAchievementRefreshWarning(json))));

        Disposables.Add(_server.Events.Of("achievement_refresh_completed")
            .Subscribe(msg => Dispatcher.UIThread.Post(() => _ = OnAchievementRefreshCompletedAsync())));

        Disposables.Add(_server.Events.Of("achievement_refresh_failed")
            .Subscribe(json => Dispatcher.UIThread.Post(() => OnAchievementRefreshFailed(json))));
    }

    /// <summary>Refresh job has started — show the progress bar.</summary>
    private void OnAchievementRefreshStarted()
    {
        RefreshProgressText  = "Connecting to achievement providers…";
        RefreshProgressValue = 0;
        RefreshProgressMax   = 100;
        IsRefreshWaiting     = false;
        IsRefreshing         = true;
    }

    /// <summary>
    /// Progress tick — updates the progress bar and status text.
    /// Payload: <c>{ provider_label, items_completed, items_total, current_item, waiting_until }</c>
    /// </summary>
    private void OnAchievementRefreshProgress(string json)
    {
        try
        {
            using var doc         = JsonDocument.Parse(json);
            var root              = doc.RootElement;
            var providerLabel     = root.TryGetProperty("provider_label",  out var pl)  ? pl.GetString()  : null;
            var currentItem       = root.TryGetProperty("current_item",    out var ci)  ? ci.GetString()  : null;
            var completed         = root.TryGetProperty("items_completed", out var cmp) ? cmp.GetInt32()  : 0;
            var total             = root.TryGetProperty("items_total",     out var tot) ? tot.GetInt32()  : 0;
            var waitingUntil      = root.TryGetProperty("waiting_until",   out var wu)  ? wu.GetString()  : null;

            // Build a human-readable status string.
            var label             = !string.IsNullOrEmpty(providerLabel) ? providerLabel : "achievements";
            var itemSuffix        = !string.IsNullOrEmpty(currentItem)   ? $" — {currentItem}" : string.Empty;
            RefreshProgressText   = total > 0
                ? $"Refreshing {label}{itemSuffix} ({completed}/{total})"
                : $"Refreshing {label}{itemSuffix}";

            // Update progress bar.
            if (total > 0)
            {
                RefreshProgressMax   = total;
                RefreshProgressValue = completed;
            }

            // Rate-limit state from the progress payload.
            IsRefreshWaiting      = !string.IsNullOrEmpty(waitingUntil);
            RefreshWaitingMessage  = IsRefreshWaiting
                ? $"Rate limited. Waiting until {waitingUntil}."
                : string.Empty;
        }
        catch
        {
            // Ignore malformed payload — progress bar simply won't update this tick.
        }
    }

    /// <summary>
    /// Provider is rate-limited and waiting before retrying.
    /// Payload: <c>{ provider_label, waiting_until, message }</c>
    /// </summary>
    private void OnAchievementRefreshWaiting(string json)
    {
        try
        {
            using var doc     = JsonDocument.Parse(json);
            var root          = doc.RootElement;
            var message       = root.TryGetProperty("message",        out var msg) ? msg.GetString() : null;
            var waitingUntil  = root.TryGetProperty("waiting_until",  out var wu)  ? wu.GetString() : null;
            var provider      = root.TryGetProperty("provider_label", out var pl)  ? pl.GetString() : null;

            IsRefreshWaiting = true;

            if (!string.IsNullOrEmpty(message))
            {
                RefreshWaitingMessage = message;
            }
            else
            {
                var providerPart  = !string.IsNullOrEmpty(provider)     ? $" ({provider})"          : string.Empty;
                var waitingPart   = !string.IsNullOrEmpty(waitingUntil) ? $". Waiting until {waitingUntil}" : string.Empty;
                RefreshWaitingMessage = $"Rate limited{providerPart}{waitingPart}.";
            }
        }
        catch { /* ignore malformed payload */ }
    }

    /// <summary>
    /// A non-fatal warning was emitted during refresh (e.g. auth error, rate limit).
    /// Payload: <c>{ job_id, message }</c>
    /// </summary>
    private void OnAchievementRefreshWarning(string json)
    {
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (!doc.RootElement.TryGetProperty("message", out var msgElem))
                return;

            var message = msgElem.GetString();
            if (string.IsNullOrEmpty(message))
                return;

            RefreshWarnings.Add(message);
            RefreshWarningCount = RefreshWarnings.Count;
            HasRefreshWarnings  = true;
        }
        catch { /* ignore malformed payload */ }
    }

    /// <summary>
    /// Refresh job completed successfully. Reloads the dashboard.
    /// Payload: <c>{ success_count, skipped_count, warning_count, … }</c>
    /// </summary>
    private async Task OnAchievementRefreshCompletedAsync()
    {
        IsRefreshing          = false;
        IsRefreshWaiting      = false;
        RefreshProgressText   = string.Empty;
        RefreshWaitingMessage = string.Empty;
        _explorerCache        = null;

        var warnSuffix = RefreshWarnings.Count > 0
            ? $" ({RefreshWarnings.Count} warning(s))"
            : string.Empty;
        _toast.Success("Achievements refreshed", $"Refresh completed{warnSuffix}.");

        await LoadAsync().ConfigureAwait(true);
    }

    /// <summary>
    /// Refresh job failed. Surfaces the error as a toast.
    /// Payload: <c>{ job_id, error, finished_at }</c>
    /// </summary>
    private void OnAchievementRefreshFailed(string json)
    {
        IsRefreshing          = false;
        IsRefreshWaiting      = false;
        RefreshProgressText   = string.Empty;
        RefreshWaitingMessage = string.Empty;

        var message = "An unknown error occurred during the refresh.";
        try
        {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("error", out var err))
                message = err.GetString() ?? message;
        }
        catch { /* ignore malformed payload */ }

        _toast.Error("Achievements refresh failed", message);
    }

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null)
            return;

        IsLoading = true;

        try
        {
            var dashboard = await _server.Api.GetAchievementsDashboardAsync().ConfigureAwait(true);

            TotalAchievements    = dashboard.Totals.TotalCount;
            UnlockedAchievements = dashboard.Totals.UnlockedCount;
            UnlockPercent        = PercentFormatter.Format(dashboard.Totals.UnlockedCount, dashboard.Totals.TotalCount);

            // Each AchievementSystemRowModel maps its own AchievementSystemStat.
            Systems = new ObservableCollection<AchievementSystemRowModel>(
                dashboard.Systems.Select(s => new AchievementSystemRowModel(s)));

            // Each AchievementGameRowModel maps its own AchievementGameEntry.
            Games = new ObservableCollection<AchievementGameRowModel>(
                dashboard.Games.Select(entry => new AchievementGameRowModel(entry, _server.Api)));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load achievements", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private void RebuildAchievementFilter()
    {
        foreach (var set in ExplorerSets)
            set.RebuildFilter(AchievementSearchText, AchievementFilterIndex);
    }
}
