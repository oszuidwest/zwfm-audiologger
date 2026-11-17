package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
)

func TestStartWaitsForStationTasks(t *testing.T) {
	s := New(&config.Config{Stations: map[string]config.Station{"station": {}}}, nil)
	ctx, cancel := context.WithCancel(t.Context())

	started, release := make(chan struct{}), make(chan struct{})
	s.forEachStationAsync(ctx, "test", func(string, *config.Station) {
		close(started)
		<-release
	})
	<-started

	// Cancelled before Start runs, so Start skips catchup and never touches
	// the nil recorder.
	cancel()
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	select {
	case err := <-done:
		t.Fatalf("Start returned (err %v) while a station task was still running", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after the station task finished")
	}
}
