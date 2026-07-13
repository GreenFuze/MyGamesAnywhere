package clientapp

import "testing"

func TestValidateServerURLAllowsHTTPFromLAN(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "host name", input: "http://tv2:8900", want: "http://tv2:8900"},
		{name: "LAN address", input: "http://192.168.68.51:8900/", want: "http://192.168.68.51:8900"},
		{name: "HTTPS remains supported", input: "https://mga.example.test/", want: "https://mga.example.test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateServerURL(test.input)
			if err != nil {
				t.Fatalf("validateServerURL(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("validateServerURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestValidateServerURLRejectsUnsupportedTransport(t *testing.T) {
	if _, err := validateServerURL("ftp://tv2:8900"); err == nil {
		t.Fatal("validateServerURL() accepted unsupported FTP transport")
	}
}
