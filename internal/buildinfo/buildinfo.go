// Package buildinfo identifies the executable using metadata embedded by Go.
package buildinfo

import "runtime/debug"

// String reports the module version (including Go's +dirty suffix) and the
// source commit. Without Git metadata, it identifies a development build.
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "Hypercube devel"
	}
	return describe(info)
}

func describe(info *debug.BuildInfo) string {
	version := info.Main.Version
	if version == "" || version == "(devel)" {
		version = "devel"
	}
	text := "Hypercube " + version
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return text + " (" + setting.Value + ")"
		}
	}
	return text
}
