using System.Globalization;
using System.Text.Json;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Discriminated type for a config field, controlling which editor is rendered.</summary>
public enum ConfigFieldType
{
    Text,
    Password,
    Boolean,
    Number,
}

/// <summary>
/// Observable model for a single dynamic config field.
/// Rendered by the integration wizard's config fields ItemsControl.
/// </summary>
public sealed partial class ConfigFieldModel : ObservableObject
{
    // ---------------------------------------------------------------------------
    // Static metadata
    // ---------------------------------------------------------------------------

    /// <summary>Raw JSON key used as the key in the config dict (e.g. "root_path").</summary>
    public string Key { get; init; } = string.Empty;

    /// <summary>Prettified display label (e.g. "Root Path").</summary>
    public string Label { get; init; } = string.Empty;

    /// <summary>Field type controlling which editor is shown.</summary>
    public ConfigFieldType Type { get; init; } = ConfigFieldType.Text;

    /// <summary>Optional human-readable description from the schema.</summary>
    public string? Description { get; init; }

    /// <summary>Whether this field must be filled in.</summary>
    public bool IsRequired { get; init; }

    /// <summary>Whether the field value should be hidden by default (e.g. API keys).</summary>
    public bool IsSecret { get; init; }

    /// <summary>Optional link to external documentation for this field.</summary>
    public string? HelpUrl { get; init; }

    // ---------------------------------------------------------------------------
    // Mutable values
    // ---------------------------------------------------------------------------

    /// <summary>Current value for text/password/number fields. Bound to TextBox.</summary>
    [ObservableProperty]
    private string _stringValue = string.Empty;

    /// <summary>Current value for boolean fields. Bound to CheckBox.</summary>
    [ObservableProperty]
    private bool _boolValue;

    /// <summary>Whether a secret field is currently being shown in plain text.</summary>
    [ObservableProperty]
    private bool _showValue;

    // ---------------------------------------------------------------------------
    // Derived properties for AXAML binding
    // ---------------------------------------------------------------------------

    /// <summary>True when the field is a boolean (renders as CheckBox).</summary>
    public bool IsBoolean => Type == ConfigFieldType.Boolean;

    /// <summary>True when the field is a non-secret text/number field (renders as plain TextBox).</summary>
    public bool IsTextNotSecret => Type != ConfigFieldType.Boolean && !IsSecret;

    /// <summary>True when a secret field is currently masked.</summary>
    public bool IsHidden => IsSecret && !ShowValue;

    // ---------------------------------------------------------------------------
    // Property change hooks
    // ---------------------------------------------------------------------------

    partial void OnShowValueChanged(bool value)
    {
        OnPropertyChanged(nameof(IsHidden));
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    /// <summary>Toggles plain-text visibility of a secret field.</summary>
    [RelayCommand]
    private void ToggleShowValue()
    {
        ShowValue = !ShowValue;
    }

    // ---------------------------------------------------------------------------
    // Factory
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Creates a ConfigFieldModel from a schema entry and an optional current value.
    /// </summary>
    /// <param name="key">Field key from the schema map.</param>
    /// <param name="schemaDef">JsonElement describing the field (type, required, description, etc.).</param>
    /// <param name="currentValue">Optional existing value to pre-populate the field.</param>
    public static ConfigFieldModel FromSchema(string key, JsonElement schemaDef, JsonElement? currentValue)
    {
        // Read optional schema properties.
        var typeStr     = ReadString(schemaDef, "type");
        var description = ReadString(schemaDef, "description");
        var helpUrl     = ReadString(schemaDef, "x-help-url");
        var isRequired  = ReadBool(schemaDef, "required");
        var isSecret    = ReadBool(schemaDef, "x-secret");

        var fieldType = typeStr switch
        {
            "boolean" => ConfigFieldType.Boolean,
            "number"  => ConfigFieldType.Number,
            _ when isSecret => ConfigFieldType.Password,
            _ => ConfigFieldType.Text,
        };

        var model = new ConfigFieldModel
        {
            Key         = key,
            Label       = PrettifyKey(key),
            Type        = fieldType,
            Description = string.IsNullOrEmpty(description) ? null : description,
            HelpUrl     = string.IsNullOrEmpty(helpUrl)     ? null : helpUrl,
            IsRequired  = isRequired,
            IsSecret    = isSecret,
        };

        // Pre-populate value from existing config.
        if (currentValue.HasValue)
        {
            var v = currentValue.Value;
            switch (fieldType)
            {
                case ConfigFieldType.Boolean:
                    model.BoolValue = v.ValueKind == JsonValueKind.True;
                    break;

                case ConfigFieldType.Number:
                    model.StringValue = v.ValueKind == JsonValueKind.Number
                        ? v.GetRawText()
                        : (v.GetString() ?? string.Empty);
                    break;

                default:
                    model.StringValue = v.ValueKind == JsonValueKind.String
                        ? (v.GetString() ?? string.Empty)
                        : v.GetRawText();
                    break;
            }
        }

        return model;
    }

    // ---------------------------------------------------------------------------
    // Private helpers
    // ---------------------------------------------------------------------------

    private static string? ReadString(JsonElement el, string key) =>
        el.TryGetProperty(key, out var v) && v.ValueKind == JsonValueKind.String
            ? v.GetString()
            : null;

    private static bool ReadBool(JsonElement el, string key) =>
        el.TryGetProperty(key, out var v) && v.ValueKind == JsonValueKind.True;

    /// <summary>
    /// Converts a snake_case or kebab-case key into a Title Case label.
    /// E.g. "root_path" → "Root Path", "api-key" → "Api Key".
    /// </summary>
    private static string PrettifyKey(string key)
    {
        var words = key.Replace('-', ' ').Replace('_', ' ').Split(' ');
        var ti    = CultureInfo.CurrentCulture.TextInfo;
        return string.Join(' ', words.Select(w => ti.ToTitleCase(w)));
    }
}
