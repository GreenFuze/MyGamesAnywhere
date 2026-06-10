using System;
using System.Collections.Generic;
using System.Linq;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Presenters;
using Avalonia.Controls.Primitives;
using Avalonia.VisualTree;

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

    /// <summary>
    /// Minimum allowed scale factor when justifying a row.
    /// When a greedy row would require scaling below this threshold (because items already
    /// overflow the available width), the panel removes items from the tail of the row
    /// until the scale is acceptable — preventing rows from becoming too short.
    /// Always honours at least 1 item per row regardless of this value.
    ///
    /// Default 0.8 means rows will never be shorter than 80 % of
    /// <see cref="TargetRowHeight"/> — matching the Google Photos heuristic.
    /// </summary>
    public static readonly StyledProperty<double> MinScaleFactorProperty =
        AvaloniaProperty.Register<JustifiedPanel, double>(nameof(MinScaleFactor), 0.8);

    /// <summary>
    /// Minimum effective aspect ratio applied when computing each item's natural width.
    /// Items whose <see cref="AspectRatioProperty"/> is below this value will be treated
    /// as if they had this ratio for layout purposes — preventing portrait covers from
    /// producing extremely narrow card cells.
    ///
    /// The actual image is displayed with <c>Stretch="Uniform"</c> and centered inside
    /// the wider cell; the card background fills any letterbox gap.
    ///
    /// Default 1.0 (square).  Set to e.g. 1.3 in AXAML to match the natural width of
    /// ~4:3 banner covers so all cards appear roughly the same size.
    /// </summary>
    public static readonly StyledProperty<double> MinAspectRatioProperty =
        AvaloniaProperty.Register<JustifiedPanel, double>(nameof(MinAspectRatio), 1.0);

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

    public double MinScaleFactor
    {
        get => GetValue(MinScaleFactorProperty);
        set => SetValue(MinScaleFactorProperty, value);
    }

    public double MinAspectRatio
    {
        get => GetValue(MinAspectRatioProperty);
        set => SetValue(MinAspectRatioProperty, value);
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
    // TrackBitmapSize — auto-correct cell aspect ratio from loaded image
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Set to <c>True</c> on an <see cref="Avalonia.Controls.Image"/>-derived control
    /// inside a <see cref="JustifiedPanel"/> grid to enable dynamic aspect-ratio correction.
    ///
    /// Once the bitmap finishes loading, the panel reads the actual pixel dimensions and
    /// updates the <see cref="AspectRatioProperty"/> on the nearest enclosing
    /// <see cref="Grid"/> with the CSS class <c>"justified-cell"</c>, then invalidates
    /// its own measure so cells reflow to the true image proportions.
    ///
    /// This is the Google-Photos approach: portrait covers stay portrait; landscape
    /// headers (e.g. Steam banners) automatically get the wider cell they deserve.
    /// </summary>
    public static readonly AttachedProperty<bool> TrackBitmapSizeProperty =
        AvaloniaProperty.RegisterAttached<JustifiedPanel, Control, bool>("TrackBitmapSize", false);

    public static bool GetTrackBitmapSize(Control control) => control.GetValue(TrackBitmapSizeProperty);
    public static void SetTrackBitmapSize(Control control, bool value) => control.SetValue(TrackBitmapSizeProperty, value);

    // ---------------------------------------------------------------------------
    // Static ctor — re-layout when styled properties change
    // ---------------------------------------------------------------------------

    static JustifiedPanel()
    {
        AffectsMeasure<JustifiedPanel>(TargetRowHeightProperty, SpacingProperty, MaxScaleFactorProperty, MinScaleFactorProperty, MinAspectRatioProperty);
        AffectsArrange<JustifiedPanel>(TargetRowHeightProperty, SpacingProperty, MaxScaleFactorProperty, MinScaleFactorProperty, MinAspectRatioProperty);

        // When TrackBitmapSize is set on any Control, detect the loaded bitmap and push
        // the true aspect ratio back up to the enclosing justified-cell Grid.
        // Note: AdvancedImage (AsyncImageLoader) is a TemplatedControl, NOT an Image subclass,
        // so we register on Control and find the inner Image part via the visual tree.
        TrackBitmapSizeProperty.Changed.AddClassHandler<Control>(OnTrackBitmapSizeChanged);
    }

    private static void OnTrackBitmapSizeChanged(Control control, AvaloniaPropertyChangedEventArgs e)
    {
        if (e.NewValue is not true) return;

        if (control is Image directImage)
        {
            // Direct Image subclass: subscribe immediately.
            SubscribeToImageSource(directImage, control);
            return;
        }

        // TemplatedControl (e.g. AdvancedImage): the actual Image lives inside the template.
        // We must wait for TemplateApplied before we can walk into the visual descendants.
        if (control is TemplatedControl tc)
        {
            var attached = false;

            void TryAttach()
            {
                if (attached) return;
                var img = tc.GetVisualDescendants().OfType<Image>().FirstOrDefault();
                if (img is null) return;
                attached = true;
                SubscribeToImageSource(img, control);
            }

            // Try immediately — template may already be applied (e.g. from cache/recycling).
            TryAttach();

            // Guard: only subscribe to TemplateApplied if we haven't found the part yet.
            if (!attached)
                tc.TemplateApplied += (_, _) => TryAttach();
        }
    }

    /// <summary>
    /// Subscribe to <paramref name="img"/>'s <see cref="Image.SourceProperty"/> and, when a
    /// non-trivial bitmap is set, update the nearest <c>justified-cell</c> Grid's
    /// <see cref="AspectRatioProperty"/> and invalidate the enclosing <see cref="JustifiedPanel"/>.
    /// <para>
    /// <paramref name="trackTarget"/> is the element whose visual ancestors are walked
    /// (the <c>Image</c> or <c>AdvancedImage</c> that owns TrackBitmapSize=True).
    /// </para>
    /// </summary>
    private static void SubscribeToImageSource(Image img, Control trackTarget)
    {
        img.GetObservable(Image.SourceProperty).Subscribe(bitmap =>
        {
            if (bitmap is null) return;
            var size = bitmap.Size;
            if (size is not { Width: > 0, Height: > 0 }) return;

            var ratio = size.Width / size.Height;

            // Update the cell for any image that is wider than tall (landscape or square).
            // Threshold 1.0: skip only strictly portrait images so they don't override
            // the server-reported or MinAspectRatio-clamped initial value with a smaller one.
            // Portrait covers (< 1.0) stay at whatever initial ratio the server reported.
            if (ratio < 1.0) return;

            // Walk up from the Image to the DataTemplate root Grid tagged "justified-cell".
            var cell = trackTarget.GetVisualAncestors()
                                  .OfType<Grid>()
                                  .FirstOrDefault(g => g.Classes.Contains("justified-cell"));
            if (cell is null) return;

            // Overwrite the initial (server-reported or default 2:3) aspect ratio with the
            // actual pixel ratio of the loaded bitmap.
            SetAspectRatio(cell, ratio);

            // Invalidate the JustifiedPanel so it re-measures at the corrected proportions.
            cell.GetVisualAncestors()
                .OfType<JustifiedPanel>()
                .FirstOrDefault()
                ?.InvalidateMeasure();
        });
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
        var minScale = MinScaleFactor;
        var minAr    = MinAspectRatio;

        // Use a sensible fallback when width is unbounded (e.g. inside a horizontal ScrollViewer).
        var available = double.IsInfinity(availableSize.Width) ? 800.0 : availableSize.Width;

        // Natural widths at targetH for each child.
        // MinAspectRatio is clamped so portrait covers never produce cells narrower than
        // minAr × targetH — the image is centered (Stretch=Uniform) inside the wider cell.
        var natural = new double[children.Count];
        for (var i = 0; i < children.Count; i++)
            natural[i] = targetH * Math.Max(ResolveAspectRatio(children[i]), minAr);

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

            // Trim from tail while the row would require too much downscaling.
            // This prevents rows that mix many landscape + portrait items from becoming too short.
            // We always allow at least 1 item per row regardless of how wide it is.
            var count = idx - rowStart;
            while (count > 1)
            {
                var gapCheck   = (count - 1) * spacing;
                var scaleCheck = (available - gapCheck) / naturalSum;
                if (scaleCheck >= minScale) break;

                // Move last item of this row to the next row.
                idx--;
                naturalSum -= natural[idx];
                count--;
            }

            var isLastRow = idx >= children.Count;
            var totalGap  = (count - 1) * spacing;

            // Decide widths for this row.
            // Height is ALWAYS targetH — only widths scale to fill.
            // This guarantees every row is the same height regardless of aspect-ratio mix,
            // which is what the user expects ("all cards same size, some just wider").
            var widths = new double[count];
            var rowH   = targetH;   // Fixed — never changes due to scale.

            if (!isLastRow && naturalSum > 0)
            {
                var scale = (available - totalGap) / naturalSum;

                if (scale <= maxScale)
                {
                    // Justify widths so the row fills the available width exactly.
                    // Heights stay at targetH (only widths change, slight aspect-ratio distortion
                    // bounded by minScale ↔ maxScale — typically < 20 % in practice).
                    for (var i = 0; i < count; i++)
                        widths[i] = natural[rowStart + i] * scale;
                }
                else
                {
                    // Scale would be too aggressive (e.g. single narrow item) — left-align.
                    for (var i = 0; i < count; i++)
                        widths[i] = natural[rowStart + i];
                }
            }
            else
            {
                // Last row: natural sizes, left-aligned.
                for (var i = 0; i < count; i++)
                    widths[i] = natural[rowStart + i];
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
