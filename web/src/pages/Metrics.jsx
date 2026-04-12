import React from 'react'
import { RefreshCw, XCircle } from 'lucide-react'
import Spinner from '../components/Spinner.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getSessions } from '../api/client.js'

function fmtTimestamp(value) {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export default function Sessions() {
  const { data, error, loading, refresh } = usePoller(getSessions, 5000)
  const sessions = Array.isArray(data) ? data : []

  if (loading && !data) return <div className="loading-center"><Spinner size="md" /></div>
  if (error && !data) return (
    <div className="error-state">
      <XCircle size={28} className="error-icon" />
      <div>{error}</div>
      <button className="btn btn-ghost mt-12" onClick={refresh}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Sessions</div>
          <div className="page-subtitle">Control-plane and user-plane mappings derived from GTP-C state</div>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={13} /></button>
      </div>

      {sessions.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No active sessions</div>
          <div className="text-muted text-sm">Once Create Session traffic passes through the proxy, state will appear here.</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Session ID</th>
                <th>IMSI</th>
                <th>APN</th>
                <th>Visited TEID</th>
                <th>Home TEID</th>
                <th>User Plane</th>
                <th>Proxy TEIDs</th>
                <th>Created</th>
                <th>Updated</th>
                <th>Expires</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((session) => (
                <tr key={session.id}>
                  <td className="mono" style={{ fontSize: '0.75rem' }}>{session.id}</td>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{session.imsi || '—'}</td>
                  <td>{session.apn || '—'}</td>
                  <td className="mono">{session.visited_control_teid || 0}</td>
                  <td className="mono">{session.home_control_teid || 0}</td>
                  <td className="mono" style={{ fontSize: '0.74rem' }}>
                    {session.visited_user_teid || 0} / {session.home_user_teid || 0}
                    <div style={{ color: 'var(--text-muted)', marginTop: 4 }}>
                      {session.visited_user_endpoint || '—'}
                    </div>
                    <div style={{ color: 'var(--text-muted)' }}>
                      {session.home_user_endpoint || '—'}
                    </div>
                  </td>
                  <td className="mono" style={{ fontSize: '0.76rem' }}>
                    C {session.proxy_visited_control_teid || 0} / {session.proxy_home_control_teid || 0}
                    <div style={{ marginTop: 4 }}>U {session.proxy_visited_user_teid || 0} / {session.proxy_home_user_teid || 0}</div>
                  </td>
                  <td className="text-muted" style={{ fontSize: '0.76rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(session.created_at)}</td>
                  <td className="text-muted" style={{ fontSize: '0.76rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(session.updated_at)}</td>
                  <td className="text-muted" style={{ fontSize: '0.76rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(session.expires_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
