using System.Reflection;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// About page — version, build date, and open-source licenses.
/// </summary>
public sealed class AboutViewModel : ViewModelBase
{
    public string Version   { get; }
    public string BuildDate { get; }

    public AboutViewModel()
    {
        // Pull the informational version from the assembly attribute if available.
        var asm = Assembly.GetExecutingAssembly();
        Version   = asm.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion
                    ?? asm.GetName().Version?.ToString()
                    ?? "0.0.0";
        BuildDate = new DateTimeOffset(File.GetLastWriteTimeUtc(asm.Location)).ToString("yyyy-MM-dd");
    }
}
