package version

// Build information (set via ldflags during build)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// UserAgent returns the User-Agent string for HTTP requests
func UserAgent() string {
	return "zwfm-audiologger/" + Version
}

// Info returns a formatted string with build information
func Info() string {
	return "zwfm-audiologger " + Version + " (commit: " + Commit + ", built: " + BuildTime + ")"
}
