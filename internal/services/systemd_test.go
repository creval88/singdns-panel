package services

import (
	"reflect"
	"testing"
)

func TestParseSystemdExecStartFieldsUsesLastEffectiveEntry(t *testing.T) {
	t.Parallel()

	output := `[Service]
ExecStart=/usr/bin/sing-box -D /var/lib/sing-box -C /etc/sing-box run

# /etc/systemd/system/sing-box.service.d/10-panel.conf
[Service]
ExecStart=
ExecStart=/usr/local/bin/sing-box -D /var/lib/sing-box -C /etc/sing-box run
`

	got := parseSystemdExecStartFields(output)
	want := []string{"/usr/local/bin/sing-box", "-D", "/var/lib/sing-box", "-C", "/etc/sing-box", "run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fields: %#v", got)
	}
}

func TestParseSystemdExecStartFieldsIgnoresEmptyResetLines(t *testing.T) {
	t.Parallel()

	output := `[Service]
ExecStart=
ExecStart=/usr/bin/sing-box run
`

	got := parseSystemdExecStartFields(output)
	want := []string{"/usr/bin/sing-box", "run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fields: %#v", got)
	}
}
