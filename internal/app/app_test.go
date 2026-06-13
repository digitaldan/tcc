package app

import "testing"

func TestParseFileURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"file:///tmp/foo", "/tmp/foo"},
		{"file://localhost/tmp/foo", "/tmp/foo"},
		{"file://host.example/home/dan", "/home/dan"},
		{"file:///tmp/my%20dir", "/tmp/my dir"},           // percent-decoded
		{"file:///repos/project%231", "/repos/project#1"}, // encoded '#'
		{"  file:///tmp/foo  ", "/tmp/foo"},               // trimmed
		{"/tmp/bare", "/tmp/bare"},                        // bare path
		{"/repos/project#1", "/repos/project#1"},          // bare path, literal '#'
		{"/repos/project?x", "/repos/project?x"},          // bare path, literal '?'
		{"/repos/100%done", "/repos/100%done"},            // bare path, literal '%'
		{"http://example.com", ""},                        // other scheme ignored
		{"", ""},
	}
	for _, tc := range cases {
		if got := parseFileURL(tc.in); got != tc.want {
			t.Errorf("parseFileURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
