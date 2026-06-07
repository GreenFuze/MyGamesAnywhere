using Avalonia.Animation;
using Avalonia.Animation.Easings;
using Avalonia.Controls;
using Avalonia.Input;
using MGA.Desktop.Controls;
using MGA.Desktop.ViewModels;

namespace MGA.Desktop.Views;

/// <summary>
/// Main application window.
///
/// Responsibilities:
/// - Wires the ToastHost to the ToastService collection.
/// - Animates the sidebar width when SidebarCollapsed changes.
/// </summary>
public partial class MainWindow : Window
{
    private const double SidebarFullWidth     = 220;
    private const double SidebarIconOnlyWidth = 56;

    public MainWindow()
    {
        InitializeComponent();
        // Chrome button wiring lives in TitleBar.axaml.cs (OnAttachedToVisualTree).
    }

    protected override void OnDataContextChanged(EventArgs e)
    {
        base.OnDataContextChanged(e);

        if (DataContext is not MainWindowViewModel vm) return;

        // ── Toast wiring ───────────────────────────────────────────
        var toastHost = this.FindControl<ToastHost>("ToastHostControl")!;
        var app       = (App)global::Avalonia.Application.Current!;
        toastHost.ToastsSource = app.ToastService?.Toasts;

        // ── Sidebar collapse animation ─────────────────────────────
        var sidebar = this.FindControl<Sidebar>("SidebarControl")!;
        sidebar.Width = vm.SidebarCollapsed ? SidebarIconOnlyWidth : SidebarFullWidth;

        vm.PropertyChanged += (_, args) =>
        {
            if (args.PropertyName == nameof(MainWindowViewModel.SidebarCollapsed))
                AnimateSidebarWidth(sidebar,
                    vm.SidebarCollapsed ? SidebarIconOnlyWidth : SidebarFullWidth);
        };
    }

    // ---------------------------------------------------------------------------
    // Mouse back-button (XButton1) — navigate back
    // ---------------------------------------------------------------------------

    protected override void OnPointerPressed(PointerPressedEventArgs e)
    {
        base.OnPointerPressed(e);

        // XButton1 is the mouse "back" thumb button (button 4 on most mice).
        if (e.GetCurrentPoint(null).Properties.PointerUpdateKind == PointerUpdateKind.XButton1Pressed
            && DataContext is MainWindowViewModel vm
            && vm.Nav.CanGoBack)
        {
            vm.Nav.NavigateBack();
            e.Handled = true;
        }
    }

    // ---------------------------------------------------------------------------
    // Sidebar animation
    // ---------------------------------------------------------------------------

    private static void AnimateSidebarWidth(Control sidebar, double targetWidth)
    {
        sidebar.Transitions ??= new Avalonia.Animation.Transitions();
        sidebar.Transitions.Clear();
        sidebar.Transitions.Add(new DoubleTransition
        {
            Property = WidthProperty,
            Duration  = TimeSpan.FromMilliseconds(220),
            Easing    = new CubicEaseInOut(),
        });
        sidebar.Width = targetWidth;
    }
}
