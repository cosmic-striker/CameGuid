import React, { useState, useEffect } from 'react';
import { scansAPI, devicesAPI } from '../api/client';
import { useToastStore, useAuthStore } from '../store/store';
import { Wifi, Search, Download, AlertCircle, CheckCircle2 } from 'lucide-react';

export default function ScanCenterPage() {
  const [scans, setScans] = useState([]);
  const [activeScan, setActiveScan] = useState(null);
  const [scanResults, setScanResults] = useState([]);
  const [scanType, setScanType] = useState('full');
  const [startIp, setStartIp] = useState('');
  const [endIp, setEndIp] = useState('');
  const [ports, setPorts] = useState('');
  const [target, setTarget] = useState('');
  const [autoImport, setAutoImport] = useState(false);
  const [loading, setLoading] = useState(false);
  const { addToast } = useToastStore();
  const { user } = useAuthStore();
  const isAdmin = user?.role === 'admin';

  useEffect(() => {
    scansAPI.list().then(({ data }) => {
      const arr = data.data || data;
      setScans(Array.isArray(arr) ? arr : []);
    }).catch(() => {});
  }, []);

  const startScan = async () => {
    if (!isAdmin) return;
    setLoading(true);
    try {
      const payload = { scan_type: scanType };
      if (scanType === 'custom') {
        if (startIp && endIp) {
            payload.start_ip = startIp;
            payload.end_ip = endIp;
            payload.target = `${startIp}-${endIp}`; // For history display
        } else if (target) {
            payload.target = target;
        }
        if (ports) payload.ports = ports;
      }
      if (autoImport) {
        payload.auto_import = true;
      }
      const { data } = await scansAPI.start(payload);
      addToast(`Scan started (ID: ${data.id || data.job_id})`, 'info');
      setActiveScan(data.id || data.job_id);
      // Poll for results
      pollScan(data.id || data.job_id);
    } catch (err) {
      addToast(err.response?.data?.error?.message || 'Failed to start scan', 'error');
    } finally {
      setLoading(false);
    }
  };

  const pollScan = (id) => {
    const interval = setInterval(() => {
      scansAPI.get(id).then(({ data }) => {
        if (data.status === 'completed' || data.status === 'failed') {
          clearInterval(interval);
          setActiveScan(null);
          addToast(`Scan ${data.status}. Found ${data.device_count || 0} devices.`, data.status === 'completed' ? 'success' : 'error');
          if (data.discovered_devices) setScanResults(data.discovered_devices);
          scansAPI.list().then(({ data }) => {
            const arr = data.data || data;
            setScans(Array.isArray(arr) ? arr : []);
          }).catch(() => {});
        }
      }).catch(() => clearInterval(interval));
    }, 3000);
  };

  const importDevice = async (dev) => {
    try {
      await devicesAPI.create({
        name: dev.hostname || `Device-${dev.ip_address}`,
        device_type: dev.device_type === 'camera' || dev.device_type === 'nvr' ? dev.device_type : 'camera',
        ip_address: dev.ip_address,
        manufacturer: dev.vendor || '',
      });
      addToast(`Imported ${dev.ip_address} as managed device`, 'success');
    } catch (err) {
      addToast('Import failed: ' + (err.response?.data?.error?.message || 'Unknown error'), 'error');
    }
  };

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Scan Center</h1>
      </div>
      <div className="main-body">
        {/* Scan Controls */}
        {isAdmin && (
          <div className="card mb-lg">
            <div className="card-header">
              <span className="card-title">Start Network Scan</span>
              <Wifi size={18} color="var(--accent-cyan)" />
            </div>
            <div className="flex gap-md items-center" style={{ flexWrap: 'wrap' }}>
              <select className="input" value={scanType} onChange={(e) => setScanType(e.target.value)} style={{ width: 160 }}>
                <option value="full">Full LAN Scan</option>
                <option value="custom">Custom Range</option>
                <option value="quick">Quick Scan</option>
              </select>
              {scanType === 'custom' && (
                <>
                  <input
                    className="input"
                    placeholder="Start IP (e.g. 192.168.1.1)"
                    value={startIp}
                    onChange={(e) => setStartIp(e.target.value)}
                    style={{ width: 190 }}
                  />
                  <span className="text-muted text-sm px-xs">to</span>
                  <input
                    className="input"
                    placeholder="End IP (e.g. 192.168.1.50)"
                    value={endIp}
                    onChange={(e) => setEndIp(e.target.value)}
                    style={{ width: 190 }}
                  />
                  <input
                    className="input"
                    placeholder="Ports (e.g. 80,554)"
                    value={ports}
                    onChange={(e) => setPorts(e.target.value)}
                    title="Comma-separated ports to scan"
                    style={{ width: 160 }}
                  />
                </>
              )}
              <button className="btn btn-primary" onClick={startScan} disabled={loading || activeScan}>
                <Search size={14} /> {activeScan ? 'Scanning...' : 'Start Scan'}
              </button>
              {activeScan && (
                <button
                  className="btn btn-secondary"
                  onClick={async () => {
                    try {
                      await scansAPI.cancel(activeScan);
                      addToast('Scan cancelled', 'info');
                      setActiveScan(null);
                    } catch {
                      addToast('Failed to cancel scan', 'error');
                    }
                  }}
                >
                  Cancel
                </button>
              )}
            </div>
            
            <div style={{ marginTop: 'var(--space-md)' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-xs)', cursor: 'pointer', userSelect: 'none' }}>
                <input 
                  type="checkbox" 
                  checked={autoImport} 
                  onChange={(e) => setAutoImport(e.target.checked)} 
                  style={{ width: 16, height: 16, cursor: 'pointer' }}
                />
                <span className="text-sm">Auto-import discovered cameras/NVRs to Device Registry</span>
              </label>
            </div>

            {activeScan && (
              <div style={{ marginTop: 'var(--space-md)' }}>
                <div className="progress-bar">
                  <div className="progress-fill animate-pulse" style={{ width: '70%' }} />
                </div>
                <p className="text-sm text-muted" style={{ marginTop: 'var(--space-xs)' }}>
                  Scanning network... This may take up to a minute.
                </p>
              </div>
            )}
          </div>
        )}

        {/* Scan Results */}
        {scanResults.length > 0 && (
          <div className="card mb-lg">
            <div className="card-header">
              <span className="card-title">Latest Scan Results ({scanResults.length} devices)</span>
            </div>
            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <th>IP Address</th>
                    <th>Hostname</th>
                    <th>Vendor</th>
                    <th>Type</th>
                    <th>Open Ports</th>
                    <th>RTSP</th>
                    <th>Confidence</th>
                    {isAdmin && <th>Import</th>}
                  </tr>
                </thead>
                <tbody>
                  {scanResults.map((dev) => (
                    <tr key={dev.id || dev.ip_address}>
                      <td style={{ fontFamily: 'monospace' }}>{dev.ip_address}</td>
                      <td>{dev.hostname || '—'}</td>
                      <td>{dev.vendor || '—'}</td>
                      <td>
                        <span className={`badge ${getTypeBadge(dev.device_type)}`}>{dev.device_type}</span>
                      </td>
                      <td style={{ fontFamily: 'monospace', fontSize: 'var(--font-size-xs)' }}>
                        {(dev.open_ports || []).join(', ') || '—'}
                      </td>
                      <td>
                        {dev.rtsp_found ? <CheckCircle2 size={14} color="var(--accent-green)" /> : <AlertCircle size={14} color="var(--text-muted)" />}
                      </td>
                      <td>{Math.round((dev.confidence || 0) * 100)}%</td>
                      {isAdmin && (
                        <td>
                          <button className="btn btn-primary btn-sm" onClick={() => importDevice(dev)}>
                            <Download size={12} /> Import
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* Scan History */}
        <div className="card">
          <div className="card-header">
            <span className="card-title">Scan History</span>
          </div>
          {scans.length === 0 ? (
            <p className="text-sm text-muted">No scans yet. Run a scan to discover network devices.</p>
          ) : (
            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <th>Date</th>
                    <th>Type</th>
                    <th>Target</th>
                    <th>Status</th>
                    <th>Devices Found</th>
                  </tr>
                </thead>
                <tbody>
                  {scans.map((s) => (
                    <tr key={s.id}>
                      <td>{new Date(s.created_at).toLocaleString()}</td>
                      <td className="badge badge-blue">{s.scan_type}</td>
                      <td style={{ fontFamily: 'monospace' }}>{s.target || 'auto'}</td>
                      <td>
                        <span className={`badge ${s.status === 'completed' ? 'badge-green' : s.status === 'failed' ? 'badge-red' : 'badge-amber'}`}>
                          {s.status}
                        </span>
                      </td>
                      <td>{s.device_count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

function getTypeBadge(deviceType) {
  const badgeMap = { camera: 'badge-blue', nvr: 'badge-purple', switch: 'badge-cyan', router: 'badge-amber', pc: 'badge-green', server: 'badge-green' };
  return badgeMap[deviceType] || 'badge-blue';
}
