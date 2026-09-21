package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// User represents a system user.
type User struct {
	ID                  uuid.UUID  `json:"id"`
	Username            string     `json:"username"`
	Email               *string    `json:"email,omitempty"`
	PasswordHash        string     `json:"-"` // Never serialized
	Role                string     `json:"role"`
	IsActive            bool       `json:"is_active"`
	MustChangePassword  bool       `json:"must_change_password"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// Device represents a managed camera or NVR.
type Device struct {
	ID               uuid.UUID       `json:"id"`
	Name             string          `json:"name"`
	DeviceType       string          `json:"device_type"` // "camera" | "nvr"
	IPAddress        string          `json:"ip_address"`
	Port             int             `json:"port"`
	MACAddress       *string         `json:"mac_address,omitempty"`
	Manufacturer     *string         `json:"manufacturer,omitempty"`
	Model            *string         `json:"model,omitempty"`
	Firmware         *string         `json:"firmware,omitempty"`
	Location         *string         `json:"location,omitempty"`
	Tags             []string        `json:"tags"`
	Notes            *string         `json:"notes,omitempty"`
	IsEnabled        bool            `json:"is_enabled"`
	StreamURL        *string         `json:"-"` // encrypted at rest, NEVER serialized
	SecondaryURL     *string         `json:"-"` // encrypted at rest, NEVER serialized
	StreamConfigured bool            `json:"stream_configured"`
	SnapshotURL      *string         `json:"snapshot_url,omitempty"`
	WebInterface     *string         `json:"web_interface,omitempty"`
	OnvifPath        *string         `json:"onvif_path,omitempty"`
	ChannelCount     int             `json:"channel_count"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	LastSeenAt       *time.Time      `json:"last_seen_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// DeviceCredential holds encrypted credentials for a device.
type DeviceCredential struct {
	ID        uuid.UUID `json:"id"`
	DeviceID  uuid.UUID `json:"device_id"`
	CredType  string    `json:"cred_type"`
	Username  *string   `json:"username,omitempty"`
	Password  *string   `json:"password,omitempty"` // Encrypted at rest
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// DeviceGroup represents a logical grouping of devices.
type DeviceGroup struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Color       *string   `json:"color,omitempty"`
	Icon        *string   `json:"icon,omitempty"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	DeviceCount int       `json:"device_count,omitempty"` // Computed field
}

// CameraNVRLink associates a camera with an NVR channel.
type CameraNVRLink struct {
	CameraID      uuid.UUID `json:"camera_id"`
	NVRID         uuid.UUID `json:"nvr_id"`
	ChannelNumber int       `json:"channel_number"`
	ChannelName   *string   `json:"channel_name,omitempty"`
}

// ScanJob represents a network discovery job.
type ScanJob struct {
	ID          uuid.UUID       `json:"id"`
	InitiatedBy *uuid.UUID      `json:"initiated_by,omitempty"`
	ScanType    string          `json:"scan_type"` // "full" | "custom" | "quick"
	Target      *string         `json:"target,omitempty"`
	Status      string          `json:"status"`
	DeviceCount int             `json:"device_count"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       *string         `json:"error,omitempty"`
	ResultsMeta json.RawMessage `json:"results_meta,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// DiscoveredDevice represents a raw scan result.
type DiscoveredDevice struct {
	ID          uuid.UUID       `json:"id"`
	ScanJobID   uuid.UUID       `json:"scan_job_id"`
	IPAddress   string          `json:"ip_address"`
	MACAddress  *string         `json:"mac_address,omitempty"`
	Hostname    *string         `json:"hostname,omitempty"`
	Vendor      *string         `json:"vendor,omitempty"`
	OpenPorts   []int           `json:"open_ports"`
	DeviceType  string          `json:"device_type"`
	IsReachable bool            `json:"is_reachable"`
	RTSPFound   bool            `json:"rtsp_found"`
	ONVIFFound  bool            `json:"onvif_found"`
	WebFound    bool            `json:"web_found"`
	WebTitle    *string         `json:"web_title,omitempty"`
	Fingerprint json.RawMessage `json:"fingerprint,omitempty"`
	Confidence  float32         `json:"confidence"`
	CreatedAt   time.Time       `json:"created_at"`
}

// StreamSession represents an active/past streaming session.
type StreamSession struct {
	ID          uuid.UUID  `json:"id"`
	DeviceID    uuid.UUID  `json:"device_id"`
	Channel     int        `json:"channel"`
	SessionType string     `json:"session_type"`
	Status      string     `json:"status"`
	HLSPath     *string    `json:"hls_path,omitempty"`
	ViewerCount int        `json:"viewer_count"`
	PID         *int       `json:"pid,omitempty"`
	Error       *string    `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	StoppedAt   *time.Time `json:"stopped_at,omitempty"`
}

// Layout represents a saved dashboard grid configuration.
type Layout struct {
	ID        uuid.UUID       `json:"id"`
	UserID    *uuid.UUID      `json:"user_id,omitempty"`
	Name      string          `json:"name"`
	GridCols  int             `json:"grid_cols"`
	GridRows  int             `json:"grid_rows"`
	Cells     json.RawMessage `json:"cells"`
	IsDefault bool            `json:"is_default"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// LayoutCell represents a single cell in a layout grid.
type LayoutCell struct {
	Position int        `json:"position"`
	DeviceID *uuid.UUID `json:"device_id,omitempty"`
	Channel  int        `json:"channel"`
	Label    string     `json:"label"`
}

// RotationSchedule defines an auto-rotation configuration.
type RotationSchedule struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	GroupID      *uuid.UUID `json:"group_id,omitempty"`
	LayoutMode   string     `json:"layout_mode"`
	IntervalSecs int        `json:"interval_secs"`
	IsActive     bool       `json:"is_active"`
	Transition   string     `json:"transition"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// Computed
	Cameras []RotationCamera `json:"cameras,omitempty"`
}

// RotationCamera is a camera entry in a rotation schedule with ordering.
type RotationCamera struct {
	ScheduleID uuid.UUID `json:"schedule_id"`
	DeviceID   uuid.UUID `json:"device_id"`
	SortOrder  int       `json:"sort_order"`
}

// AuditLog records an admin action.
type AuditLog struct {
	ID         int64           `json:"id"`
	UserID     *uuid.UUID      `json:"user_id,omitempty"`
	Actor      *string         `json:"actor,omitempty"` // username snapshot
	Action     string          `json:"action"`
	Resource   *string         `json:"resource,omitempty"`
	ResourceID *uuid.UUID      `json:"resource_id,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	Result     string          `json:"result"` // "success" | "fail" | "denied"
	IPAddress  *string         `json:"ip_address,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}
