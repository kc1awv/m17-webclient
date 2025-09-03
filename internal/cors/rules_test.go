package cors

import "testing"

func TestNewOriginValidator(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		allowed  []string
		rejected []string
	}{
		{
			name:    "allow all",
			env:     "*",
			allowed: []string{"https://evil.com", "http://example.org"},
		},
		{
			name:     "prefix suffix rule",
			env:      "https://*.example.com",
			allowed:  []string{"https://foo.example.com"},
			rejected: []string{"https://example.com", "http://foo.example.com", "https://foo.example.org"},
		},
		{
			name:     "exact match rule",
			env:      "https://example.com",
			allowed:  []string{"https://example.com"},
			rejected: []string{"https://example.org", "http://example.com", "https://example.com/"},
		},
		{
			name: "mixed list with whitespace",
			env:  " https://exact.com , https://*.example.org,https://sub.test.com ",
			allowed: []string{
				"https://exact.com",
				"https://api.example.org",
				"https://sub.test.com",
			},
			rejected: []string{
				"https://example.org",
				"https://sub.test.org",
				"https://other.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := ParseOriginRules(tt.env)
			v := NewOriginValidator(rules)
			for _, origin := range tt.allowed {
				if !v(origin) {
					t.Errorf("origin %q not allowed for env %q", origin, tt.env)
				}
			}
			for _, origin := range tt.rejected {
				if v(origin) {
					t.Errorf("origin %q unexpectedly allowed for env %q", origin, tt.env)
				}
			}
		})
	}
}
