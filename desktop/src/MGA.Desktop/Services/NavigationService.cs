using System.Reactive.Subjects;
using MGA.Desktop.ViewModels;

namespace MGA.Desktop.Services;

/// <summary>
/// Manages the active page ViewModel and maintains a capped back-navigation history stack.
///
/// MainWindowViewModel subscribes to CurrentPage and binds the ContentPresenter to it.
/// Avalonia's DataTemplate + ViewLocator convention resolves the matching View automatically.
/// </summary>
public sealed class NavigationService : IDisposable
{
    private readonly BehaviorSubject<ViewModelBase?> _currentPage = new(null);

    // ---------------------------------------------------------------------------
    // History — bounded stack for back navigation
    // ---------------------------------------------------------------------------

    /// <summary>Back-navigation history, most-recent at front. Capped at 20 entries.</summary>
    private readonly LinkedList<ViewModelBase> _history = new();

    private const int MaxHistoryDepth = 20;

    // Flag is true only during NavigateBack() so MainWindowViewModel can skip disposal.
    private bool _isNavigatingBack;

    // ---------------------------------------------------------------------------
    // Public surface
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Observable of the currently active page ViewModel.
    /// Emits null on startup before any navigation has occurred.
    /// </summary>
    public IObservable<ViewModelBase?> CurrentPage => _currentPage;

    /// <summary>The synchronously available current page, for initial binding.</summary>
    public ViewModelBase? Current => _currentPage.Value;

    /// <summary>True when there is at least one page in the back-history stack.</summary>
    public bool CanGoBack => _history.Count > 0;

    /// <summary>
    /// True only during the synchronous execution of <see cref="NavigateBack"/>.
    /// Subscribers (e.g. MainWindowViewModel) use this flag to skip disposing the
    /// outgoing page, since that page is being restored from history and must stay alive.
    /// </summary>
    public bool IsNavigatingBack => _isNavigatingBack;

    // ---------------------------------------------------------------------------
    // Navigation
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Navigates to the given ViewModel, pushing the current page onto the history stack
    /// (unless this call originates from <see cref="NavigateBack"/>).
    /// </summary>
    public void NavigateTo(ViewModelBase vm)
    {
        // Push the current page into history (but not during a back-navigation).
        if (!_isNavigatingBack && _currentPage.Value is { } current)
        {
            _history.AddFirst(current);

            // Trim oldest entry when the cap is exceeded.
            if (_history.Count > MaxHistoryDepth)
                _history.RemoveLast();
        }

        _currentPage.OnNext(vm);
    }

    /// <summary>
    /// Pops the top of the history stack and navigates back to it.
    /// Does nothing when <see cref="CanGoBack"/> is false.
    /// <see cref="IsNavigatingBack"/> is true for the duration of this call.
    /// </summary>
    public void NavigateBack()
    {
        if (_history.Count == 0)
            return;

        var previous = _history.First!.Value;
        _history.RemoveFirst();

        _isNavigatingBack = true;
        try
        {
            _currentPage.OnNext(previous);
        }
        finally
        {
            _isNavigatingBack = false;
        }
    }

    // ---------------------------------------------------------------------------
    // IDisposable
    // ---------------------------------------------------------------------------

    public void Dispose() => _currentPage.Dispose();
}
