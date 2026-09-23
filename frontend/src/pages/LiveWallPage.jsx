import React, { useState, useEffect, useRef, useCallback } from 'react';
import { devicesAPI, streamsAPI, snapshotsAPI } from '../api/client';
import { Grid2X2, Grid3X3, Square, Maximize, RotateCcw, AlertTriangle, RefreshCw, Play, X } from 'lucide-react';
import Hls from 'hls.js';

// Thumbnails are refreshed (and re-captured) roughly every 30s. This is the
// cheap path that lets a wall show hundreds of cameras without transcoding
// all of them simultaneously.
const SNAPSHOT_REFRESH_MS = 30000;
// The relay reclaims HLS sessions whose Redis lease expires after the idle
// timeout. We renew the lease a little under that (60s default) while a cell
// stays live, so streams are on-demand and free their FFmpeg slot when closed.
const HEARTBEAT_MS = 25000;

export default function LiveWallPage() {
  const [devices, setDevices] = useState([]);
  const [gridMode, setGridMode] = useState('2x2');
  const [rotating, setRotating] = useState(false);
  const [rotationInterval, setRotationInterval] = useState(10);
  const [currentOffset, setCurrentOffset] = useState(0);
  const [liveCells, setLiveCells] = useState({});

  useEffect(() => {
    // No camera-connect limit — pull the whole fleet, not a hard-capped page.
    devicesAPI.list({ type: 'camera', enabled: true, limit: 1000 })
      .then(({ data }) => setDevices(data.data || []))
      .catch(() => {});
  }, []);

  // Rotation logic
  useEffect(() => {
    if (!rotating || devices.length === 0) return;
    const timer = setInterval(() => {
      const gridSize = getGridSize(gridMode);
      setCurrentOffset((prev) => {
        const next = prev + gridSize;
        return next >= devices.length ? 0 : next;
      });
    }, rotationInterval * 1000);
    return () => clearInterval(timer);
  }, [rotating, rotationInterval, devices.length, gridMode]);

  // Reset offset when grid mode changes to prevent blank screens
  useEffect(() => {
    setCurrentOffset(0);
  }, [gridMode]);

  // Clamp offset when device list shrinks
  useEffect(() => {
    if (devices.length > 0 && currentOffset >= devices.length) {
      setCurrentOffset(0);
    }
  }, [devices.length, currentOffset]);

  const toggleLive = useCallback((deviceId) => {
    setLiveCells((prev) => {
      const next = { ...prev };
      if (next[deviceId]) delete next[deviceId];
      else next[deviceId] = Date.now(); // timestamp so starting again resets HLS
      return next;
    });
  }, []);

  const gridSize = getGridSize(gridMode);
  const sliced = devices.slice(currentOffset, currentOffset + gridSize);
  // Pad with nulls — use a new array to avoid mutating state
  const visibleDevices = [...sliced];
  while (visibleDevices.length < gridSize) visibleDevices.push(null);

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Live Wall</h1>
        <div className="flex items-center gap-md">
          {/* Rotation controls */}
          <div className="flex items-center gap-sm">
            <button
              className={`btn ${rotating ? 'btn-primary' : 'btn-secondary'} btn-sm`}
              onClick={() => setRotating(!rotating)}
              title="Auto-rotate cameras"
            >
              <RotateCcw size={14} /> {rotating ? 'Stop' : 'Rotate'}
            </button>
            {rotating && (
              <select
                className="input"
                value={rotationInterval}
                onChange={(e) => setRotationInterval(Number(e.target.value))}
                style={{ width: 80, padding: '4px 8px', fontSize: 'var(--font-size-xs)' }}
              >
                <option value={5}>5s</option>
                <option value={10}>10s</option>
                <option value={15}>15s</option>
                <option value={30}>30s</option>
                <option value={60}>60s</option>
              </select>
            )}
          </div>

          {/* Grid layout toggle */}
          <div className="grid-controls">
            <button className={`grid-btn ${gridMode === '1x1' ? 'active' : ''}`} onClick={() => setGridMode('1x1')} title="1×1">
              <Square size={14} />
            </button>
            <button className={`grid-btn ${gridMode === '2x2' ? 'active' : ''}`} onClick={() => setGridMode('2x2')} title="2×2">
              <Grid2X2 size={14} />
            </button>
            <button className={`grid-btn ${gridMode === '3x3' ? 'active' : ''}`} onClick={() => setGridMode('3x3')} title="3×3">
              <Grid3X3 size={14} />
            </button>
            <button className={`grid-btn ${gridMode === '4x4' ? 'active' : ''}`} onClick={() => setGridMode('4x4')} title="4×4">
              <Maximize size={14} />
            </button>
          </div>
        </div>
      </div>
      <div className="main-body" style={{ padding: 'var(--space-sm)', height: 'calc(100vh - 60px)' }}>
        <div className={`video-grid grid-${gridMode}`} style={{ height: '100%' }}>
          {visibleDevices.map((device, i) => (
            <VideoCell
              key={device?.id || `empty-${i}`}
              device={device}
              live={!!device && !!liveCells[device.id]}
              onToggleLive={device ? () => toggleLive(device.id) : undefined}
            />
          ))}
        </div>
      </div>
    </>
  );
}

function VideoCell({ device, live, onToggleLive }) {
  const videoRef = useRef(null);
  const hlsRef = useRef(null);
  const sessionIdRef = useRef(null);
  const tokenRef = useRef(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [retryKey, setRetryKey] = useState(0);
  const [snapTs, setSnapTs] = useState(Date.now());

  // Mint (and refresh) a short-lived relay stream token for this device. The
  // token is sent as X-Stream-Token on relay calls instead of the raw RTSP URL,
  // which may embed credentials and must never leave the backend.
  const getToken = useCallback(async () => {
    const { data } = await streamsAPI.token(device.id);
    tokenRef.current = data.token;
    setTimeout(() => { tokenRef.current = null; }, Math.max(data.expires_in - 10, 5) * 1000);
    return data.token;
  }, [device]);

  // ---- Thumbnail mode: request a fresh capture and poll the image ----
  useEffect(() => {
    if (live || !device) return;
    if (device.stream_configured) {
      getToken()
        .then((token) => snapshotsAPI.capture(device.id, token).catch(() => {}))
        .catch(() => {});
    }
    const timer = setInterval(() => setSnapTs(Date.now()), SNAPSHOT_REFRESH_MS);
    return () => clearInterval(timer);
  }, [live, device, getToken, retryKey]);

  const startLive = useCallback(() => {
    if (!device) return;
    if (!device.stream_configured) {
      setError('No RTSP stream URL configured');
      return;
    }

    setLoading(true);
    setError(null);

    getToken()
      .then((token) => streamsAPI.start(device.id, 1, token))
      .then(({ data }) => {
        setLoading(false);
        sessionIdRef.current = data.session_id;
        const hlsUrl = data.hls_url;

        if (!videoRef.current) return;

        if (Hls.isSupported()) {
          const hls = new Hls({ enableWorker: true, lowLatencyMode: true, maxBufferLength: 5 });
          hls.loadSource(hlsUrl);
          hls.attachMedia(videoRef.current);
          hls.on(Hls.Events.ERROR, (event, errData) => {
            if (errData.fatal) {
              console.warn('HLS error for', device.name, errData);
              setError('HLS playback error — click Retry');
            }
          });
          hlsRef.current = hls;
        } else if (videoRef.current.canPlayType('application/vnd.apple.mpegurl')) {
          videoRef.current.src = hlsUrl;
        }
      })
      .catch((err) => {
        setLoading(false);
        setError('Failed to start stream relay');
        console.error('Relay error:', err);
      });
  }, [device, getToken, retryKey]);

  // ---- Start / stop the live HLS session when the cell is toggled ----
  useEffect(() => {
    if (!live) {
      // Leaving live mode — tear down the HLS player and tell the relay to stop.
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      if (videoRef.current) videoRef.current.src = '';
      const sid = sessionIdRef.current;
      sessionIdRef.current = null;
      if (sid) streamsAPI.stop(sid).catch(() => {});
      return;
    }

    startLive();

    return () => {
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      if (videoRef.current) videoRef.current.src = '';
      const sid = sessionIdRef.current;
      sessionIdRef.current = null;
      if (sid) streamsAPI.stop(sid, tokenRef.current).catch(() => {});
    };
  }, [live, startLive]);

  // ---- Keep the session lease alive while the cell is live ----
  useEffect(() => {
    if (!live) return;
    const timer = setInterval(() => {
      if (sessionIdRef.current) {
        streamsAPI.heartbeat(sessionIdRef.current, tokenRef.current).catch(() => {});
      }
    }, HEARTBEAT_MS);
    return () => clearInterval(timer);
  }, [live]);

  const handleRetry = () => {
    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }
    setError(null);
    setLoading(false);
    sessionIdRef.current = null;
    setRetryKey((k) => k + 1);
  };

  if (!device) {
    return (
      <div className="video-cell">
        <div className="video-cell-empty">No Camera</div>
      </div>
    );
  }

  return (
    <div className="video-cell">
      {live ? (
        <>
          {error ? (
            <div className="video-cell-empty flex-col gap-sm" style={{ color: 'var(--accent-red)' }}>
              <AlertTriangle size={24} />
              <span className="text-sm">{error}</span>
              <button className="btn btn-secondary btn-sm" onClick={handleRetry} style={{ marginTop: 'var(--space-xs)' }}>
                <RefreshCw size={12} /> Retry
              </button>
            </div>
          ) : (
            <video ref={videoRef} muted autoPlay playsInline style={{ background: '#000' }} />
          )}

          {loading && !error && (
            <div className="absolute-center pointer-events-none p-md" style={{ background: 'rgba(0,0,0,0.5)', borderRadius: 'var(--radius-md)' }}>
              <span className="text-sm">Connecting to stream...</span>
            </div>
          )}

          {/* Back-to-grid overlay */}
          <button
            className="video-cell-action"
            onClick={onToggleLive}
            title="Back to grid"
            style={{ position: 'absolute', top: 8, right: 8, zIndex: 5 }}
          >
            <X size={14} />
          </button>
        </>
      ) : (
        <>
          {device.stream_configured ? (
            <img
              className="video-cell-thumb"
              src={snapshotsAPI.url(device.id, snapTs)}
              alt={device.name}
            />
          ) : (
            <div className="video-cell-empty flex-col gap-sm">
              <AlertTriangle size={18} />
              <span className="text-xs">No RTSP configured</span>
            </div>
          )}

          {onToggleLive && (
            <button className="video-cell-play" onClick={onToggleLive} title="Watch live">
              <Play size={18} fill="currentColor" />
            </button>
          )}
        </>
      )}

      <div className="video-cell-label">
        {device.name}
        <span style={{ float: 'right', opacity: 0.7, fontSize: '0.65rem' }}>{device.ip_address}</span>
      </div>
      <div className="video-cell-status">
        <span className={`badge ${error ? 'badge-red' : loading ? 'badge-amber' : live ? 'badge-green' : 'badge-gray'}`} style={{ fontSize: '0.6rem' }}>
          <span className="badge-dot" /> {error ? 'ERROR' : loading ? 'CONNECTING' : live ? 'LIVE' : 'SNAPSHOT'}
        </span>
        {!live && onToggleLive && (
          <span className="text-xs" style={{ float: 'right', opacity: 0.7 }}>
            click to view
          </span>
        )}
      </div>
    </div>
  );
}

function getGridSize(mode) {
  const sizes = { '1x1': 1, '2x2': 4, '3x3': 9, '4x4': 16 };
  return sizes[mode] || 4;
}