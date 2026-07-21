package config

import "testing"

// TestLoad_RequiredChecksFlag locks in the default-off contract: the
// required-checks feature only activates on the exact string "true".
func TestLoad_RequiredChecksFlag(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want bool
	}{
		{"unset defaults off", "", false},
		{"true enables", "true", true},
		{"other values stay off", "1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("REQUIRED_CHECKS", c.env)
			if got := Load().RequiredChecks; got != c.want {
				t.Errorf("REQUIRED_CHECKS=%q: got %v want %v", c.env, got, c.want)
			}
		})
	}
}
