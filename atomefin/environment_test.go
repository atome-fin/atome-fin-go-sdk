package atomefin

import (
	"strings"
	"testing"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		env       Environment
		wantHost  string
		wantError bool
	}{
		{EnvPre, "id-api-pre.apaylater.net", false},
		{EnvProd, "api.atome.id", false},
		{Environment("test"), "", true},
		{Environment("staging"), "", true},
		{Environment(""), "", true},
	}
	for _, tt := range tests {
		got, err := BaseURL(tt.env)
		if (err != nil) != tt.wantError {
			t.Errorf("BaseURL(%q) err = %v, wantError = %v", tt.env, err, tt.wantError)
			continue
		}
		if tt.wantError {
			continue
		}
		if !strings.Contains(got, tt.wantHost) {
			t.Errorf("BaseURL(%q) = %q, want host %q", tt.env, got, tt.wantHost)
		}
		if !strings.HasPrefix(got, "https://") {
			t.Errorf("BaseURL(%q) = %q, want https://", tt.env, got)
		}
		if !strings.HasSuffix(got, "/grabpaylater") {
			t.Errorf("BaseURL(%q) = %q, want suffix /grabpaylater", tt.env, got)
		}
	}
}
