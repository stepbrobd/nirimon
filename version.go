package main

import (
	"fmt"
	"runtime"
)

// Build variables - these are set via ldflags during build
var (
	Version   = "git" // Semantic version
	GoVersion = runtime.Version()
)

// VersionInfo returns formatted version information
func VersionInfo() string {
	return fmt.Sprintf("nirimon %s (%s)", Version, GoVersion)
}

// ShortVersion returns just the version number
func ShortVersion() string {
	return Version
}
