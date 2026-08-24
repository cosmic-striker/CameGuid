# CamGuid Scale-Up Plan — Enterprise CCTV Federation (600+ cameras / 20+ NVRs)

## 1. Problem Statement

A large enterprise operates multiple physical networks (buildings/floors/warehouses)
across the workspace. Each network has its own Video Surveillance deployment:

- **600+ IP cameras** (all PoE-powered, attached to the network switches)
- **20+ NVRs** that aggregate those cameras (e.g., 32 channels each)

The operator needs ONE central software platform to view and manage ALL feeds.

**Critical constraint — why we connect NVRs, not cameras:**
Individually configuring 600+ cameras means entering each camera's IP +
username + password + vendor manually — effectively impossible and
unmaintainable. But configuring **20 NVRs is trivial** (one IP + one
credential each). The software must therefore:

1. Treat the **NVR as the unit of management** (not the camera).
2. Auto-discover every camera channel attached to each NVR.
3. Expose all **600+ channels simultaneously** with **no camera-connect limit**
   and no per-camera credential entry.
4. Support **all 20+ NVRs** with no NVR-count limit.
5. Remain responsive on a single server within one "private" LAN.

## 2. Feasibility: Is it possible via NVR only?

**YES — and it is the industry-standard design.** An NVR exposes every
attached camera channel over RTSP. With one NVR credential you can reach
every camera that NVR manages:

| Vendor | Main stream URL pattern (channel N)      |
|--------|------------------------------------------|
| Hikvision | `rtsp://user:pass@ip:554/Streaming/Channels/{N}01` |
| Dahua    | `rtsp://user:pass@ip:554/cam/realmonitor?channel={N}&subtype=0` |
| Generic  | configurable pattern                       |

Capacity math: 20 NVRs × 32 channels = **640 channels ≥ 600 cameras**. ✅
Credentials stay secure (Fernet-encrypted in DB; HMAC-signed stream tokens
sent to browsers — raw RTSP creds never exposed).

## 3. Current-State Review (what already exists)

- **nvr-service (Python/FastAPI):** NVR CRUD, per-vendor RTSP URL builders,
  `generate_channels()` auto-creates channels 1..channel_count, password
  encryption, FFprobe RTSP validation, signed stream tokens.
- **relay (Go):** on-demand RTSP→HLS FFmpeg transcode per camera, auto-retry,
  `STREAM_MAX_SESSIONS=20`, HLS static serving via nginx.
- **scanner (Go):** LAN auto-discovery (ARP/port/ONVIF probe planned).
- **frontend (React/Vite):** Live Wall grid, Devices, Scan Center.
- **Current design already meets the core requirement** (NVR-first, no hard
  channel cap). The gaps below are **scale**, not architecture.

## 4. Scale Gaps at 600+ cameras (what must be built)

| # | Gap | Impact |
|---|-----|--------|
| 1 | `channel_count` entered manually | Requires knowing NVR capacity; error-prone |
| 2 | Relay: 1 FFmpeg process per active stream | 600 simultaneous = host exhaustion |
| 3 | No snapshot/thumbnail generator | Live Wall can't show 600 cams cheaply |
| 4 | `cleanupLoop` idle detection is a no-op | Sessions never freed → cap hit at 20 |
| 5 | Single relay instance | No headroom / HA for 600 channels |
| 6 | Bandwidth not budgeted | 600 × 2–8 Mbps can saturate uplinks |

## 5. Implementation Plan

### Phase 1 — Auto channel discovery (NVR-first, zero manual counting)
- Add ONVIF WS-Discovery / vendor API enumeration (Hikvision ISAPI
  `/ISAPI/ContentMgmt/search` + Dahua CGI) in `nvr-service` to return the
  **real list of connected channels** and NVR max channel count.
- New endpoint `POST /api/v1/nvr/{id}/discover` → stores channels with real
  names/locations; falls back to `generate_channels()` for "generic".
- Enforce **no channel cap** (int already unbounded — keep it).

### Phase 2 — Snapshot grid + on-demand HLS (the Live Wall fix)
- **Snapshot worker** (in relay): periodic FFmpeg single-frame capture per
  channel → `/tmp/snapshots/{channel_id}.jpg` (e.g., every 30s, low-res).
- Live Wall renders **thumbnails for all 600+**; clicking a cell starts ONE
  on-demand HLS session (existing `/start`) — 20 concurrent sessions is
  sufficient by design.
- Implement real idle-timeout in `cleanupLoop` (heartbeats via Redis) so
  sessions free up.

### Phase 3 — Relay scale-out & HA
- Config-driven `STREAM_MAX_SESSIONS` per instance + N relay replicas behind
  nginx (round-robin); token/HLS served from shared `stream_segments` volume
  already in place.
- Move session registry to Redis so any replica can stop/observe sessions.

### Phase 4 — Tests, observability, ops
- Unit tests: auto-discovery parsing (Hik/Dahua fixtures), snapshot worker,
  session limits.
- Live Wall "Retry" button + rotation fixes (from implementation_plan.md).
- Capacity doc: recommended NVR/relay ratio, PoE uplink sizing
  (600×sub-stream≈0.5–1.5 Gbps; use sub-stream for the wall).

## 6. Verification Plan
1. Add 1 mock/real NVR → auto-discover lists correct cameras (not 1..N).
2. Live Wall with 600 fake channels: all thumbnail cells render, clicking
   streams, idle timeout frees slots, retry recovers failed streams.
3. `NVR_STREAM_MAX_SESSIONS` respected; 2nd relay replica picks up load.
4. `cd nvr-service && pytest` and backend `go test ./...` pass.

## 7. Risks & Mitigations
- **CPU**: 600 live transcodes impossible on 1 box → snapshot+on-demand model.
- **Bandwidth**: mitigated by sub-stream thumbnails + NVR-side aggregation.
- **Vendor API drift**: isolation behind `VendorStrategy`; generic fallback.

## 8. Progress

### Phase 1 — Auto channel discovery ✅ implemented
- `POST /api/v1/nvr/{id}/discover` enumerates real connected cameras:
  - **Hikvision** — ISAPI `InputProxy/channels` (names/enabled/online), Digest auth.
  - **Dahua** — CGI `configManager?name=ChannelTitle` (filters empty slots).
  - **Generic** — falls back to `channel_count` generation.
- New: `discovery_service`, vendor `discover_channels` hooks, RFC 7616 Digest
  HTTP client, `DiscoverChannelsResponse`. `channel_count` auto-syncs to the
  real detected count. Tested (`nvr-service/tests/test_vendor_discovery.py`).

### Phase 2 — Snapshot grid + on-demand HLS ✅ implemented
- **Relay (Go)**: `POST /snapshot/capture` (one-shot) and `POST /snapshot/start`
  (periodic) FFmpeg single-frame jobs → `/tmp/snapshots/{id}.jpg`; `GET
  /snapshot/jobs`, `DELETE /snapshot/stop/{id}`.
- **Idle reclaim**: HLS sessions get a Redis lease; `POST /heartbeat/{id}`
  renews it while viewed; `cleanupLoop` now genuinely stops leases that expire,
  so FFmpeg usage tracks viewers (~`STREAM_MAX_SESSIONS`), not camera count.
- **Live Wall (React)**: default = snapshot thumbnails (cheap, scale to
  hundreds); click a cell → on-demand HLS for that one camera; heartbeats keep
  it alive while open; device list limit raised to 1000 (no camera cap).
- Config: `CAMGUID_SNAPSHOT_INTERVAL`, `CAMGUID_SNAPSHOT_WIDTH`. Relay tests +
  race-fixed (`backend/cmd/relay/snapshot_test.go`).

### Remaining
- **Phase 4** — observability (metrics/logs per replica) + capacity sizing doc.

### Phase 3 — Relay scale-out & HA ✅ implemented
- **Redis session registry**: every HLS session publishes metadata to
  `camguid:session:*`. Any replica can observe all sessions (`GET /sessions`
  global view), renew heartbeats for sessions it doesn't own, and stop remote
  sessions via the `camguid:relay:stop` pub/sub channel.
- **Crash recovery**: `cleanupLoop` sweeps orphaned registry entries left by
  dead replicas and removes their shared HLS files.
- **Infra**: `docker-compose.yml` now runs `relay`, `relay-2`, `relay-3`
  replicas sharing the `stream_segments` + `snapshots` volumes, and nginx
  round-robins `/stream/api/` across the pool. Effective concurrency =
  replicas × `STREAM_MAX_SESSIONS`.
- Tests: registry key/round-trip + relay suite with `-race`.