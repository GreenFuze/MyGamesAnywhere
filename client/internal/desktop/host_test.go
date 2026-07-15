package desktop

import "testing"

func TestNewHostFailsFastForInvalidOptions(t *testing.T) {
	tests := []Options{
		{LogPath: "client.log", Version: "1.0.0"},
		{DisplayName: "PC / user", Version: "1.0.0"},
		{DisplayName: "PC / user", LogPath: "client.log"},
	}
	for _, options := range tests {
		if _, err := NewHost(options); err == nil {
			t.Fatalf("NewHost(%+v) succeeded", options)
		}
	}
}
