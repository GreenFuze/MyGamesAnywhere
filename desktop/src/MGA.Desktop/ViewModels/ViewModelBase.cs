using CommunityToolkit.Mvvm.ComponentModel;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Base class for all page and control ViewModels.
///
/// Inherits ObservableObject from CommunityToolkit.Mvvm for [ObservableProperty]
/// and [RelayCommand] source-generator support.
///
/// Holds a CompositeDisposable for Rx subscriptions so every ViewModel cleans up
/// its SSE listeners when the page is navigated away from (RAII).
/// </summary>
public abstract class ViewModelBase : ObservableObject, IDisposable
{
    protected readonly System.Reactive.Disposables.CompositeDisposable Disposables = new();

    public virtual void Dispose()
    {
        Disposables.Dispose();
    }
}
