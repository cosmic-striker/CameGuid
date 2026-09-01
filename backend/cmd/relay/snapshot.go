package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// snapshotCaptureTimeout bounds every one-shot FFmpeg frame grab so a dead
// camera cannot hang a capture goroutine forever.
const snapshotCaptureTimeout = 15 * time.Second

// SnapshotJob captures a single frame from a camera's stream periodically and
// writes it to <snapshotBaseDir>/<deviceID>.jpg, which nginx serves at
// /stream/snapshots/<deviceID>.jpg. This is the cheap "thumbnail" engine that
// lets Live Walls show hundreds of cameras without transcoding all of them.
type SnapshotJob struct {
	DeviceID    string
	Channel     int
	StreamURL   string
	Width       int
	Interval    time.Duration
	Ctx         context.Context
	Cancel      context.CancelFunc
	LastCapture time.Time
	LastError   string
	Active      bool
}

// SnapshotManager owns all periodic snapshot jobs in the relay process.
type SnapshotManager struct {
	runner func(ctx context.Context, streamURL, outPath string, width int) error
	jobs   map[string]*SnapshotJob
	mu     sync.Mutex

	// Observability counters
	capturesTotal atomic.Int64
	failuresTotal atomic.Int64
}

func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{
		runner: captureSnapshot,
		jobs:   make(map[string]*SnapshotJob),
	}
}

// running reports how many periodic capture jobs are active, for /metrics.
func (m *SnapshotManager) running() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.jobs))
}

// Start registers (or replaces) a periodic capture job for a device.
func (m *SnapshotManager) Start(deviceID, streamURL string, channel, width int, interval time.Duration) *SnapshotJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop any existing job for this device first so intervals stay consistent
	if existing, ok := m.jobs[deviceID]; ok {
		existing.Cancel()
		delete(m.jobs, deviceID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &SnapshotJob{
		DeviceID:  deviceID,
		Channel:   channel,
		StreamURL: streamURL,
		Width:     width,
		Interval:  interval,
		Ctx:       ctx,
		Cancel:    cancel,
		Active:    true,
	}
	m.jobs[deviceID] = job

	go m.captureLoop(job)
	return job
}

// Stop cancels the periodic capture job for a device, if any.
func (m *SnapshotManager) Stop(deviceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[deviceID]
	if !ok {
		return false
	}
	job.Cancel()
	delete(m.jobs, deviceID)
	return true
}

// IsActive reports whether a device currently has a periodic snapshot job
// whose last capture succeeded. Safe for concurrent use.
func (m *SnapshotManager) IsActive(deviceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[deviceID]
	return ok && job.Active
}

// CaptureNow performs a single immediate frame grab for a device.
func (m *SnapshotManager) CaptureNow(deviceID, streamURL string, channel, width int) error {
	outPath := filepath.Join(snapshotBaseDir, deviceID+".jpg")
	ctx, cancel := context.WithTimeout(context.Background(), snapshotCaptureTimeout)
	defer cancel()
	err := m.runner(ctx, streamURL, outPath, width)
	m.capturesTotal.Add(1)
	if err != nil {
		m.failuresTotal.Add(1)
	}
	return err
}

// captureLoop repeatedly captures frames until the job is cancelled.
func (m *SnapshotManager) captureLoop(job *SnapshotJob) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	// Grab the first frame immediately so the wall isn't empty on start.
	m.capture(job)

	for {
		select {
		case <-job.Ctx.Done():
			return
		case <-ticker.C:
			m.capture(job)
		}
	}
}

func (m *SnapshotManager) capture(job *SnapshotJob) {
	outPath := filepath.Join(snapshotBaseDir, job.DeviceID+".jpg")
	ctx, cancel := context.WithTimeout(context.Background(), snapshotCaptureTimeout)
	defer cancel()

	err := m.runner(ctx, job.StreamURL, outPath, job.Width)
	job.LastCapture = time.Now()
	m.capturesTotal.Add(1)
	if err != nil {
		m.failuresTotal.Add(1)
	}

	m.mu.Lock()
	if err != nil {
		job.LastError = err.Error()
		job.Active = false
	} else {
		job.LastError = ""
		job.Active = true
	}
	m.mu.Unlock()
}

// captureSnapshot runs FFmpeg once to grab a single frame from a stream.
//
// Uses a bounded subprocess with a context timeout so unresponsive cameras
// never leak goroutines or hang the snapshot worker.
func captureSnapshot(ctx context.Context, streamURL, outPath string, width int) error {
	args := []string{"-y"}
	if len(streamURL) >= 4 && streamURL[:4] == "rtsp" {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args,
		"-i", streamURL,
		"-frames:v", "1",
		"-ss", "1",
	)
	if width > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", width))
	}
	args = append(args,
		"-an",
		"-sn",
		"-f", "image2",
		outPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("snapshot capture failed: %w", err)
	}
	return nil
}

// ensureSnapshotDir creates the shared snapshot directory if needed.
func ensureSnapshotDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}