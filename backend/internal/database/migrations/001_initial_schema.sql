-- 001_initial_schema.sql
-- Core tables for CamGuid CCTV/NVR monitoring system

-- Users
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(64) UNIQUE NOT NULL,
    email         VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(16) NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('admin', 'viewer')),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Devices (cameras and NVRs)
CREATE TABLE IF NOT EXISTS devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(128) NOT NULL,
    device_type     VARCHAR(16) NOT NULL CHECK (device_type IN ('camera', 'nvr')),
    ip_address      INET NOT NULL,
    port            INTEGER DEFAULT 554,
    mac_address     MACADDR,
    manufacturer    VARCHAR(128),
    model           VARCHAR(128),
    firmware        VARCHAR(64),
    location        VARCHAR(255),
    tags            TEXT[] DEFAULT '{}',
    notes           TEXT,
    is_enabled      BOOLEAN NOT NULL DEFAULT true,
    stream_url      VARCHAR(512),
    secondary_url   VARCHAR(512),
    snapshot_url    VARCHAR(512),
    web_interface   VARCHAR(512),
    onvif_path      VARCHAR(128) DEFAULT '/onvif/device_service',
    channel_count   INTEGER DEFAULT 1,
    metadata        JSONB DEFAULT '{}',
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_devices_type ON devices(device_type);
CREATE INDEX IF NOT EXISTS idx_devices_ip ON devices(ip_address);
CREATE INDEX IF NOT EXISTS idx_devices_enabled ON devices(is_enabled);
CREATE INDEX IF NOT EXISTS idx_devices_tags ON devices USING GIN(tags);

-- Device credentials (encrypted at application layer)
CREATE TABLE IF NOT EXISTS device_credentials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    cred_type   VARCHAR(32) NOT NULL DEFAULT 'rtsp'
                CHECK (cred_type IN ('rtsp', 'onvif', 'web', 'ssh', 'api')),
    username    VARCHAR(128),
    password    VARCHAR(255),
    priority    INTEGER DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(device_id, cred_type, priority)
);

-- Device groups
CREATE TABLE IF NOT EXISTS device_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(128) UNIQUE NOT NULL,
    description TEXT,
    color       VARCHAR(7),
    icon        VARCHAR(32),
    sort_order  INTEGER DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Group members
CREATE TABLE IF NOT EXISTS group_members (
    group_id   UUID NOT NULL REFERENCES device_groups(id) ON DELETE CASCADE,
    device_id  UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    sort_order INTEGER DEFAULT 0,
    PRIMARY KEY (group_id, device_id)
);

-- Camera-NVR links
CREATE TABLE IF NOT EXISTS camera_nvr_links (
    camera_id      UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    nvr_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    channel_number INTEGER NOT NULL DEFAULT 1,
    channel_name   VARCHAR(64),
    PRIMARY KEY (camera_id, nvr_id)
);

-- Scan jobs
CREATE TABLE IF NOT EXISTS scan_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    initiated_by UUID REFERENCES users(id),
    scan_type    VARCHAR(16) NOT NULL CHECK (scan_type IN ('full', 'custom', 'quick')),
    target       VARCHAR(64),
    status       VARCHAR(16) NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    device_count INTEGER DEFAULT 0,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT,
    results_meta JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scans_status ON scan_jobs(status);

-- Discovered devices (raw scan results)
CREATE TABLE IF NOT EXISTS discovered_devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_job_id     UUID NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
    ip_address      INET NOT NULL,
    mac_address     MACADDR,
    hostname        VARCHAR(255),
    vendor          VARCHAR(128),
    open_ports      INTEGER[] DEFAULT '{}',
    device_type     VARCHAR(32) DEFAULT 'unknown'
                    CHECK (device_type IN ('camera','nvr','switch','router',
                                           'pc','server','printer','unknown')),
    is_reachable    BOOLEAN DEFAULT true,
    rtsp_found      BOOLEAN DEFAULT false,
    onvif_found     BOOLEAN DEFAULT false,
    web_found       BOOLEAN DEFAULT false,
    web_title       VARCHAR(255),
    fingerprint     JSONB DEFAULT '{}',
    confidence      REAL DEFAULT 0.0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_discovered_scan ON discovered_devices(scan_job_id);
CREATE INDEX IF NOT EXISTS idx_discovered_ip ON discovered_devices(ip_address);

-- Stream sessions
CREATE TABLE IF NOT EXISTS stream_sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    channel      INTEGER DEFAULT 1,
    session_type VARCHAR(16) NOT NULL DEFAULT 'hls'
                 CHECK (session_type IN ('hls', 'webrtc', 'mjpeg', 'snapshot')),
    status       VARCHAR(16) NOT NULL DEFAULT 'starting'
                 CHECK (status IN ('starting', 'active', 'error', 'stopped')),
    hls_path     VARCHAR(255),
    viewer_count INTEGER DEFAULT 0,
    pid          INTEGER,
    error        TEXT,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_streams_device ON stream_sessions(device_id);
CREATE INDEX IF NOT EXISTS idx_streams_status ON stream_sessions(status);

-- Layouts
CREATE TABLE IF NOT EXISTS layouts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    name        VARCHAR(128) NOT NULL,
    grid_cols   INTEGER NOT NULL DEFAULT 2,
    grid_rows   INTEGER NOT NULL DEFAULT 2,
    cells       JSONB NOT NULL DEFAULT '[]',
    is_default  BOOLEAN DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Rotation schedules
CREATE TABLE IF NOT EXISTS rotation_schedules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(128) NOT NULL,
    group_id        UUID REFERENCES device_groups(id) ON DELETE SET NULL,
    layout_mode     VARCHAR(16) NOT NULL DEFAULT '2x2'
                    CHECK (layout_mode IN ('1x1','2x2','3x3','4x4','1x2','2x3')),
    interval_secs   INTEGER NOT NULL DEFAULT 10 CHECK (interval_secs >= 3),
    is_active       BOOLEAN NOT NULL DEFAULT false,
    transition      VARCHAR(16) DEFAULT 'fade'
                    CHECK (transition IN ('fade', 'slide', 'cut')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Rotation cameras (ordered list)
CREATE TABLE IF NOT EXISTS rotation_cameras (
    schedule_id UUID NOT NULL REFERENCES rotation_schedules(id) ON DELETE CASCADE,
    device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (schedule_id, device_id)
);

-- Audit logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    action      VARCHAR(64) NOT NULL,
    resource    VARCHAR(64),
    resource_id UUID,
    details     JSONB DEFAULT '{}',
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);
