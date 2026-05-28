using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;

namespace MGA.Desktop.Controls;

/// <summary>
/// Custom immersive title bar — logo, search box, server URL, drag region.
/// Chrome buttons are in MainWindow (not here) so they always render at the right edge.
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

        var window = TopLevel.GetTopLevel(this) as Window;
        if (window is null) return;

        // ── Drag region ────────────────────────────────────────────
        var drag = this.FindControl<Border>("DragRegion")!;
        drag.PointerPressed += (_, args) =>
        {
            if (args.GetCurrentPoint(this).Properties.IsLeftButtonPressed)
                window.BeginMoveDrag(args);
        };

        // Double-click toggles maximize.
        drag.DoubleTapped += (_, _) =>
            window.WindowState = window.WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;

        // ── Chrome buttons ─────────────────────────────────────────
        var minimize = this.FindControl<Button>("MinimizeButton");
        var maximize = this.FindControl<Button>("MaximizeButton");
        var close    = this.FindControl<Button>("CloseButton");

        if (minimize is not null)
            minimize.Click += (_, _) => window.WindowState = WindowState.Minimized;

        if (maximize is not null)
            maximize.Click += (_, _) =>
                window.WindowState = window.WindowState == WindowState.Maximized
                    ? WindowState.Normal
                    : WindowState.Maximized;

        if (close is not null)
            close.Click += (_, _) => window.Close();
    }
}
