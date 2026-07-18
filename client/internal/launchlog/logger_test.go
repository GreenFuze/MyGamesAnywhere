package launchlog

import "testing"

func TestFormatArgsRedactsPairingSecrets(t *testing.T) {
	t.Parallel()

	got := FormatArgs([]string{
		"mga-client-agent.exe",
		"protocol",
		"mga://pair?server=http://tv2:8900&code=secret-code",
	})
	want := "mga-client-agent.exe protocol mga://pair?code=REDACTED&server=http%3A%2F%2Ftv2%3A8900"
	if got != want {
		t.Fatalf("FormatArgs() = %q, want %q", got, want)
	}
}

func TestFormatArgsRedactsLaunchToken(t *testing.T) {
	t.Parallel()

	got := FormatArgs([]string{
		"protocol",
		"mga://start?server=http://tv2:8900&launch_id=abc&token=secret&mode=standard",
	})
	if got != "protocol mga://start?launch_id=abc&mode=standard&server=http%3A%2F%2Ftv2%3A8900&token=REDACTED" {
		t.Fatalf("FormatArgs() = %q", got)
	}
}
