namespace MGA.Desktop.ViewModels;

/// <summary>
/// Shared platform display utilities — mapping raw API platform slugs to
/// human-readable names and accent colors.
/// Used by <see cref="GameCardModel"/> and <see cref="GameDetailViewModel"/>.
/// </summary>
internal static class PlatformHelper
{
    // ---------------------------------------------------------------------------
    // Slug → display name
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Maps raw API platform slugs to short, human-readable display names.
    /// Unknown slugs are title-cased and underscores replaced with spaces.
    /// </summary>
    internal static string FormatPlatform(string? slug) =>
        (slug ?? string.Empty).ToLowerInvariant() switch
        {
            // PC / desktop
            "windows_pc" or "pc" or "windows"          => "PC",
            "mac"        or "macos"                     => "Mac",
            "linux"                                     => "Linux",
            "dos" or "ms_dos" or "msdos" or "ms-dos"   => "DOS",
            "mr_dos" or "mrdos" or "mr-dos"             => "DOS",
            "scummvm"                                   => "ScummVM",

            // Nintendo
            "nes"       or "nintendo"                   => "NES",
            "snes"      or "super_nintendo"             => "SNES",
            "n64"       or "nintendo_64"                => "N64",
            "gamecube"  or "gc"                         => "GameCube",
            "wii"                                       => "Wii",
            "wiiu"      or "wii_u"                      => "Wii U",
            "switch"    or "nintendo_switch"            => "Switch",
            "gb"        or "game_boy"                   => "Game Boy",
            "gbc"       or "game_boy_color"             => "GBC",
            "gba"       or "game_boy_advance"           => "GBA",
            "ds"        or "nintendo_ds"                => "DS",
            "3ds"       or "nintendo_3ds"               => "3DS",

            // Sega
            "genesis"   or "megadrive" or "mega_drive"  => "Genesis",
            "sega_cd"   or "segacd"                     => "Sega CD",
            "saturn"    or "sega_saturn"                => "Saturn",
            "dreamcast" or "dc"                         => "Dreamcast",
            "game_gear" or "gamegear"                   => "Game Gear",

            // Sony
            "ps1"  or "psx"        or "playstation"    => "PS1",
            "ps2"  or "playstation2"                   => "PS2",
            "ps3"  or "playstation3"                   => "PS3",
            "ps4"  or "playstation4"                   => "PS4",
            "ps5"  or "playstation5"                   => "PS5",
            "psp"  or "playstation_portable"           => "PSP",
            "vita" or "psvita" or "playstation_vita"   => "PS Vita",

            // Microsoft
            "xbox"                                      => "Xbox",
            "xbox_360"    or "xbox360"                  => "Xbox 360",
            "xbox_one"    or "xboxone"                  => "Xbox One",
            "xbox_series" or "xboxseries"               => "Xbox Series",

            // Arcade / other
            "arcade" or "mame"                          => "Arcade",
            "atari_2600" or "atari2600"                 => "Atari 2600",
            "atari_7800" or "atari7800"                 => "Atari 7800",
            "turbografx" or "tg16"                      => "TG-16",

            // Fallback: title-case and replace underscores
            "" => string.Empty,
            var other => System.Globalization.CultureInfo.CurrentCulture.TextInfo
                             .ToTitleCase(other.Replace('_', ' ')),
        };

    // ---------------------------------------------------------------------------
    // Slug → badge accent color
    // ---------------------------------------------------------------------------

    /// <summary>
    /// Returns a platform-specific accent color hex string for the badge chip.
    /// Colors are vibrant and distinctive so platform identity is immediately
    /// recognizable in a dense game grid.
    /// </summary>
    internal static string GetBadgeColor(string? slug) =>
        (slug ?? string.Empty).ToLowerInvariant() switch
        {
            // PC / Desktop — steel blue (Windows) / silver (Mac) / orange (Linux)
            "windows_pc" or "pc" or "windows"                                  => "#1d6fa4",
            "mac" or "macos"                                                   => "#6b7280",
            "linux"                                                             => "#ea580c",
            "dos" or "ms_dos" or "msdos" or "ms-dos" or "mr_dos"
                or "mrdos" or "mr-dos" or "scummvm"                            => "#475569",

            // PlayStation — iconic PlayStation blue
            "ps1" or "psx" or "playstation" or "ps2" or "playstation2"
                or "ps3" or "playstation3"                                      => "#1d4ed8",
            "ps4" or "playstation4" or "ps5" or "playstation5"                 => "#2563eb",
            "psp" or "playstation_portable"
                or "vita" or "psvita" or "playstation_vita"                    => "#3b82f6",

            // Nintendo — vivid red
            "nes" or "nintendo"                                                => "#dc2626",
            "snes" or "super_nintendo"                                         => "#7c3aed",  // purple (SNES branding)
            "n64" or "nintendo_64"                                             => "#c2410c",
            "gamecube" or "gc"                                                 => "#6d28d9",
            "wii"                                                              => "#e5e7eb",
            "wiiu" or "wii_u"                                                  => "#1d4ed8",
            "switch" or "nintendo_switch"                                      => "#e11d48",
            "gb" or "game_boy"                                                 => "#059669",  // original GB green
            "gbc" or "game_boy_color"                                          => "#16a34a",
            "gba" or "game_boy_advance"                                        => "#9333ea",
            "ds" or "nintendo_ds"                                              => "#0891b2",
            "3ds" or "nintendo_3ds"                                            => "#0284c7",

            // Xbox — signature green
            "xbox"                                                             => "#16a34a",
            "xbox_360" or "xbox360"                                            => "#15803d",
            "xbox_one" or "xboxone" or "xbox_series" or "xboxseries"          => "#166534",

            // Sega — bright Sega blue
            "genesis" or "megadrive" or "mega_drive"                          => "#1e40af",
            "sega_cd" or "segacd"                                              => "#1d4ed8",
            "saturn" or "sega_saturn"                                          => "#1e3a8a",
            "dreamcast" or "dc"                                                => "#0ea5e9",  // Dreamcast swirl cyan
            "game_gear" or "gamegear"                                          => "#0284c7",

            // Arcade / legacy — vivid purple
            "arcade" or "mame"                                                 => "#7c3aed",
            "atari_2600" or "atari2600"                                        => "#b45309",
            "atari_7800" or "atari7800"                                        => "#92400e",
            "turbografx" or "tg16"                                             => "#be185d",

            // Fallback — neutral slate
            _                                                                  => "#475569",
        };
}
