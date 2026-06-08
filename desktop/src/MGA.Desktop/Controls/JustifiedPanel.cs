using System;
using System.Collections.Generic;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Presenters;

namespace MGA.Desktop.Controls;

/// <summary>
/// A panel that arranges children in justified rows — similar to the Google Photos grid.
///
/// Each row fills the full available width.  All items in a row share the same height
/// (derived from <see cref="TargetRowHeight"/>); their widths vary according to each
/// item's natural aspect ratio so that the row exactly fills the available space.
/// The last (partial) row is left-aligned at the natural size and NOT stretched.
///
/// Usage in AXAML:
/// <code>
///   &lt;ItemsControl&gt;
///     &lt;ItemsControl.ItemsPanel&gt;
///       &lt;ItemsPanelTemplate&gt;
///         &lt;controls:JustifiedPanel TargetRowHeight="210" Spacing="12"/&gt;
///       &lt;/ItemsPanelTemplate&gt;
///     &lt;/ItemsControl.ItemsPanel&gt;
///     &lt;ItemsControl.ItemTemplate&gt;
///       &lt;DataTemplate&gt;
///         &lt;MyCard controls:JustifiedPanel.AspectRatio="{Binding CoverAspectRatio}"/&gt;
///       &lt;/DataTemplate&gt;
///     &lt;/ItemsControl.ItemTemplate&gt;
///   &lt;/ItemsControl&gt;
/// </code>
///
/// The panel reads <see cref="AspectRatioProperty"/> from each direct child.  When the
/// direct child is a <see cref="ContentPresenter"/> (as in <c>ItemsControl</c>), it
/// also looks at the rendered <c>ContentPresenter.Child</c> so the property can be
/// placed on the DataTemplate root element instead of the item container.
/// </summary>
public sealed class JustifiedPanel : Panel
{
    // ---------------------------------------------------------------------------
    // Styled properties
    // ---------------------------------------------------------------------------

    /// <summary>Target row height in device-independent pixels.
    /// Actual row heights may vary slightly to fill each row.</summary>
    public static readonly StyledProperty<double> TargetRowHeightProperty =
        AvaloniaProperty.Register<JustifiedPanel, double>(nameof(TargetRowHeight), 210.0);

    /// <summary>Uniform gap between items (horizontal) and between rows (vertical).</summary>
    public static readonly StyledProperty<double> SpacingProperty =
        AvaloniaProperty.Register<JustifiedPanel, double>(nameof(Spacing), 12.0);

    /// <summary>
    /// Maximum allowed scale factor when justifying a row.
    /// Rows that would require a larger scale (e.g. a single very-narrow item) are
    /// left-aligned at their natural size instead of being stretched.
    /// </summary>
    public static readonly StyledProperty<double> MaxScaleFactorProperty =
        AvaloniaProperty.Register<JustifiedPanel, double>(nameof(MaxScaleFactor), 1.5);

    public double TargetRowHeight
    {
        get => GetValue(TargetRowHeightProperty);
        set => SetValue(TargetRowHeightProperty, value);
    }

    public double Spacing
    {
        get => GetValue(SpacingProperty);
        set => SetValue(SpacingProperty, value);
    }

    public double MaxScaleFactor
    {
        get => GetValue(MaxScaleFactorProperty);
        set => SetValue(MaxScaleFactorProperty, value);
    }

    // ---------------------------------------------------------------------------
    // Attached property — set on each DataTemplate root element
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Natural aspect ratio of the item (width ÷ height).  Set this on each child
    /// or DataTemplate root element.  0 means "use the default" (2/3 portrait cover).
    /// </summary>
    public static readonly AttachedProperty<double> AspectRatioProperty =
        AvaloniaProperty.RegisterAttached<JustifiedPanel, Control, double>("AspectRatio", 0.0);

    public static double GetAspectRatio(Control control) => control.GetValue(AspectRatioProperty);
    public static void   SetAspectRatio(Control control, double value) => control.SetValue(AspectRatioProperty, value);

    // ---------------------------------------------------------------------------
    // Static ctor — re-layout when styled properties change
    // ---------------------------------------------------------------------------

    static JustifiedPanel()
    {
        AffectsMeasure<JustifiedPanel>(TargetRowHeightProperty, SpacingProperty, MaxScaleFactorProperty);
        AffectsArrange<JustifiedPanel>(TargetRowHeightProperty, SpacingProperty, MaxScaleFactorProperty);
    }

    // ---------------------------------------------------------------------------
    // Layout state (Measure → Arrange)
    // ---------------------------------------------------------------------------

    private readonly record struct RowSpec(int Start, int Count, double Height, double[] Widths);
    private readonly List<RowSpec> _rows = [];

    // ---------------------------------------------------------------------------
    // Helpers
    // ---------------------------------------------------------------------------

    private const double DefaultAspectRatio = 2.0 / 3.0; // Portrait game cover

    /// <summary>
    /// Reads the aspect ratio from <paramref name="child"/>.
    /// Falls through to <c>ContentPresenter.Child</c> so that the property
    /// can be placed on the DataTemplate root (e.g. a Grid or Button).
    /// </summary>
    private static double ResolveAspectRatio(Control child)
    {
        // Direct value on the child (e.g. set via Style or explicitly).
        var ar = child.GetValue(AspectRatioProperty);
        if (ar > 0) return ar;

        // ItemsControl wraps DataTemplate content in a ContentPresenter.
        // Look at the rendered child of that ContentPresenter.
        if (child is ContentPresenter { Child: Control inner })
        {
            var innerAr = inner.GetValue(AspectRatioProperty);
            if (innerAr > 0) return innerAr;
        }

        return DefaultAspectRatio;
    }

    // ---------------------------------------------------------------------------
    // Measure
    // ---------------------------------------------------------------------------

    protected override Size MeasureOverride(Size availableSize)
    {
        _rows.Clear();

        var children  = Children;
        if (children.Count == 0) return default;

        var targetH  = TargetRowHeight;
        var spacing  = Spacing;
        var maxScale = MaxScaleFactor;

        // Use a sensible fallback when width is unbounded (e.g. inside a horizontal ScrollViewer).
        var available = double.IsInfinity(availableSize.Width) ? 800.0 : availableSize.Width;

        // Natural widths at targetH for each child.
        var natural = new double[children.Count];
        for (var i = 0; i < children.Count; i++)
            natural[i] = targetH * ResolveAspectRatio(children[i]);

        // Greedy row packing.
        var idx = 0;
        while (idx < children.Count)
        {
            var rowStart   = idx;
            var naturalSum = 0.0;

            // Fill this row greedily.
            while (idx < children.Count)
            {
                var gap       = idx > rowStart ? spacing : 0.0;
                var tentative = naturalSum + gap + natural[idx];
                if (tentative > available && idx > rowStart) break;
                naturalSum += natural[idx];
                idx++;
            }

            var count     = idx - rowStart;
            var isLastRow = idx >= children.Count;
            var totalGap  = (count - 1) * spacing;

            // Decide widths and height for this row.
            var widths = new double[count];
            double rowH;

            if (!isLastRow && naturalSum > 0)
            {
                var scale = (available - totalGap) / naturalSum;

                if (scale <= maxScale)
                {
                    // Justify: scale every item so the row fills exactly.
                    for (var i = 0; i < count; i++)
                        widths[i] = natural[rowStart + i] * scale;
                    rowH = targetH * scale;
                }
                else
                {
                    // Scale would be too aggressive (e.g. single wide item) — left-align instead.
                    for (var i = 0; i < count; i++)
                        widths[i] = natural[rowStart + i];
                    rowH = targetH;
                }
            }
            else
            {
                // Last row: natural sizes, left-aligned.
                for (var i = 0; i < count; i++)
                    widths[i] = natural[rowStart + i];
                rowH = targetH;
            }

            // Measure each child at its computed slot size.
            for (var i = 0; i < count; i++)
                children[rowStart + i].Measure(new Size(widths[i], rowH));

            _rows.Add(new RowSpec(rowStart, count, rowH, widths));
        }

        // Total height: sum of row heights + row spacing.
        var totalH = 0.0;
        for (var r = 0; r < _rows.Count; r++)
            totalH += _rows[r].Height + (r < _rows.Count - 1 ? spacing : 0.0);

        return new Size(available, totalH);
    }

    // ---------------------------------------------------------------------------
    // Arrange
    // ---------------------------------------------------------------------------

    protected override Size ArrangeOverride(Size finalSize)
    {
        var children = Children;
        var spacing  = Spacing;
        var y        = 0.0;

        foreach (var row in _rows)
        {
            var x = 0.0;
            for (var i = 0; i < row.Count; i++)
            {
                children[row.Start + i].Arrange(new Rect(x, y, row.Widths[i], row.Height));
                x += row.Widths[i] + spacing;
            }
            y += row.Height + spacing;
        }

        return finalSize;
    }
}
