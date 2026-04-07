import React, { useState, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, NavLink, useNavigate } from 'react-router-dom';
import { useAuthStore, useToastStore } from './store/store';
import { authAPI } from './api/client';
import LoginPage from './pages/LoginPage';
import DashboardPage from './pages/DashboardPage';
import DevicesPage from './pages/DevicesPage';
import LiveWallPage from './pages/LiveWallPage';
import ScanCenterPage from './pages/ScanCenterPage';
import SettingsPage from './pages/SettingsPage';
import LogsPage from './pages/LogsPage';
import { Monitor, LayoutDashboard, Camera, Wifi, Settings, ScrollText, Shield, LogOut } from 'lucide-react';

function App() {
  return (
    <BrowserRouter>
      <ToastContainer />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/*" element={<ProtectedLayout />} />
      </Routes>
    </BrowserRouter>
  );
}

function ProtectedLayout() {
  const { isAuthenticated, user, setUser, logout } = useAuthStore();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isAuthenticated) {
      navigate('/login');
      return;
    }
    // Fetch user profile
    authAPI.me().then(({ data }) => setUser(data)).catch(() => {
      logout();
      navigate('/login');
    });
  }, [isAuthenticated]);

  if (!isAuthenticated) return null;

  return (
    <div className="app-layout">
      <Sidebar user={user} onLogout={() => { logout(); navigate('/login'); }} />
      <div className="main-content">
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/devices" element={<DevicesPage />} />
          <Route path="/live-wall" element={<LiveWallPage />} />
          <Route path="/scan-center" element={<ScanCenterPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/logs" element={<LogsPage />} />
        </Routes>
      </div>
    </div>
  );
}

function Sidebar({ user, onLogout }) {
  const navItems = [
    { path: '/dashboard', icon: LayoutDashboard, label: 'Dashboard' },
    { path: '/live-wall', icon: Monitor, label: 'Live Wall' },
    { path: '/devices', icon: Camera, label: 'Devices' },
    { path: '/scan-center', icon: Wifi, label: 'Scan Center' },
    { path: '/settings', icon: Settings, label: 'Settings' },
    { path: '/logs', icon: ScrollText, label: 'Audit Logs' },
  ];

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo">C</div>
        <div>
          <div className="sidebar-title">CamGuid</div>
          <div className="sidebar-subtitle">Surveillance</div>
        </div>
      </div>

      <nav className="sidebar-nav">
        <div className="sidebar-section-label">Navigation</div>
        {navItems.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}
          >
            <item.icon />
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <div style={{ padding: 'var(--space-md)', borderTop: '1px solid var(--border-default)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-sm)', marginBottom: 'var(--space-sm)' }}>
          <div style={{
            width: 32, height: 32, borderRadius: 'var(--radius-full)',
            background: 'var(--accent-glow)', display: 'flex', alignItems: 'center',
            justifyContent: 'center', color: 'var(--accent-primary)', fontWeight: 700,
            fontSize: 'var(--font-size-sm)'
          }}>
            {user?.username?.[0]?.toUpperCase() || 'U'}
          </div>
          <div>
            <div style={{ fontSize: 'var(--font-size-sm)', fontWeight: 600, color: 'var(--text-primary)' }}>
              {user?.username || 'User'}
            </div>
            <div style={{ fontSize: 'var(--font-size-xs)', color: 'var(--text-muted)' }}>
              <Shield style={{ width: 10, height: 10, display: 'inline', marginRight: 4 }} />
              {user?.role || 'viewer'}
            </div>
          </div>
        </div>
        <button className="btn btn-ghost btn-sm w-full" onClick={onLogout} style={{ justifyContent: 'flex-start' }}>
          <LogOut style={{ width: 14, height: 14 }} />
          Sign Out
        </button>
      </div>
    </aside>
  );
}

function ToastContainer() {
  const { toasts, removeToast } = useToastStore();

  return (
    <div className="toast-container">
      {toasts.map((toast) => (
        <div key={toast.id} className={`toast toast-${toast.type}`} onClick={() => removeToast(toast.id)}>
          {toast.message}
        </div>
      ))}
    </div>
  );
}

export default App;
