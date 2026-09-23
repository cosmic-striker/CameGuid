import React, { useState, useEffect } from 'react';
import { useAuthStore, useToastStore } from '../store/store';
import { authAPI } from '../api/client';
import { Shield, Users, Lock, Server } from 'lucide-react';

export default function SettingsPage() {
  const { user } = useAuthStore();
  const isAdmin = user?.role === 'admin';
  const [tab, setTab] = useState('password');

  return (
    <>
      <div className="main-header">
        <h1 className="page-title">Settings</h1>
      </div>
      <div className="main-body">
        <div className="flex gap-md mb-lg">
          <button className={`btn ${tab === 'password' ? 'btn-primary' : 'btn-secondary'} btn-sm`} onClick={() => setTab('password')}>
            <Lock size={14} /> Change Password
          </button>
          {isAdmin && (
            <button className={`btn ${tab === 'users' ? 'btn-primary' : 'btn-secondary'} btn-sm`} onClick={() => setTab('users')}>
              <Users size={14} /> User Management
            </button>
          )}
          <button className={`btn ${tab === 'system' ? 'btn-primary' : 'btn-secondary'} btn-sm`} onClick={() => setTab('system')}>
            <Server size={14} /> System Info
          </button>
        </div>

        {tab === 'password' && <PasswordSection />}
        {tab === 'users' && isAdmin && <UserManagementSection />}
        {tab === 'system' && <SystemInfoSection />}
      </div>
    </>
  );
}

function PasswordSection() {
  const [form, setForm] = useState({ old_password: '', new_password: '', confirm: '' });
  const [saving, setSaving] = useState(false);
  const { addToast } = useToastStore();

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (form.new_password !== form.confirm) {
      addToast('Passwords do not match', 'error');
      return;
    }
    setSaving(true);
    try {
      await authAPI.changePassword({ old_password: form.old_password, new_password: form.new_password });
      addToast('Password changed successfully', 'success');
      setForm({ old_password: '', new_password: '', confirm: '' });
    } catch (err) {
      addToast(err.response?.data?.error?.message || 'Failed to change password', 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="card" style={{ maxWidth: 480 }}>
      <div className="card-header">
        <span className="card-title">Change Password</span>
      </div>
      <form onSubmit={handleSubmit}>
        <div className="input-group">
          <label className="input-label">Current Password</label>
          <input className="input" type="password" value={form.old_password} onChange={(e) => setForm({...form, old_password: e.target.value})} required />
        </div>
        <div className="input-group">
          <label className="input-label">New Password</label>
          <input className="input" type="password" value={form.new_password} onChange={(e) => setForm({...form, new_password: e.target.value})} required minLength={8} />
        </div>
        <div className="input-group">
          <label className="input-label">Confirm New Password</label>
          <input className="input" type="password" value={form.confirm} onChange={(e) => setForm({...form, confirm: e.target.value})} required />
        </div>
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Changing...' : 'Update Password'}
        </button>
      </form>
    </div>
  );
}

function UserManagementSection() {
  const [form, setForm] = useState({ username: '', password: '', role: 'viewer' });
  const [saving, setSaving] = useState(false);
  const { addToast } = useToastStore();

  const handleCreate = async (e) => {
    e.preventDefault();
    setSaving(true);
    try {
      await authAPI.register(form);
      addToast(`User "${form.username}" created`, 'success');
      setForm({ username: '', password: '', role: 'viewer' });
    } catch (err) {
      addToast(err.response?.data?.error?.message || 'Failed to create user', 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="card" style={{ maxWidth: 480 }}>
      <div className="card-header">
        <span className="card-title">Create User</span>
        <Shield size={18} color="var(--accent-purple)" />
      </div>
      <form onSubmit={handleCreate}>
        <div className="input-group">
          <label className="input-label">Username</label>
          <input className="input" value={form.username} onChange={(e) => setForm({...form, username: e.target.value})} required minLength={3} />
        </div>
        <div className="input-group">
          <label className="input-label">Password</label>
          <input className="input" type="password" value={form.password} onChange={(e) => setForm({...form, password: e.target.value})} required minLength={8} />
        </div>
        <div className="input-group">
          <label className="input-label">Role</label>
          <select className="input" value={form.role} onChange={(e) => setForm({...form, role: e.target.value})}>
            <option value="viewer">Viewer</option>
            <option value="operator">Operator</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Creating...' : 'Create User'}
        </button>
      </form>
    </div>
  );
}

function SystemInfoSection() {
  return (
    <div className="card" style={{ maxWidth: 480 }}>
      <div className="card-header">
        <span className="card-title">System Information</span>
      </div>
      <table style={{ width: '100%' }}>
        <tbody>
          <tr><td className="text-muted">Application</td><td>CamGuid v1.0.0</td></tr>
          <tr><td className="text-muted">API Server</td><td>Go + Fiber</td></tr>
          <tr><td className="text-muted">Database</td><td>PostgreSQL 16</td></tr>
          <tr><td className="text-muted">Cache</td><td>Redis 7</td></tr>
          <tr><td className="text-muted">Stream Engine</td><td>FFmpeg 6 (HLS)</td></tr>
          <tr><td className="text-muted">Network Mode</td><td>Local LAN Only</td></tr>
          <tr><td className="text-muted">Port</td><td>2005</td></tr>
        </tbody>
      </table>
    </div>
  );
}
