package services

import "testing"

func TestExpandAllPlaceholderFallbackDirectWhenEmpty(t *testing.T) {
	group := map[string]any{
		"type":      "selector",
		"outbounds": []any{"{all}"},
		"filter": []any{
			map[string]any{"action": "include", "keywords": []any{"HK"}},
		},
	}
	out, changed := expandAllPlaceholder(group, []string{"US-1"})
	if !changed {
		t.Fatalf("expected changed")
	}
	if len(out) != 1 || out[0] != "direct" {
		t.Fatalf("out=%#v, want [direct]", out)
	}
}
