using System.Text.Json.Serialization;

namespace MGA.Desktop.Services.Emulation;

/// <summary>
/// Maps a single platform to a libretro-compatible core file for use with RetroArch
/// (or any multi-core frontend).
/// </summary>
public sealed class CoreMapping
{
    [JsonPropertyName("filename")]
    public string Filename { get; set; } = string.Empty;

    [JsonPropertyName("displayName")]
    public string DisplayName { get; set; } = string.Empty;
}

/// <summary>
/// A BIOS file required (or optionally recommended) by an emulator for a specific platform.
/// </summary>
public sealed class BiosCatalogEntry
{
    [JsonPropertyName("filename")]
    public string Filename { get; set; } = string.Empty;

    /// <summary>
    /// SHA-256 hash of the file in lowercase hex.
    /// Empty string means the hash is not yet documented in the catalog —
    /// the BIOS directory checker will skip hash verification for this entry
    /// and only check presence.
    /// </summary>
    [JsonPropertyName("sha256")]
    public string Sha256 { get; set; } = string.Empty;

    [JsonPropertyName("description")]
    public string Description { get; set; } = string.Empty;

    /// <summary>
    /// If true, the emulator cannot run games for this platform without this file.
    /// If false, the file is optional (e.g. a regional BIOS variant).
    /// </summary>
    [JsonPropertyName("required")]
    public bool Required { get; set; }
}

/// <summary>
/// Static catalog entry describing a known emulator.
/// Loaded from <c>Assets/Data/emulators.json</c> embedded in the assembly.
/// Immutable after deserialization — treat as read-only.
/// </summary>
public sealed class EmulatorCatalogEntry
{
    // ---------------------------------------------------------------------------
    // Identity
    // ---------------------------------------------------------------------------

    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;

    [JsonPropertyName("displayName")]
    public string DisplayName { get; set; } = string.Empty;

    [JsonPropertyName("description")]
    public string Description { get; set; } = string.Empty;

    // ---------------------------------------------------------------------------
    // Platform support
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Platform IDs supported by this emulator.
    /// Matches the server-side Platform string values (e.g. "nes", "ps1", "ms_dos").
    /// </summary>
    [JsonPropertyName("platforms")]
    public List<string> Platforms { get; set; } = [];

    // ---------------------------------------------------------------------------
    // Legal / distribution
    // ---------------------------------------------------------------------------

    [JsonPropertyName("license")]
    public string License { get; set; } = string.Empty;

    [JsonPropertyName("websiteUrl")]
    public string WebsiteUrl { get; set; } = string.Empty;

    /// <summary>
    /// If true, MGA may automatically download and install this emulator without
    /// navigating the user to an external page.  The license must explicitly permit
    /// redistribution (e.g. GPL, MIT).
    /// </summary>
    [JsonPropertyName("canAutoInstall")]
    public bool CanAutoInstall { get; set; }

    // ---------------------------------------------------------------------------
    // Feature flags
    // ---------------------------------------------------------------------------

    /// <summary>True when this emulator supports the RetroAchievements network.</summary>
    [JsonPropertyName("supportsRetroAchievements")]
    public bool SupportsRetroAchievements { get; set; }

    // ---------------------------------------------------------------------------
    // Download
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Download page or direct installer URLs keyed by OS-arch string
    /// (e.g. <c>"windows_x64"</c>, <c>"linux_x64"</c>).
    /// </summary>
    [JsonPropertyName("downloadUrls")]
    public Dictionary<string, string> DownloadUrls { get; set; } = [];

    // ---------------------------------------------------------------------------
    // Launch configuration
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Default command-line argument template.
    /// Placeholders: {rom}, {core}, {gamedir}, {conf}.
    /// </summary>
    [JsonPropertyName("defaultArgsTemplate")]
    public string DefaultArgsTemplate { get; set; } = "{rom}";

    /// <summary>
    /// For multi-core frontends (e.g. RetroArch): maps platform ID → core info.
    /// Empty for single-system standalone emulators.
    /// </summary>
    [JsonPropertyName("coreMapping")]
    public Dictionary<string, CoreMapping> CoreMapping { get; set; } = [];

    // ---------------------------------------------------------------------------
    // BIOS
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Maps platform ID → list of required/optional BIOS files for that platform.
    /// Platforms not listed here require no BIOS files.
    /// </summary>
    [JsonPropertyName("biosRequirements")]
    public Dictionary<string, List<BiosCatalogEntry>> BiosRequirements { get; set; } = [];

    // ---------------------------------------------------------------------------
    // Query helpers
    // ---------------------------------------------------------------------------

    /// <summary>Returns BIOS requirements for a platform, or an empty list.</summary>
    public IReadOnlyList<BiosCatalogEntry> GetBiosRequirements(string platform) =>
        BiosRequirements.TryGetValue(platform, out var list) ? list : [];

    /// <summary>Returns the core mapping for a platform, or null.</summary>
    public CoreMapping? GetCore(string platform) =>
        CoreMapping.TryGetValue(platform, out var core) ? core : null;

    /// <summary>Whether this emulator supports the given platform (case-insensitive).</summary>
    public bool SupportsPlatform(string platform) =>
        Platforms.Contains(platform, StringComparer.OrdinalIgnoreCase);

    /// <summary>
    /// Returns the best download URL for this machine's OS/arch,
    /// or the first available URL as a fallback, or null if none are defined.
    /// </summary>
    public string? GetDownloadUrl()
    {
        // Detect current OS-arch key.
        string osArch = (OperatingSystem.IsWindows(), System.Runtime.InteropServices.RuntimeInformation.OSArchitecture) switch
        {
            (true,  System.Runtime.InteropServices.Architecture.X64)   => "windows_x64",
            (true,  System.Runtime.InteropServices.Architecture.Arm64) => "windows_arm64",
            (false, System.Runtime.InteropServices.Architecture.X64)   => "linux_x64",
            (false, System.Runtime.InteropServices.Architecture.Arm64) => "linux_arm64",
            _ => string.Empty,
        };

        if (!string.IsNullOrEmpty(osArch) && DownloadUrls.TryGetValue(osArch, out var url))
            return url;

        return DownloadUrls.Values.FirstOrDefault();
    }
}
