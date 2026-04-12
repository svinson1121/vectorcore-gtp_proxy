import React, { useCallback, useMemo } from 'react'
import { RefreshCw, XCircle, History, Route, ServerCog, AlertTriangle } from 'lucide-react'
import Spinner from '../components/Spinner.jsx'
import StatCard from '../components/StatCard.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getAuditHistory, getMetricDetails, getPeerDiagnostics, getRouteDiagnostics, getStatus } from '../api/client.js'

function fmtTimestamp(value) {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString()
}

export default function OAM() {
  const statusState = usePoller(getStatus, 5000)
  const peersState = usePoller(getPeerDiagnostics, 5000)
  const routesState = usePoller(getRouteDiagnostics, 5000)
  const auditState = usePoller(() => getAuditHistory(25), 5000)
  const metricsState = usePoller(getMetricDetails, 5000)

  const refreshAll = useCallback(() => {
    statusState.refresh()
    peersState.refresh()
    routesState.refresh()
    auditState.refresh()
    metricsState.refresh()
  }, [auditState, metricsState, peersState, routesState, statusState])

  const loading = statusState.loading && !statusState.data
  const error = statusState.error && !statusState.data ? statusState.error : null
  const peers = Array.isArray(peersState.data) ? peersState.data : []
  const routes = Array.isArray(routesState.data) ? routesState.data : []
  const auditEntries = Array.isArray(auditState.data) ? auditState.data : []
  const metricDetails = metricsState.data || { peer_counters: {}, message_errors: {} }
  const status = statusState.data || {}

  const errorRows = useMemo(() => {
    return Object.entries(metricDetails.message_errors || {})
      .sort((a, b) => b[1] - a[1])
  }, [metricDetails.message_errors])

  if (loading) return <div className="loading-center"><Spinner size="lg" /><span>Loading diagnostics...</span></div>
  if (error) return (
    <div className="error-state">
      <XCircle size={32} className="error-icon" />
      <div>{error}</div>
      <button className="btn btn-ghost mt-12" onClick={refreshAll}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">OAM</div>
          <div className="page-subtitle">Operational diagnostics for peers, routing, audit history, and error visibility</div>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={refreshAll}><RefreshCw size={13} /></button>
      </div>

      <div className="stats-grid">
        <StatCard
          title="Peers"
          value={(peers.length || 0).toLocaleString()}
          icon={<ServerCog size={18} />}
          color="var(--accent)"
          subtitle={`${peers.filter((peer) => peer.status === 'active').length} active`}
        />
        <StatCard
          title="Route Decisions"
          value={(routes.length || 0).toLocaleString()}
          icon={<Route size={18} />}
          color="var(--success)"
          subtitle={`${status.active_sessions || 0} active sessions`}
        />
        <StatCard
          title="Audit Entries"
          value={(auditEntries.length || 0).toLocaleString()}
          icon={<History size={18} />}
          color="var(--warning)"
          subtitle="latest mutable config changes"
        />
        <StatCard
          title="Message Errors"
          value={errorRows.reduce((sum, [, count]) => sum + count, 0).toLocaleString()}
          icon={<AlertTriangle size={18} />}
          color="var(--danger)"
          subtitle={`${Object.keys(metricDetails.message_errors || {}).length} distinct counters`}
        />
      </div>

      <div className="section-title">Peer Status</div>
      {peers.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><ServerCog size={32} /></div>
          <div>No peers configured yet</div>
          <div className="text-muted text-sm">First-run state is valid. Add peers and routes when you are ready to steer traffic.</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Peer</th>
                <th>Status</th>
                <th>Address</th>
                <th>Routes</th>
                <th>Sessions</th>
                <th>Control Plane</th>
                <th>User Plane</th>
                <th>Last Activity</th>
              </tr>
            </thead>
            <tbody>
              {peers.map((peer) => (
                <tr key={peer.name}>
                  <td style={{ fontWeight: 600 }}>{peer.name}</td>
                  <td>{peer.status}</td>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{peer.address}</td>
                  <td className="mono">{peer.route_count}</td>
                  <td className="mono">{peer.active_sessions}</td>
                  <td className="mono">{peer.control_plane_packets || 0}</td>
                  <td className="mono">{peer.user_plane_packets || 0}</td>
                  <td className="text-muted" style={{ fontSize: '0.78rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(peer.last_session_update)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="section-title">Active Route Decisions</div>
      {routes.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><Route size={32} /></div>
          <div>No active route decisions to display</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Session</th>
                <th>IMSI</th>
                <th>APN</th>
                <th>Match Type</th>
                <th>Match Value</th>
                <th>Selected Peer</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {routes.map((route) => (
                <tr key={route.session_id}>
                  <td className="mono" style={{ fontSize: '0.76rem' }}>{route.session_id}</td>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{route.imsi || '—'}</td>
                  <td>{route.apn || '—'}</td>
                  <td>{route.route_match_type || '—'}</td>
                  <td className="mono" style={{ fontSize: '0.78rem' }}>{route.route_match_value || '—'}</td>
                  <td>{route.route_peer || '—'}</td>
                  <td className="text-muted" style={{ fontSize: '0.78rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(route.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="section-title">Audit History</div>
      {auditEntries.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><History size={32} /></div>
          <div>No mutable config changes recorded yet</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Action</th>
                <th>Object</th>
                <th>Key</th>
                <th>Before</th>
                <th>After</th>
              </tr>
            </thead>
            <tbody>
              {auditEntries.map((entry) => (
                <tr key={entry.id}>
                  <td className="text-muted" style={{ fontSize: '0.78rem', whiteSpace: 'nowrap' }}>{fmtTimestamp(entry.created_at)}</td>
                  <td>{entry.action}</td>
                  <td>{entry.object_type}</td>
                  <td className="mono" style={{ fontSize: '0.78rem' }}>{entry.object_key}</td>
                  <td className="mono" style={{ fontSize: '0.72rem', maxWidth: 240, wordBreak: 'break-all' }}>{entry.before_json || '—'}</td>
                  <td className="mono" style={{ fontSize: '0.72rem', maxWidth: 240, wordBreak: 'break-all' }}>{entry.after_json || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="section-title">Message Error Counters</div>
      {errorRows.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><AlertTriangle size={32} /></div>
          <div>No protocol/message errors recorded</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Protocol / Message</th>
                <th>Count</th>
              </tr>
            </thead>
            <tbody>
              {errorRows.map(([key, count]) => (
                <tr key={key}>
                  <td className="mono">{key}</td>
                  <td className="mono">{count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
