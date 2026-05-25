using Avalonia.Controls;

namespace MGA.Desktop.Controls;

/// <summary>
/// Collapsible sidebar navigation.
/// Width is driven by the parent Grid's column definition which is animated
/// by MainWindow code-behind when SidebarCollapsed changes.
/// </summary>
public partial class Sidebar : UserControl
{
    public Sidebar()
    {
        InitializeComponent();
    }
}
