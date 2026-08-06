package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionRegistryKey(t *testing.T) {
	got := sessionRegistryKey("sess-1")
	want := sessionRegistryPrefix + "sess-1"
	if got != want {
		t.Fatalf("sessionRegistryKey() = %q, want %q", got, want)
	}
}

func TestRegisteredSessionRoundTrip(t *testing.T) {
	started := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	reg := RegisteredSession{
		ID:        "abcd-1234",
		DeviceID:  "cam-42",
		Channel:   3,
		Status:    "active",
		HLSPath:   "/tmp/streams/abcd-1234/index.m3u8",
		StartedAt: started,
	}

	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded RegisteredSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded != reg {
		t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", decoded, reg)
	}
}