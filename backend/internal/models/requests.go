package models

import "github.com/google/uuid"

// --- Auth ---

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=64"`
	Email    string `json:"email,omitempty" validate:"omitempty,email"`
	Password string `json:"password" validate:"required,min=8"`
	Role     string `json:"role" validate:"required,oneof=admin operator viewer"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// --- Devices ---

type CreateDeviceRequest struct {
	Name         string   `json:"name" validate:"required,max=128"`
	DeviceType   string   `json:"device_type" validate:"required,oneof=camera nvr"`
	IPAddress    string   `json:"ip_address" validate:"required"`
	Port         int      `json:"port,omitempty"`
	MACAddress   *string  `json:"mac_address,omitempty"`
	Manufacturer *string  `json:"manufacturer,omitempty"`
	Model        *string  `json:"model,omitempty"`
	Firmware     *string  `json:"firmware,omitempty"`
	Location     *string  `json:"location,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
	StreamURL    *string  `json:"stream_url,omitempty"`
	SecondaryURL *string  `json:"secondary_url,omitempty"`
	SnapshotURL  *string  `json:"snapshot_url,omitempty"`
	WebInterface *string  `json:"web_interface,omitempty"`
	OnvifPath    *string  `json:"onvif_path,omitempty"`
	ChannelCount int      `json:"channel_count,omitempty"`
}

type UpdateDeviceRequest struct {
	Name         *string  `json:"name,omitempty"`
	IPAddress    *string  `json:"ip_address,omitempty"`
	Port         *int     `json:"port,omitempty"`
	MACAddress   *string  `json:"mac_address,omitempty"`
	Manufacturer *string  `json:"manufacturer,omitempty"`
	Model        *string  `json:"model,omitempty"`
	Firmware     *string  `json:"firmware,omitempty"`
	Location     *string  `json:"location,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
	StreamURL    *string  `json:"stream_url,omitempty"`
	SecondaryURL *string  `json:"secondary_url,omitempty"`
	SnapshotURL  *string  `json:"snapshot_url,omitempty"`
	WebInterface *string  `json:"web_interface,omitempty"`
	OnvifPath    *string  `json:"onvif_path,omitempty"`
	ChannelCount *int     `json:"channel_count,omitempty"`
}

type DeviceListParams struct {
	Type    string `query:"type"`
	Enabled *bool  `query:"enabled"`
	GroupID string `query:"group_id"`
	Search  string `query:"search"`
	Page    int    `query:"page"`
	Limit   int    `query:"limit"`
}

// --- Groups ---

type CreateGroupRequest struct {
	Name        string  `json:"name" validate:"required,max=128"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

type AddGroupMemberRequest struct {
	DeviceID  uuid.UUID `json:"device_id" validate:"required"`
	SortOrder int       `json:"sort_order"`
}

// --- Credentials ---

type CreateCredentialRequest struct {
	CredType string  `json:"cred_type" validate:"required,oneof=rtsp onvif web ssh api"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	Priority int     `json:"priority"`
}

// --- Scans ---

type StartScanRequest struct {
	ScanType string  `json:"scan_type" validate:"required,oneof=full custom quick"`
	Target   *string `json:"target,omitempty"` // CIDR for custom scan
}

// --- Streams ---

type StartStreamRequest struct {
	DeviceID uuid.UUID `json:"device_id" validate:"required"`
	Channel  int       `json:"channel,omitempty"`
	Type     string    `json:"type,omitempty"` // hls (default)
}

type StartStreamResponse struct {
	SessionID string `json:"session_id"`
	HLSURL    string `json:"hls_url"`
	Status    string `json:"status"`
}

// --- Layouts ---

type CreateLayoutRequest struct {
	Name     string `json:"name" validate:"required,max=128"`
	GridCols int    `json:"grid_cols" validate:"required,min=1,max=6"`
	GridRows int    `json:"grid_rows" validate:"required,min=1,max=6"`
	Cells    []LayoutCell `json:"cells" validate:"required"`
}

// --- Rotations ---

type CreateRotationRequest struct {
	Name         string     `json:"name" validate:"required,max=128"`
	GroupID      *uuid.UUID `json:"group_id,omitempty"`
	LayoutMode   string     `json:"layout_mode" validate:"required,oneof=1x1 2x2 3x3 4x4 1x2 2x3"`
	IntervalSecs int        `json:"interval_secs" validate:"required,min=3"`
	Transition   string     `json:"transition,omitempty"`
	CameraIDs    []uuid.UUID `json:"camera_ids,omitempty"`
}

// --- Pagination ---

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

// --- Dashboard ---

type DashboardStats struct {
	TotalDevices   int `json:"total_devices"`
	TotalCameras   int `json:"total_cameras"`
	TotalNVRs      int `json:"total_nvrs"`
	ActiveStreams   int `json:"active_streams"`
	EnabledDevices int `json:"enabled_devices"`
	TotalGroups    int `json:"total_groups"`
	LastScanAt     *string `json:"last_scan_at,omitempty"`
}

// --- Error ---

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}
