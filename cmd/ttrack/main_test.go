package main

import "testing"

func TestLastNonFlagArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty", nil, ""},
		// Flag values are not treated specially: this mirrors the original
		// inline loop, which picks the last arg not starting with "-".
		{"flag value picked when no real target", []string{"--speed", "2", "-q"}, "2"},
		{"all flags no values", []string{"-q", "-a"}, ""},
		{"single target", []string{"abc123"}, "abc123"},
		{"flag then target", []string{"--speed", "2", "abc123"}, "abc123"},
		{"target then flag", []string{"abc123", "-q"}, "abc123"},
		{"multiple non-flags picks last", []string{"first", "second", "third"}, "third"},
		{"flags interleaved picks last non-flag", []string{"-a", "one", "--user", "two"}, "two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastNonFlagArg(tt.args); got != tt.want {
				t.Errorf("lastNonFlagArg(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseLsScope(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantAll  bool
		wantUser string
	}{
		{"none", nil, false, ""},
		{"all long", []string{"--all"}, true, ""},
		{"all short", []string{"-a"}, true, ""},
		{"user space form", []string{"--user", "alice"}, false, "alice"},
		{"user equals form", []string{"--user=bob"}, false, "bob"},
		{"user flag without value", []string{"--user"}, false, ""},
		{"all and user both", []string{"--all", "--user", "carol"}, true, "carol"},
		{"user space last wins", []string{"--user", "x", "--user", "y"}, false, "y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAll, gotUser := parseLsScope(tt.args)
			if gotAll != tt.wantAll || gotUser != tt.wantUser {
				t.Errorf("parseLsScope(%v) = (%v, %q), want (%v, %q)",
					tt.args, gotAll, gotUser, tt.wantAll, tt.wantUser)
			}
		})
	}
}
