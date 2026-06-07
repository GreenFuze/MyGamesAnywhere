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
    /// </summary>
    internal static string GetBadgeColor(string? slug) =>
        (slug ?? string.Empty).ToLowerInvariant() switch
        {
            // PC / Desktop — slate
            "windows_pc" or "pc" or "windows" or "mac" or "macos" or "linux"
                or "dos" or "ms_dos" or "msdos" or "ms-dos" or "scummvm"       => "#334155",
            // PlayStation — dark blue
            "ps1" or "psx" or "playstation" or "ps2" or "playstation2"
                or "ps3" or "playstation3" or "ps4" or "playstation4"
                or "ps5" or "playstation5" or "psp" or "playstation_portable"
                or "vita" or "psvita" or "playstation_vita"                     => "#003087",
            // Nintendo — red
            "nes" or "nintendo" or "snes" or "super_nintendo" or "n64"
                or "nintendo_64" or "gamecube" or "gc" or "wii" or "wiiu"
                or "wii_u" or "switch" or "nintendo_switch"
                or "gb" or "game_boy" or "gbc" or "gba" or "ds" or "3ds"       => "#9D0C0C",
            // Xbox — green
            "xbox" or "xbox_360" or "xbox360" or "xbox_one" or "xboxone"
                or "xbox_series" or "xboxseries"                                => "#107C10",
            // Sega — blue
            "genesis" or "megadrive" or "mega_drive" or "sega_cd" or "segacd"
                or "saturn" or "sega_saturn" or "dreamcast" or "dc"
                or "game_gear" or "gamegear"                                    => "#0A4499",
            // Arcade — purple
            "arcade" or "mame"                                                  => "#5B21B6",
            // Fallback
            _                                                                   => "#334155",
        };
}
