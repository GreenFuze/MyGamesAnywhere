package desktop

import "testing"

func TestNewHostFailsFastForInvalidOptions(t *testing.T) {
	binding := BindingOption{ServerURL: "http://mga", Unpair: func() error { return nil }}
	tests := []Options{
		{LogPath: "client.log", Version: "1.0.0", Bindings: []BindingOption{binding}},
		{DisplayName: "PC / user", Version: "1.0.0", Bindings: []BindingOption{binding}},
		{DisplayName: "PC / user", LogPath: "client.log", Bindings: []BindingOption{binding}},
		{DisplayName: "PC / user", LogPath: "client.log", Version: "1.0.0"},
		{DisplayName: "PC / user", LogPath: "client.log", Version: "1.0.0", Bindings: []BindingOption{{ServerURL: "http://mga"}}},
	}
	for _, options := range tests {
		if _, err := NewHost(options); err == nil {
			t.Fatalf("NewHost(%+v) succeeded", options)
		}
	}
}

func TestNewHostAcceptsMultipleBindings(t *testing.T) {
	_, err := NewHost(Options{DisplayName: "PC / user", LogPath: "client.log", Version: "1.0.0", Bindings: []BindingOption{
		{ServerURL: "http://localhost:8900", Unpair: func() error { return nil }},
		{ServerURL: "http://tv2:8900", Unpair: func() error { return nil }},
	}})
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
}
