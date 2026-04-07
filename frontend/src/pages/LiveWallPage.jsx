import React, { useState, useEffect, useRef, useCallback } from 'react';
import { devicesAPI, streamsAPI } from '../api/client';
import { Grid2X2, Grid3X3, Square, Maximize, RotateCcw, AlertTriangle, RefreshCw } from 'lucide-react';
import Hls from 'hls.js';

export default function LiveWallPage() {
  const [devices, setDevices] = useState([]);
  const [gridMode, setGridMode] = useState('2x2');
  const [rotating, setRotating] = useState(false);
  const [rotationInterval, setRotationInterval] = useState(10);
  const [currentOffset, setCurrentOffset] = useState(0);

  useEffect(() => {
    devicesAPI.list({ type: 'camera', enabled: true, limit: 100 })
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
            <VideoCell key={device?.id || `empty-${i}`} device={device} />
          ))}
        </div>
      </div>
    </>
  );
}

function VideoCell({ device }) {
  const videoRef = useRef(null);
  const hlsRef = useRef(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [retryKey, setRetryKey] = useState(0);

  const startStream = useCallback(() => {
    if (!device) return;
    if (!device.stream_url) {
      setError('No RTSP stream URL configured');
      return;
    }

    let localSessionId = null;
    let hls = null;
    setLoading(true);
    setError(null);

    streamsAPI.start({
      device_id: device.id,
      channel: 1,
      stream_url: device.stream_url,
    })
      .then(({ data }) => {
        setLoading(false);
        localSessionId = data.session_id;
        const hlsUrl = data.hls_url;

        if (!videoRef.current) return;

        if (Hls.isSupported()) {
          hls = new Hls({ enableWorker: true, lowLatencyMode: true, maxBufferLength: 5 });
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

    return () => {
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      if (localSessionId) {
        streamsAPI.stop(localSessionId).catch(() => {});
      }
    };
  }, [device, retryKey]);

  useEffect(() => {
    const cleanup = startStream();
    return cleanup;
  }, [startStream]);

  const handleRetry = () => {
    // Destroy existing HLS instance
    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }
    setError(null);
    setLoading(false);
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

      <div className="video-cell-label">
        {device.name}
        <span style={{ float: 'right', opacity: 0.7, fontSize: '0.65rem' }}>{device.ip_address}</span>
      </div>
      <div className="video-cell-status">
        <span className={`badge ${error ? 'badge-red' : loading ? 'badge-amber' : 'badge-green'}`} style={{ fontSize: '0.6rem' }}>
          <span className="badge-dot" /> {error ? 'ERROR' : loading ? 'CONNECTING' : 'LIVE'}
        </span>
      </div>
    </div>
  );
}

function getGridSize(mode) {
  const sizes = { '1x1': 1, '2x2': 4, '3x3': 9, '4x4': 16 };
  return sizes[mode] || 4;
}
