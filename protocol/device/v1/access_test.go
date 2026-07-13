package v1

import "testing"

func TestAccessLevelAllows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		granted  AccessLevel
		required AccessLevel
		want     bool
	}{
		{name: "same level", granted: AccessPlay, required: AccessPlay, want: true},
		{name: "higher level", granted: AccessOwner, required: AccessManage, want: true},
		{name: "lower level", granted: AccessView, required: AccessPlay, want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.granted.Allows(tt.required)
			if err != nil {
				t.Fatalf("Allows() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Allows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccessLevelAllowsRejectsUnknownLevel(t *testing.T) {
	t.Parallel()

	if _, err := AccessLevel("administrator").Allows(AccessView); err == nil {
		t.Fatal("Allows() error = nil, want error")
	}
	if _, err := AccessOwner.Allows(AccessLevel("administrator")); err == nil {
		t.Fatal("Allows() with unknown required level error = nil, want error")
	}
}
