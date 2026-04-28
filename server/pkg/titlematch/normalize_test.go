package titlematch

import "testing"

func TestNormalizeLookupTitleStripsCommonDumpNoise(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "region and dump tags", in: "aladdin (u) [!]", want: "aladdin"},
		{name: "mame set suffixes", in: "Altered Beast (set 8) (8751 317-0078)", want: "altered beast"},
		{name: "multiple bracket suffixes", in: "Sonic The Hedgehog [!] (USA) [Rev A]", want: "sonic the hedgehog"},
		{name: "setup prefix and version", in: "setup_aladdin_v1.2", want: "aladdin"},
		{name: "repeated whitespace and punctuation", in: "  Aladdin!!!   ", want: "aladdin"},
		{name: "mixed suffix noise", in: "doom [beta] (usa) v1.1", want: "doom"},
		{name: "keeps legitimate title text", in: "prince of persia", want: "prince of persia"},
		{name: "keeps decimal sequel title text", in: "Final Fantasy 2.0", want: "final fantasy 2 0"},
		{name: "keeps middle qualifier text", in: "The Legend of Zelda (Classic) Adventures", want: "the legend of zelda classic adventures"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeLookupTitle(tc.in); got != tc.want {
				t.Fatalf("NormalizeLookupTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanDisplayTitleStripsSuffixNoiseAndPreservesCasing(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "mame set suffixes", in: "Altered Beast (set 8) (8751 317-0078)", want: "Altered Beast"},
		{name: "multiple bracket suffixes", in: "Sonic The Hedgehog [!] (USA) [Rev A]", want: "Sonic The Hedgehog"},
		{name: "setup prefix and version", in: "setup_Aladdin_v1.2", want: "Aladdin"},
		{name: "keeps decimal sequel title text", in: "Final Fantasy 2.0", want: "Final Fantasy 2.0"},
		{name: "keeps middle qualifier text", in: "The Legend of Zelda (Classic) Adventures", want: "The Legend of Zelda (Classic) Adventures"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanDisplayTitle(tc.in); got != tc.want {
				t.Fatalf("CleanDisplayTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
