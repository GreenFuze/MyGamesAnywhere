package clientapp

import "testing"

func TestExactScummVMGameIDRequiresOneEngineQualifiedTarget(t *testing.T) {
	for name, test := range map[string]struct {
		output string
		want   string
		ok     bool
	}{
		"one":       {output: "Game ID: scumm:monkey\nName: Monkey Island", want: "scumm:monkey", ok: true},
		"duplicate": {output: "scumm:monkey SCUMM:MONKEY", want: "scumm:monkey", ok: true},
		"none":      {output: "monkey", ok: false},
		"ambiguous": {output: "scumm:monkey\nscumm:monkey2", ok: false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := exactScummVMGameID(test.output)
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("exactScummVMGameID() = %q, %v", got, err)
			}
		})
	}
}
