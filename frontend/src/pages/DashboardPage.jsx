import React, { useState, useEffect } from 'react';
import { dashboardAPI } from '../api/client';
import { Camera, Monitor, Wifi, Radio, Users, FolderOpen } from 'lucide-react';

export default function DashboardPage() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    dashboardAPI.stats()
      .then(({ data }) => setStats(data))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Dashboard</h1>
        <span className="text-sm text-muted">System Overview</span>
      </div>
      <div className="main-body">
        {loading ? (
          <div className="text-center text-muted animate-pulse" style={{ paddingTop: 'var(--space-2xl)' }}>
            Loading dashboard...
          </div>
        ) : (
          <>
            <div className="stats-grid">
              <StatCard icon={Camera} label="Cameras" value={stats?.total_cameras || 0} color="blue" />
              <StatCard icon={Monitor} label="NVRs" value={stats?.total_nvrs || 0} color="purple" />
              <StatCard icon={Radio} label="Active Streams" value={stats?.active_streams || 0} color="green" />
              <StatCard icon={Wifi} label="Enabled Devices" value={stats?.enabled_devices || 0} color="cyan" />
              <StatCard icon={FolderOpen} label="Groups" value={stats?.total_groups || 0} color="amber" />
              <StatCard icon={Users} label="Total Devices" value={stats?.total_devices || 0} color="red" />
            </div>

            <div className="card">
              <div className="card-header">
                <span className="card-title">Quick Info</span>
              </div>
              <div style={{ color: 'var(--text-secondary)', fontSize: 'var(--font-size-sm)' }}>
                <p>Last scan: {stats?.last_scan_at ? new Date(stats.last_scan_at).toLocaleString() : 'Never'}</p>
                <p style={{ marginTop: 'var(--space-sm)' }}>
                  Navigate to <strong>Devices</strong> to manage cameras and NVRs, 
                  <strong> Live Wall</strong> to view streams, or 
                  <strong> Scan Center</strong> to discover network devices.
                </p>
              </div>
            </div>
          </>
        )}
      </div>
    </>
  );
}

function StatCard({ icon: Icon, label, value, color }) {
  return (
    <div className="stat-card">
      <div className={`stat-icon ${color}`}>
        <Icon size={22} />
      </div>
      <div>
        <div className="stat-value">{value}</div>
        <div className="stat-label">{label}</div>
      </div>
    </div>
  );
}
