package main

import "testing"

func TestParseUIArgs(t *testing.T) {
	for _, tt := range []struct {
		args []string
		port string
		menu bool
	}{
		{nil, "4280", false},
		{[]string{"5000"}, "5000", false},
		{[]string{"--menu"}, "4280", true},
		{[]string{"5000", "--menu"}, "5000", true},
		{[]string{"--menu", "5000"}, "5000", true},
	} {
		port, menu, err := parseUIArgs(tt.args)
		if err != nil || port != tt.port || menu != tt.menu {
			t.Errorf("parseUIArgs(%v) = %q, %v, %v; want %q, %v", tt.args, port, menu, err, tt.port, tt.menu)
		}
	}
	for _, args := range [][]string{{"0"}, {"70000"}, {"abc"}, {"5000", "6000"}, {"--bogus"}} {
		if _, _, err := parseUIArgs(args); err == nil {
			t.Errorf("parseUIArgs(%v) succeeded", args)
		}
	}
}
