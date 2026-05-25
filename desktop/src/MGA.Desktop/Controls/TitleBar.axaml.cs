using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;

namespace MGA.Desktop.Controls;

/// <summary>
/// Custom immersive title bar with drag-region and native window chrome buttons.
///
/// ExtendClientAreaToDecorationsHint=true + SystemDecorations=BorderOnly on the
/// window lets us own the entire non-client area while still getting OS-level
/// window management (snap, resize, taskbar).
/// </summary>
public partial class TitleBar : UserControl
{
    public TitleBar()
    {
        InitializeComponent();
    }

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);

        // Wire up the drag region for the host Window.
        var window = TopLevel.GetTopLevel(this) as Window;
        if (window is null) return;

        // Make the bar drag the window when the user clicks anywhere except chrome buttons.
        var drag = this.FindControl<Border>("DragRegion")!;
        drag.PointerPressed += (_, args) =>
        {
            if (args.GetCurrentPoint(this).Properties.IsLeftButtonPressed)
                window.BeginMoveDrag(args);
        };

        // Wire chrome buttons.
        this.FindControl<Button>("MinimizeButton")!.Click += (_, _) =>
            window.WindowState = WindowState.Minimized;

        this.FindControl<Button>("MaximizeButton")!.Click += (_, _) =>
            window.WindowState = window.WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;

        this.FindControl<Button>("CloseButton")!.Click += (_, _) =>
            window.Close();

        // Double-click on drag region toggles maximize.
        drag.DoubleTapped += (_, _) =>
            window.WindowState = window.WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;
    }
}
