import React from 'react'
import { Activity, Route, ArrowLeftRight, Clock3, RefreshCw, XCircle } from 'lucide-react'
import StatCard from '../components/StatCard.jsx'
import Spinner from '../components/Spinner.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getStatus, getStatusPeers, getSessions } from '../api/client.js'

function fmtTimestamp(value) {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export default function Dashboard() {
  const statusState = usePoller(getStatus, 5000)
  const peersState = usePoller(getStatusPeers, 5000)
  const sessionsState = usePoller(getSessions, 5000)

  const loading = statusState.loading && !statusState.data
  const error = statusState.error && !statusState.data ? statusState.error : null
  const peers = Array.isArray(peersState.data) ? peersState.data : []
  const sessions = Array.isArray(sessionsState.data) ? sessionsState.data : []
  const status = statusState.data || {}

  if (loading) return <div className="loading-center"><Spinner size="lg" /><span>Loading dashboard...</span></div>
  if (error) return (
    <div className="error-state">
      <XCircle size={32} className="error-icon" />
      <div>{error}</div>
      <button className="btn btn-ghost mt-12" onClick={statusState.refresh}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Dashboard</div>
          <div className="page-subtitle">Phase 2 control-plane and user-plane overview</div>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => {
          statusState.refresh()
          peersState.refresh()
          sessionsState.refresh()
        }}><RefreshCw size={13} /></button>
      </div>

      <div className="stats-grid">
        <StatCard
          title="Active Sessions"
          value={(status.active_sessions || 0).toLocaleString()}
          icon={<Activity size={18} />}
          color="var(--accent)"
          subtitle={`${status.pending_transactions || 0} pending transactions`}
        />
        <StatCard
          title="GTP-U Forwarded"
          value={(status.gtpu_packets_forwarded || 0).toLocaleString()}
          icon={<ArrowLeftRight size={18} />}
          color="var(--success)"
          subtitle={`${status.gtpu_forward_hits || 0} hits / ${status.gtpu_forward_misses || 0} misses`}
        />
        <StatCard
          title="APN Routes"
          value={(status.apn_route_count || 0).toLocaleString()}
          icon={<Route size={18} />}
          color="var(--warning)"
          subtitle={`default ${status.default_peer || '—'}`}
        />
        <StatCard
          title="Uptime"
          value={status.uptime || '—'}
          icon={<Clock3 size={18} />}
          color="var(--text-muted)"
          subtitle={`v${status.version || '—'}`}
        />
      </div>

      <div className="section-title">Recent Sessions</div>
      {sessions.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><Activity size={32} /></div>
          <div>No active Create Session state yet</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>IMSI</th>
                <th>APN</th>
                <th>Visited Peer</th>
                <th>Home Peer</th>
                <th>Proxy TEIDs</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {sessions.slice(0, 12).map((session) => (
                <tr key={session.id}>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{session.imsi || '—'}</td>
                  <td>{session.apn || '—'}</td>
                  <td className="mono" style={{ fontSize: '0.78rem' }}>{session.visited_control_endpoint || '—'}</td>
                  <td className="mono" style={{ fontSize: '0.78rem' }}>{session.home_control_endpoint || '—'}</td>
                  <td className="mono" style={{ fontSize: '0.76rem' }}>
                    {session.proxy_visited_control_teid || 0} / {session.proxy_home_control_teid || 0}
                    <div style={{ color: 'var(--text-muted)', marginTop: 4 }}>
                      U {session.proxy_visited_user_teid || 0} / {session.proxy_home_user_teid || 0}
                    </div>
                  </td>
                  <td className="text-muted" style={{ fontSize: '0.78rem', whiteSpace: 'nowrap' }}>
                    {fmtTimestamp(session.updated_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
