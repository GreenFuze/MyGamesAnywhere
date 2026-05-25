using System.Reactive.Subjects;
using MGA.Desktop.ViewModels;

namespace MGA.Desktop.Services;

/// <summary>
/// Manages the active page ViewModel.
///
/// MainWindowViewModel subscribes to CurrentPage and binds the ContentPresenter to it.
/// Avalonia's DataTemplate + ViewLocator convention resolves the matching View automatically.
/// </summary>
public sealed class NavigationService : IDisposable
{
    private readonly BehaviorSubject<ViewModelBase?> _currentPage = new(null);

    /// <summary>
    /// Observable of the currently active page ViewModel.
    /// Emits null on startup before any navigation has occurred.
    /// </summary>
    public IObservable<ViewModelBase?> CurrentPage => _currentPage;

    /// <summary>The synchronously available current page, for initial binding.</summary>
    public ViewModelBase? Current => _currentPage.Value;

    // ---------------------------------------------------------------------------
    // Navigation
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Navigates to the given ViewModel.
    /// The old ViewModel is disposed by MainWindowViewModel before this is called.
    /// </summary>
    public void NavigateTo(ViewModelBase vm) => _currentPage.OnNext(vm);

    // ---------------------------------------------------------------------------
    // IDisposable
    // ---------------------------------------------------------------------------

    public void Dispose() => _currentPage.Dispose();
}
