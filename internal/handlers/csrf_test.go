package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOriginRequiresExactHostMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		header string
		value  string
		wantOK bool
	}{
		{
			name:   "exact origin host",
			host:   "panel.local:9999",
			header: "Origin",
			value:  "http://panel.local:9999",
			wantOK: true,
		},
		{
			name:   "referer path still exact host",
			host:   "panel.local:9999",
			header: "Referer",
			value:  "http://panel.local:9999/system",
			wantOK: true,
		},
		{
			name:   "substring host does not pass",
			host:   "panel.local:9999",
			header: "Origin",
			value:  "http://evil-panel.local:9999",
			wantOK: false,
		},
		{
			name:   "host embedded in attacker domain does not pass",
			host:   "panel.local:9999",
			header: "Referer",
			value:  "http://panel.local:9999.evil.example/system",
			wantOK: false,
		},
		{
			name:   "missing origin and referer does not pass",
			host:   "panel.local:9999",
			header: "",
			value:  "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/api/system/network", nil)
			req.Host = tt.host
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}

			if got := sameOrigin(req); got != tt.wantOK {
				t.Fatalf("sameOrigin()=%v, want %v", got, tt.wantOK)
			}
		})
	}
}
