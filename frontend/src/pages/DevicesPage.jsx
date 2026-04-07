import React, { useState, useEffect } from 'react';
import { devicesAPI } from '../api/client';
import { useToastStore, useAuthStore } from '../store/store';
import { Plus, Search, Camera, Monitor, ToggleLeft, ToggleRight, Trash2, Edit, X, Wand2 } from 'lucide-react';

export default function DevicesPage() {
  const [devices, setDevices] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [page, setPage] = useState(1);
  const [showModal, setShowModal] = useState(false);
  const [editDevice, setEditDevice] = useState(null);
  const { addToast } = useToastStore();
  const { user } = useAuthStore();
  const isAdmin = user?.role === 'admin';

  const fetchDevices = () => {
    setLoading(true);
    devicesAPI.list({ page, limit: 20, type: typeFilter, search })
      .then(({ data }) => { setDevices(data.data || []); setTotal(data.total); })
      .catch(() => addToast('Failed to load devices', 'error'))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchDevices(); }, [page, typeFilter, search]);

  const handleDelete = async (id) => {
    if (!confirm('Delete this device?')) return;
    try {
      await devicesAPI.delete(id);
      addToast('Device deleted', 'success');
      fetchDevices();
    } catch {
      addToast('Delete failed', 'error');
    }
  };

  const handleToggle = async (id) => {
    try {
      await devicesAPI.toggle(id);
      fetchDevices();
    } catch {
      addToast('Toggle failed', 'error');
    }
  };

  const handleGenerateChannels = async (id, name) => {
    if (!confirm(`Auto-generate cameras for NVR "${name}"?`)) return;
    try {
      const res = await devicesAPI.generateChannels(id);
      addToast(`Successfully created ${res.data.count} cameras`, 'success');
      fetchDevices();
    } catch (err) {
      addToast(err.response?.data?.error?.message || 'Failed to generate channels', 'error');
    }
  };

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Devices</h1>
        {isAdmin && (
          <button className="btn btn-primary" onClick={() => { setEditDevice(null); setShowModal(true); }}>
            <Plus size={16} /> Add Device
          </button>
        )}
      </div>
      <div className="main-body">
        {/* Filters */}
        <div className="flex items-center gap-md mb-lg" style={{ flexWrap: 'wrap' }}>
          <div style={{ position: 'relative', flex: '1', minWidth: 200 }}>
            <Search size={16} style={{ position: 'absolute', left: 12, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
            <input
              className="input"
              placeholder="Search by name, IP, or manufacturer..."
              value={search}
              onChange={(e) => { setSearch(e.target.value); setPage(1); }}
              style={{ paddingLeft: 36 }}
            />
          </div>
          <select className="input" value={typeFilter} onChange={(e) => { setTypeFilter(e.target.value); setPage(1); }} style={{ width: 160 }}>
            <option value="">All Types</option>
            <option value="camera">Cameras</option>
            <option value="nvr">NVRs</option>
          </select>
          <span className="text-sm text-muted">{total} device{total !== 1 ? 's' : ''}</span>
        </div>

        {/* Table */}
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Status</th>
                <th>Name</th>
                <th>Type</th>
                <th>IP Address</th>
                <th>Manufacturer</th>
                <th>Tags</th>
                {isAdmin && <th>Actions</th>}
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={7} className="text-center text-muted">Loading...</td></tr>
              ) : devices.length === 0 ? (
                <tr><td colSpan={7} className="text-center text-muted">No devices found. Add one to get started.</td></tr>
              ) : devices.map((d) => (
                <tr key={d.id}>
                  <td>
                    <span className={`badge ${d.is_enabled ? 'badge-green' : 'badge-red'}`}>
                      <span className="badge-dot" /> {d.is_enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </td>
                  <td style={{ fontWeight: 600, color: 'var(--text-primary)' }}>{d.name}</td>
                  <td>
                    <span className={`badge ${d.device_type === 'camera' ? 'badge-blue' : 'badge-purple'}`}>
                      {d.device_type === 'camera' ? <Camera size={12} /> : <Monitor size={12} />}
                      {d.device_type}
                    </span>
                  </td>
                  <td style={{ fontFamily: 'monospace', fontSize: 'var(--font-size-sm)' }}>{d.ip_address}:{d.port}</td>
                  <td>{d.manufacturer || '—'}</td>
                  <td>
                    {(d.tags || []).map((t, i) => (
                      <span key={i} className="badge badge-amber" style={{ marginRight: 4 }}>{t}</span>
                    ))}
                  </td>
                  {isAdmin && (
                    <td>
                      <div className="flex gap-sm">
                        {d.device_type === 'nvr' && (
                          <button className="btn btn-ghost btn-sm" onClick={() => handleGenerateChannels(d.id, d.name)} title="Auto-Generate Cameras">
                            <Wand2 size={14} color="var(--accent-purple)" />
                          </button>
                        )}
                        <button className="btn btn-ghost btn-sm" onClick={() => handleToggle(d.id)} title="Toggle">
                          {d.is_enabled ? <ToggleRight size={16} color="var(--accent-green)" /> : <ToggleLeft size={16} />}
                        </button>
                        <button className="btn btn-ghost btn-sm" onClick={() => { setEditDevice(d); setShowModal(true); }} title="Edit">
                          <Edit size={14} />
                        </button>
                        <button className="btn btn-ghost btn-sm" onClick={() => handleDelete(d.id)} title="Delete">
                          <Trash2 size={14} color="var(--accent-red)" />
                        </button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {total > 20 && (
          <div className="flex items-center justify-between" style={{ marginTop: 'var(--space-md)' }}>
            <button className="btn btn-secondary btn-sm" onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}>Previous</button>
            <span className="text-sm text-muted">Page {page} of {Math.ceil(total / 20)}</span>
            <button className="btn btn-secondary btn-sm" onClick={() => setPage(p => p + 1)} disabled={page >= Math.ceil(total / 20)}>Next</button>
          </div>
        )}
      </div>

      {/* Add/Edit Modal */}
      {showModal && (
        <DeviceModal
          device={editDevice}
          onClose={() => setShowModal(false)}
          onSave={() => { setShowModal(false); fetchDevices(); }}
        />
      )}
    </>
  );
}

function DeviceModal({ device, onClose, onSave }) {
  const isEditing = !!device;
  const [form, setForm] = useState({
    name: device?.name || '',
    device_type: device?.device_type || 'camera',
    ip_address: device?.ip_address || '',
    port: device?.port || 554,
    manufacturer: device?.manufacturer || '',
    model: device?.model || '',
    location: device?.location || '',
    stream_url: device?.stream_url || '',
    notes: device?.notes || '',
    channel_count: device?.channel_count || 1,
  });
  const [saving, setSaving] = useState(false);
  const { addToast } = useToastStore();

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    try {
      if (isEditing) {
        await devicesAPI.update(device.id, form);
        addToast('Device updated', 'success');
      } else {
        await devicesAPI.create(form);
        addToast('Device added', 'success');
      }
      onSave();
    } catch (err) {
      addToast(err.response?.data?.error?.message || 'Save failed', 'error');
    } finally {
      setSaving(false);
    }
  };

  const set = (key) => (e) => setForm({ ...form, [key]: e.target.value });

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">{isEditing ? 'Edit Device' : 'Add Device'}</span>
          <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={18} /></button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="modal-body">
            <div className="input-group">
              <label className="input-label">Name *</label>
              <input className="input" value={form.name} onChange={set('name')} required placeholder="Front Door Camera" />
            </div>
            <div className="flex gap-md">
              <div className="input-group" style={{ flex: 1 }}>
                <label className="input-label">Type *</label>
                <select className="input" value={form.device_type} onChange={set('device_type')}>
                  <option value="camera">Camera</option>
                  <option value="nvr">NVR</option>
                </select>
              </div>
              <div className="input-group" style={{ flex: 1 }}>
                <label className="input-label">Port</label>
                <input className="input" type="number" value={form.port} onChange={set('port')} />
              </div>
              {form.device_type === 'nvr' && (
                <div className="input-group" style={{ flex: 1 }}>
                  <label className="input-label">Channels</label>
                  <input className="input" type="number" min="1" max="128" value={form.channel_count} onChange={set('channel_count')} />
                </div>
              )}
            </div>
            <div className="input-group">
              <label className="input-label">IP Address *</label>
              <input className="input" value={form.ip_address} onChange={set('ip_address')} required placeholder="192.168.1.50" />
            </div>
            <div className="input-group">
              <label className="input-label">Stream URL (RTSP)</label>
              <input 
                className="input" 
                value={form.stream_url} 
                onChange={set('stream_url')} 
                placeholder={form.device_type === 'nvr' ? "rtsp://user:pass@ip:554/cam/realmonitor..." : "rtsp://user:pass@ip:554/stream1"} 
              />
              {form.device_type === 'nvr' && (
                <span className="text-xs text-muted" style={{ marginTop: 4 }}>
                  Tip: Put NVR credentials in the URL here, then use the Magic Wand icon to auto-generate camera channels!
                </span>
              )}
            </div>
            <div className="flex gap-md">
              <div className="input-group" style={{ flex: 1 }}>
                <label className="input-label">Manufacturer</label>
                <input className="input" value={form.manufacturer} onChange={set('manufacturer')} placeholder="Hikvision" />
              </div>
              <div className="input-group" style={{ flex: 1 }}>
                <label className="input-label">Model</label>
                <input className="input" value={form.model} onChange={set('model')} placeholder="DS-2CD2043G2" />
              </div>
            </div>
            <div className="input-group">
              <label className="input-label">Location</label>
              <input className="input" value={form.location} onChange={set('location')} placeholder="Main Entrance" />
            </div>
            <div className="input-group">
              <label className="input-label">Notes</label>
              <textarea className="input" value={form.notes} onChange={set('notes')} rows={2} placeholder="Additional notes..." style={{ resize: 'vertical' }} />
            </div>
          </div>
          <div className="modal-footer">
            <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={saving}>
              {saving ? 'Saving...' : (isEditing ? 'Update' : 'Add Device')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
