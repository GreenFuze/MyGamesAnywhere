using System.Reflection;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Desktop.Services.Emulation;

/// <summary>
/// Loads the static emulator catalog from <c>Assets/Data/emulators.json</c>,
/// embedded in the assembly as an <c>EmbeddedResource</c>, and exposes it as
/// a queryable, read-only collection.
///
/// RAII: catalog is loaded once in the constructor; no I/O after construction.
/// Fail-fast: throws <see cref="InvalidOperationException"/> if the embedded
/// resource is missing or malformed — this is a build-time defect, not a runtime error.
/// </summary>
public sealed class EmulatorCatalogService
{
    // Logical resource name: assembly-default-namespace + folder path (dots) + filename.
    private const string ResourceName = "MGA.Desktop.Assets.Data.emulators.json";

    private static readonly JsonSerializerOptions s_opts = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private readonly IReadOnlyList<EmulatorCatalogEntry> _catalog;

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public EmulatorCatalogService()
    {
        _catalog = LoadCatalog();
    }

    // ---------------------------------------------------------------------------
    // Public API
    // ---------------------------------------------------------------------------

    /// <summary>All emulator entries in the catalog, in definition order.</summary>
    public IReadOnlyList<EmulatorCatalogEntry> All => _catalog;

    /// <summary>
    /// Returns the catalog entry with the given ID (case-insensitive), or null.
    /// </summary>
    public EmulatorCatalogEntry? GetById(string id) =>
        _catalog.FirstOrDefault(e => string.Equals(e.Id, id, StringComparison.OrdinalIgnoreCase));

    /// <summary>
    /// Returns all catalog entries that support the specified platform
    /// (matches server-side Platform strings such as "ps1", "snes", "ms_dos").
    /// </summary>
    public IReadOnlyList<EmulatorCatalogEntry> GetForPlatform(string platform) =>
        _catalog.Where(e => e.SupportsPlatform(platform)).ToList();

    // ---------------------------------------------------------------------------
    // Private
    // ---------------------------------------------------------------------------

    private static IReadOnlyList<EmulatorCatalogEntry> LoadCatalog()
    {
        var asm = typeof(EmulatorCatalogService).Assembly;

        using var stream = asm.GetManifestResourceStream(ResourceName)
            ?? throw new InvalidOperationException(
                $"Embedded emulator catalog not found: '{ResourceName}'. " +
                "Ensure Assets/Data/emulators.json is marked as EmbeddedResource in the csproj.");

        var doc = JsonSerializer.Deserialize<CatalogDocument>(stream, s_opts)
            ?? throw new InvalidOperationException("emulators.json deserialized to null.");

        if (doc.Emulators is null || doc.Emulators.Count == 0)
            throw new InvalidOperationException("emulators.json contains no emulator entries.");

        return doc.Emulators;
    }

    private sealed class CatalogDocument
    {
        [JsonPropertyName("emulators")]
        public List<EmulatorCatalogEntry>? Emulators { get; set; }
    }
}
