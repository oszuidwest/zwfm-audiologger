// Package version provides build information and versioning utilities for the application.
package version

// Version is the current version of the application, set during build.
var Version = "dev"

// Commit is the git commit hash, set during build.
var Commit = "unknown"

// BuildTime is the build timestamp, set during build.
var BuildTime = "unknown"

// UserAgent returns the User-Agent string for HTTP requests.
func UserAgent() string {
	return "zwfm-audiologger/" + Version
}

// Info returns a formatted string with build information.
func Info() string {
	return "zwfm-audiologger " + Version + " (commit: " + Commit + ", built: " + BuildTime + ")"
}
