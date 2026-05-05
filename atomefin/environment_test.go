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
		{EnvTest, "id-api.apaylater.net", false},
		{EnvPre, "id-api-pre.apaylater.net", false},
		{EnvProd, "api.atome.id", false},
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
		if !strings.HasSuffix(got, "/white-label/G") {
			t.Errorf("BaseURL(%q) = %q, want suffix /white-label/G", tt.env, got)
		}
	}
}
