package selfupdate

import "testing"

func TestIsNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		tag, current string
		want         bool
	}{
		{"v0.9.0", "0.8.0", true},
		{"v0.8.1", "0.8.0", true},
		{"v1.0.0", "0.99.99", true},

		// Compared as numbers: as strings "0.10.0" would sort below "0.9.0".
		{"v0.10.0", "0.9.0", true},
		{"v0.9.0", "0.10.0", false},

		// The screenshot: a re-cut release briefly made an older one "latest".
		{"v0.4.1", "0.5.0", false},
		{"v0.8.0", "0.8.0", false},

		// Unparseable or unknown is never an upgrade.
		{"", "0.8.0", false},
		{"latest", "0.8.0", false},
		{"v0.9", "0.8.0", false},
		{"v0.9.0.1", "0.8.0", false},
		{"v0.9.0-rc1", "0.8.0", false},
		{"v0.9.0+build5", "0.8.0", false},
		{"v+0.9.0", "0.8.0", false},
		{"v0.-9.0", "0.8.0", false},
		{"v0.9.0", "dev", false},
	} {
		if got := IsNewer(tc.tag, tc.current); got != tc.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.tag, tc.current, got, tc.want)
		}
	}
}
