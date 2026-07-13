package auth

import "testing"

func TestValidateNewCredentialUsesSimpleTrustedLANPolicy(t *testing.T) {
	tests := []struct {
		name    string
		kind    CredentialKind
		value   string
		wantErr bool
	}{
		{name: "four character password", kind: CredentialPassword, value: "abcd"},
		{name: "password has no composition rules", kind: CredentialPassword, value: " a! "},
		{name: "four Unicode character password", kind: CredentialPassword, value: "🔐🔐🔐🔐"},
		{name: "three character password", kind: CredentialPassword, value: "abc", wantErr: true},
		{name: "three Unicode character password", kind: CredentialPassword, value: "🔐🔐🔐", wantErr: true},
		{name: "four digit PIN", kind: CredentialPIN, value: "1234"},
		{name: "long PIN has no maximum", kind: CredentialPIN, value: "123456789012345678901234567890"},
		{name: "three digit PIN", kind: CredentialPIN, value: "123", wantErr: true},
		{name: "PIN remains digits only", kind: CredentialPIN, value: "12a4", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNewCredential(test.kind, test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateNewCredential(%q, %q) error = %v, wantErr = %v", test.kind, test.value, err, test.wantErr)
			}
		})
	}
}
