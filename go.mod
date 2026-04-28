module github.com/oszuidwest/zwfm-audiologger

go 1.26.2

tool (
	golang.org/x/tools/cmd/deadcode
	golang.org/x/vuln/cmd/govulncheck
)

require (
	github.com/dustin/go-humanize v1.0.1
	github.com/netresearch/go-cron v0.14.0
)

require golang.org/x/oauth2 v0.36.0

require (
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/telemetry v0.0.0-20260421165255-392afab6f40e // indirect
	golang.org/x/tools v0.44.0 // indirect
	golang.org/x/vuln v1.3.0 // indirect
)
