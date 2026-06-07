using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.VisualTree;
using MGA.Desktop.ViewModels;
using MGA.Desktop.ViewModels.Settings;

namespace MGA.Desktop.Views;

public partial class SettingsView : UserControl
{
    public SettingsView() => InitializeComponent();

    // ---------------------------------------------------------------------------
    // Visual-tree lifecycle — inject StorageProvider + wire drag-and-drop
    // ---------------------------------------------------------------------------

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);

        // Inject the StorageProvider so the Browse Directory commands can open folder pickers.
        if (DataContext is SettingsViewModel settingsVm)
        {
            settingsVm.Emulators.StorageProvider =
                TopLevel.GetTopLevel(this)?.StorageProvider;
        }

        // Wire the drag-and-drop handlers onto the BIOS drop zone.
        var dropZone = this.FindControl<Border>("BiosDropZone");
        if (dropZone is not null)
        {
            dropZone.AddHandler(DragDrop.DropEvent,     OnBiosDrop,     handledEventsToo: false);
            dropZone.AddHandler(DragDrop.DragOverEvent, OnBiosDragOver, handledEventsToo: false);
        }
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnDetachedFromVisualTree(e);

        // Remove handlers to avoid memory leaks.
        var dropZone = this.FindControl<Border>("BiosDropZone");
        if (dropZone is not null)
        {
            dropZone.RemoveHandler(DragDrop.DropEvent,     OnBiosDrop);
            dropZone.RemoveHandler(DragDrop.DragOverEvent, OnBiosDragOver);
        }
    }

    // ---------------------------------------------------------------------------
    // Drag-and-drop handlers  (Avalonia 12: e.DataTransfer, not e.Data)
    // ---------------------------------------------------------------------------

    private static void OnBiosDragOver(object? sender, DragEventArgs e)
    {
        // In Avalonia 12, file availability is checked via DataFormat.File.
        var hasFiles = e.DataTransfer.Contains(DataFormat.File)
                    || e.DataTransfer.TryGetFiles()?.Length > 0;

        e.DragEffects = hasFiles ? DragDropEffects.Copy : DragDropEffects.None;
        e.Handled     = true;
    }

    private void OnBiosDrop(object? sender, DragEventArgs e)
    {
        // Collect all dropped files. Fall back to single-file API if needed.
        var files = e.DataTransfer.TryGetFiles() ?? [];

        if (files.Length == 0)
        {
            var single = e.DataTransfer.TryGetFile();
            if (single is not null) files = [single];
        }

        if (files.Length == 0) return;

        // Only process the first dropped file.
        var filePath = files[0].Path.LocalPath;
        if (string.IsNullOrEmpty(filePath)) return;

        // Forward to the ViewModel; fire-and-forget on the UI thread.
        if (DataContext is SettingsViewModel settingsVm)
        {
            _ = settingsVm.Emulators.HandleBiosDropAsync(filePath);
        }

        e.Handled = true;
    }
}
