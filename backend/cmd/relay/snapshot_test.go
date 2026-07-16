package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeRunner records capture calls instead of running FFmpeg.
type fakeRunner struct {
	mu   sync.Mutex
	calls []captureCall
	err  error
}

type captureCall struct {
	streamURL string
	outPath   string
	width     int
}

func (f *fakeRunner) run(ctx context.Context, streamURL, outPath string, width int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, captureCall{streamURL: streamURL, outPath: outPath, width: width})
	return f.err
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeRunner) last() captureCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return captureCall{}
	}
	return f.calls[len(f.calls)-1]
}

func TestHeartbeatKey(t *testing.T) {
	got := heartbeatKey("abc-123")
	want := heartbeatKeyPrefix + "abc-123"
	if got != want {
		t.Fatalf("heartbeatKey() = %q, want %q", got, want)
	}
}

func TestSnapshotCaptureNow(t *testing.T) {
	fake := &fakeRunner{}
	m := NewSnapshotManager()
	m.runner = fake.run

	if err := m.CaptureNow("cam-1", "rtsp://x/y", 1, 640); err != nil {
		t.Fatalf("CaptureNow() error = %v", err)
	}

	call := fake.last()
	if call.streamURL != "rtsp://x/y" {
		t.Errorf("streamURL = %q, want %q", call.streamURL, "rtsp://x/y")
	}
	if want := "/tmp/snapshots/cam-1.jpg"; call.outPath != want {
		t.Errorf("outPath = %q, want %q", call.outPath, want)
	}
	if call.width != 640 {
		t.Errorf("width = %d, want 640", call.width)
	}
}

func TestSnapshotCaptureNowPropagatesError(t *testing.T) {
	fake := &fakeRunner{err: errors.New("boom")}
	m := NewSnapshotManager()
	m.runner = fake.run

	if err := m.CaptureNow("cam-1", "rtsp://x/y", 1, 0); err == nil {
		t.Fatal("CaptureNow() expected error, got nil")
	}
}

func TestSnapshotPeriodicJobStartsAndStops(t *testing.T) {
	fake := &fakeRunner{}
	m := NewSnapshotManager()
	m.runner = fake.run

	// 20ms interval; the loop grabs a frame immediately, then every 20ms.
	m.Start("cam-2", "rtsp://x/stream", 2, 320, 20*time.Millisecond)

	// Give it a handful of intervals so several captures happen.
	time.Sleep(90 * time.Millisecond)

	if fake.count() < 3 {
		t.Fatalf("expected at least 3 captures after 90ms, got %d", fake.count())
	}
	if want := "/tmp/snapshots/cam-2.jpg"; fake.last().outPath != want {
		t.Errorf("outPath = %q, want %q", fake.last().outPath, want)
	}
	if !m.IsActive("cam-2") {
		t.Error("job should report active")
	}

	// Stop should cancel the loop and halt further captures.
	if !m.Stop("cam-2") {
		t.Fatal("Stop() returned false for an active job")
	}
	before := fake.count()
	time.Sleep(50 * time.Millisecond)
	after := fake.count()
	if after != before {
		t.Errorf("captures continued after Stop() (before=%d, after=%d)", before, after)
	}
	if m.IsActive("cam-2") {
		t.Error("job should report inactive after Stop()")
	}

	// Stopping again should report no active job.
	if m.Stop("cam-2") {
		t.Error("Stop() returned true for a job that was already stopped")
	}
}