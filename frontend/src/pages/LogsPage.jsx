import React, { useState, useEffect } from 'react';
import { ScrollText, Filter } from 'lucide-react';
import api from '../api/client';

export default function LogsPage() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [actionFilter, setActionFilter] = useState('');

  useEffect(() => {
    setLoading(true);
    const params = {};
    if (actionFilter) params.action = actionFilter;
    api.get('/audit-logs', { params })
      .then(({ data }) => setLogs(data.data || data || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [actionFilter]);

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Audit Logs</h1>
        <ScrollText size={20} color="var(--text-muted)" />
      </div>
      <div className="main-body">
        <div className="flex items-center gap-md mb-lg">
          <Filter size={16} color="var(--text-muted)" />
          <select
            className="input"
            value={actionFilter}
            onChange={(e) => setActionFilter(e.target.value)}
            style={{ width: 200 }}
          >
            <option value="">All Actions</option>
            <option value="device.create">Device Created</option>
            <option value="device.update">Device Updated</option>
            <option value="device.delete">Device Deleted</option>
            <option value="scan.start">Scan Started</option>
            <option value="auth.login">Login</option>
            <option value="user.create">User Created</option>
          </select>
        </div>

        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Action</th>
                <th>Resource</th>
                <th>User</th>
                <th>IP Address</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={6} className="text-center text-muted">Loading logs...</td></tr>
              ) : logs.length === 0 ? (
                <tr><td colSpan={6} className="text-center text-muted">No audit logs found.</td></tr>
              ) : logs.map((log) => (
                <tr key={log.id}>
                  <td style={{ whiteSpace: 'nowrap', fontSize: 'var(--font-size-xs)' }}>
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td>
                    <span className={`badge ${getActionBadge(log.action)}`}>{log.action}</span>
                  </td>
                  <td>{log.resource || '—'}</td>
                  <td>{log.user_id ? log.user_id.substring(0, 8) + '...' : 'system'}</td>
                  <td style={{ fontFamily: 'monospace', fontSize: 'var(--font-size-xs)' }}>{log.ip_address || '—'}</td>
                  <td style={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 'var(--font-size-xs)' }}>
                    {log.details ? JSON.stringify(log.details) : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

function getActionBadge(action) {
  if (action?.includes('create')) return 'badge-green';
  if (action?.includes('delete')) return 'badge-red';
  if (action?.includes('update')) return 'badge-amber';
  if (action?.includes('login')) return 'badge-blue';
  if (action?.includes('scan')) return 'badge-purple';
  return 'badge-blue';
}
