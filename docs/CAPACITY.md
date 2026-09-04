# CamGuid Capacity & Sizing Guide

Sizing guidance for the enterprise use case in `plan.md`:
**600+ cameras attached to 20+ NVRs, all PoE-powered, added and managed
through the NVRs rather than camera-by-camera.**

The platform is designed around two load-reducing ideas:

1. **NVR-first management** — configure one NVR credential, auto-discover all
   of its camera channels. Cost of onboarding is `O(NVRs)`, not `O(cameras)`.
2. **Snapshot grid + on-demand HLS** — the Live Wall shows cheap single-frame
   thumbnails for all cameras; a full HLS transcode only runs while a user is
   actually watching. FFmpeg usage tracks *viewers*, not camera count.

---

## 1. Bandwidth

Per-camera realistic bitrates (H.264):

| Stream            | Bitrate      | Notes                             |
|-------------------|--------------|-----------------------------------|
| Sub-stream        | 0.5 – 1 Mbps | Low-res; fine for thumbnails      |
| Main stream       | 2 – 6 Mbps   | 1080p; higher for 4K / H.265      |

Fleet totals:

| Scenario                                    | Bite estimate    | Verdict                        |
|---------------------------------------------|------------------|--------------------------------|
| All 600 on sub-stream (snapshot wall)        | ~600 Mbps        | Needs ~1 Gbit uplinks to relays|
| All 600 on main stream                      | ~2.4 – 3.6 Gbps  | Exceeds a single 1 Gbit link   |
| On-demand live (≤60 concurrent viewers @main)| ≤240 – 360 Mbps  | Comfortable on 1 Gbit          |

**Rule of thumb:** never pull all main streams permanently. The snapshot +
on-demand model keeps the fleet-wide demand proportional to active viewers.
RTP/RTSP is pulled *TCP* by the relay (`-rtsp_transport tcp`), so no UDP
broadcast storms; flows are point-to-point NVR→relay.

**PoE switch uplinks:** cameras attach to PoE switches per NVR. Ensure each
NVR-to-core uplink carries the aggregate of the cameras it serves (≈ channels ×
main bitrate) if that NVR is heavily watched. For typical 32-ch NVRs:
32 × 4 Mbps ≈ 128 Mbps → 1 Gbit uplink is ample headroom.

## 2. CPU / Relay sessions

Each on-demand HLS session uses FFmpeg with `-c:v copy` — **no re-transcode**,
so CPU cost is tiny (~1–3% of a core per stream). Re-encoding (unused by
default) costs ~1 core per 1–2 streams.

| Component                          | Capacity per CPU core (approx.) |
|------------------------------------|---------------------------------|
| HLS stream copy session            | 30 – 60 sessions                |
| Snapshot single-frame grab (640px) | ~10–20 per second staggered     |
| Snapshots for 600 cameras / 30 s   | ~20 grabs/s average → ~2–4 cores|

**Concurrency math:** default `STREAM_MAX_SESSIONS=20` per replica × 3 relay
replicas = **60 concurrent live streams** — enough for a 600-camera fleet where
thumbnails cover the rest. Expand by raising the env var or adding replicas
(`RELAY_N`).

## 3. Memory

- FFmpeg stream-copy session on HLS: ~100–200 MB (5 × segment buffers).
  20 sessions/replica ≈ 2–4 GB → give each relay 1–2 GB with headroom.
- Snapshot FFmpeg processes are short-lived (1 frame) and bounded by the
  `snapshotCaptureTimeout`, so they don't accumulate.
- Redis/Postgres are tiny at this scale (session registry + heartbeat keys are
  Kb-level).

## 4. Storage

- HLS segments are deleted after playback (`delete_segments`); no long-term
  growth on `stream_segments`.
- Snapshots are ~20–50 KB each at 640 px. A full 600-camera rotation ≈ 24 MB.
  Snapshot retention is operator-managed (files are overwritten in place).
- Camera recording/retention is done by the **NVRs themselves** — CamGuid is a
  live-viewing/management layer, not a recorder.

## 5. Recommended deployment (600 cameras / 20 NVRs)

| Service        | Replicas | Sizing             | Notes                               |
|----------------|----------|--------------------|-------------------------------------|
| Relay          | 3        | 0.5–1 vCPU, 1–2 GB | Round-robin via nginx; shared volumes|
| NVR service    | 1–2      | 0.5–1 vCPU, 1 GB   | Onboarding/discovery is O(NVRs)     |
| API (Go)       | 1–2      | 1 vCPU, 1–2 GB     | Auth, metadata, orchestration       |
| Scanner        | 1        | low                | Ad-hoc LAN scans on demand          |
| Postgres 16    | 1        | 1–2 GB             | Metadata only; no video             |
| Redis 7        | 1        | 128–256 MB         | Heartbeats, session registry, jobs  |
| nginx          | 1        | low                | HLS/snapshot static serving + proxy |

Host-level network: 1–10 Gbit NIC recommended. Peak LAN demand during live
viewing ≈ active viewers × main bitrate (bounded, e.g. 60 × 4 Mbps ≈ 240 Mbps).

## 6. When to scale further

| Symptom                              | Fix                                             |
|--------------------------------------|-------------------------------------------------|
| `429 Maximum stream sessions reached`| Raise `STREAM_MAX_SESSIONS` or add a replica    |
| Relay CPU > 70%                      | Add a replica; prefer sub-stream for the wall   |
| Live Wall thumbs lag > interval      | Raise snapshot concurrency / shorten `-ss`      |
| WAN (remote viewers) saturating      | Serve HLS through nginx with CDN/caching        |

## 7. Operational notes

- **Per-replica observability:** each relay exposes `GET /metrics`
  (Prometheus text) and `GET /health` (instance id + local session count).
  Useful dashboards: `camguid_relay_sessions_active`,
  `camguid_relay_sessions_registered` (global),
  `camguid_relay_snapshot_failures_total`.
- **Fleet-wide stream status** is available on any replica via
  `GET /stream/api/sessions` (Redis registry view).
- **Idle reclaim:** sessions with no viewer heartbeat are stopped after
  `STREAM_IDLE_TIMEOUT` (default 60 s) and reported via
  `camguid_relay_idle_reclaims_total`.
- Keep `FRONTEND` heartbeat interval (< idle timeout) so genuinely-viewed
  streams are never reclaimed (frontend default: 25 s).