package core

import "strings"

type platformAliasRule struct {
	platform Platform
	aliases  []string
}

var platformAliasRules = []platformAliasRule{
	{platform: PlatformWindowsPC, aliases: []string{"windows_pc", "windows", "pc"}},
	{platform: PlatformMSDOS, aliases: []string{"ms_dos", "ms dos", "ms-dos", "msdos", "dos"}},
	{platform: PlatformArcade, aliases: []string{"arcade", "mame"}},
	{platform: PlatformNES, aliases: []string{"nes", "nintendo entertainment system"}},
	{platform: PlatformSNES, aliases: []string{"snes", "super nintendo", "super nintendo entertainment system"}},
	{platform: PlatformGB, aliases: []string{"gb", "game boy", "nintendo game boy"}},
	{platform: PlatformGBC, aliases: []string{"gbc", "game boy color", "nintendo game boy color"}},
	{platform: PlatformGBA, aliases: []string{"gba", "game boy advance", "game boy advanced", "nintendo game boy advance"}},
	{platform: PlatformN64, aliases: []string{"n64", "nintendo 64", "nintendo64"}},
	{platform: PlatformGenesis, aliases: []string{"genesis", "sega genesis", "mega drive", "megadrive", "sega mega drive"}},
	{platform: PlatformSegaMasterSystem, aliases: []string{"sega_master_system", "master system", "sega master system", "sms", "mastersystem"}},
	{platform: PlatformGameGear, aliases: []string{"game_gear", "game gear", "sega game gear", "gamegear"}},
	{platform: PlatformSegaCD, aliases: []string{"sega_cd", "sega cd", "mega cd", "megacd", "sega mega cd", "sega mega-cd"}},
	{platform: PlatformSega32X, aliases: []string{"sega_32x", "sega 32x", "32x", "sega32x"}},
	{platform: PlatformPS1, aliases: []string{"ps1", "playstation", "sony playstation", "psx"}},
	{platform: PlatformPS2, aliases: []string{"ps2", "playstation 2", "sony playstation 2"}},
	{platform: PlatformPS3, aliases: []string{"ps3", "playstation 3", "sony playstation 3"}},
	{platform: PlatformPSP, aliases: []string{"psp", "playstation portable", "sony psp"}},
	{platform: PlatformXbox360, aliases: []string{"xbox_360", "xbox 360", "xbox360", "microsoft xbox 360"}},
	{platform: PlatformScummVM, aliases: []string{"scummvm"}},
	{platform: PlatformUnknown, aliases: []string{"unknown"}},
}

func NormalizePlatformAlias(value string) Platform {
	normalized := normalizePlatformAliasValue(value)
	if normalized == "" {
		return PlatformUnknown
	}
	for _, rule := range platformAliasRules {
		for _, alias := range rule.aliases {
			if normalized == normalizePlatformAliasValue(alias) {
				return rule.platform
			}
		}
	}
	return PlatformUnknown
}

func normalizePlatformAliasValue(value string) string {
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", "\\", " ")
	normalized := replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	return strings.Join(strings.Fields(normalized), " ")
}
