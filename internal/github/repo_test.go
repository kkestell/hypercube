package github

import "testing"

func TestParseRepo(t *testing.T) {
	for _, tc := range []struct {
		url         string
		owner, name string
		ok          bool
	}{
		{"https://github.com/o/r", "o", "r", true},
		{"https://github.com/o/r.git", "o", "r", true},
		{"https://github.com/o/r/", "o", "r", true},
		{"http://github.com/o/r.git/", "o", "r", true},
		{"https://GitHub.com/o/r", "o", "r", true},
		{"https://user@github.com/o/r", "o", "r", true},
		{"ssh://git@github.com/o/r.git", "o", "r", true},
		{"ssh://git@github.com:22/o/r", "o", "r", true},
		{"git://github.com/o/r", "o", "r", true},
		{"git@github.com:o/r", "o", "r", true},
		{"git@github.com:o/r.git", "o", "r", true},
		{"git@GITHUB.COM:o/r.git/", "o", "r", true},
		{"https://gitlab.com/o/r", "", "", false},
		{"git@gitlab.com:o/r.git", "", "", false},
		{"https://github.com.evil.com/o/r", "", "", false},
		{"https://github.com/o/r/extra", "", "", false},
		{"git@github.com:o/r/extra", "", "", false},
		{"https://github.com/o", "", "", false},
		{"https://github.com/o/", "", "", false},
		{"https://github.com//r", "", "", false},
		{"file:///srv/github.com/o/r", "", "", false},
		{"/local/path/o/r", "", "", false},
		{"", "", "", false},
	} {
		owner, name, ok := parseRepo(tc.url)
		if owner != tc.owner || name != tc.name || ok != tc.ok {
			t.Errorf("parseRepo(%q) = %q, %q, %v; want %q, %q, %v", tc.url, owner, name, ok, tc.owner, tc.name, tc.ok)
		}
	}
}
