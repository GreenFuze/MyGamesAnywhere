using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using MGA.Desktop.Services;

namespace MGA.Desktop.Controls;

/// <summary>
/// Renders stacked toast notifications in the bottom-right corner.
/// Bind the ToastService.Toasts collection to this control's ToastsSource property.
/// </summary>
public partial class ToastHost : UserControl
{
    public static readonly StyledProperty<System.Collections.ObjectModel.ObservableCollection<ToastMessage>?>
        ToastsSourceProperty = AvaloniaProperty.Register<ToastHost,
            System.Collections.ObjectModel.ObservableCollection<ToastMessage>?>(nameof(ToastsSource));

    public System.Collections.ObjectModel.ObservableCollection<ToastMessage>? ToastsSource
    {
        get => GetValue(ToastsSourceProperty);
        set => SetValue(ToastsSourceProperty, value);
    }

    public ToastHost()
    {
        InitializeComponent();
    }

    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);

        if (change.Property == ToastsSourceProperty)
        {
            var list = this.FindControl<ItemsControl>("ToastList")!;
            list.ItemsSource = ToastsSource;
        }
    }
}
