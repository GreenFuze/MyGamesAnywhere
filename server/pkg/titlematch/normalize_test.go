package titlematch

import "testing"

func TestNormalizeLookupTitleStripsCommonDumpNoise(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "region and dump tags", in: "aladdin (u) [!]", want: "aladdin"},
		{name: "setup prefix and version", in: "setup_aladdin_v1.2", want: "aladdin"},
		{name: "repeated whitespace and punctuation", in: "  Aladdin!!!   ", want: "aladdin"},
		{name: "mixed suffix noise", in: "doom [beta] (usa) v1.1", want: "doom"},
		{name: "keeps legitimate title text", in: "prince of persia", want: "prince of persia"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeLookupTitle(tc.in); got != tc.want {
				t.Fatalf("NormalizeLookupTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
