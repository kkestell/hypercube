package main

import "testing"

func TestParsePort(t *testing.T) {
	for _, tt := range []struct {
		args []string
		port string
	}{
		{nil, "4280"},
		{[]string{"5000"}, "5000"},
	} {
		if port, err := parsePort(tt.args); err != nil || port != tt.port {
			t.Errorf("parsePort(%v) = %q, %v; want %q", tt.args, port, err, tt.port)
		}
	}
	for _, args := range [][]string{{"0"}, {"70000"}, {"abc"}, {"5000", "6000"}} {
		if _, err := parsePort(args); err == nil {
			t.Errorf("parsePort(%v) succeeded", args)
		}
	}
}
