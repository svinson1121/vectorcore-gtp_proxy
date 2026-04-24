import React, { useCallback, useMemo, useState } from 'react'
import { Plus, Trash2, Edit3, RefreshCw, XCircle } from 'lucide-react'
import Badge from '../components/Badge.jsx'
import Spinner from '../components/Spinner.jsx'
import Modal from '../components/Modal.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import {
  getTransportDomains,
  getPeers, getRouting, setDefaultPeer,
  upsertAPNRoute, deleteAPNRoute,
  upsertIMSIRoute, deleteIMSIRoute,
  upsertIMSIPrefixRoute, deleteIMSIPrefixRoute,
  upsertPLMNRoute, deletePLMNRoute,
} from '../api/client.js'

const TABS = [
  { id: 'imsi', label: 'IMSI' },
  { id: 'imsi-prefix', label: 'IMSI Prefix' },
  { id: 'apn', label: 'APN' },
  { id: 'plmn', label: 'PLMN' },
]

export default function Routing() {
  const toast = useToast()
  const routingState = usePoller(getRouting, 5000)
  const peersState = usePoller(getPeers, 5000)
  const domainsState = usePoller(getTransportDomains, 5000)
  const [tab, setTab] = useState('imsi')
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)
  const [settingDefault, setSettingDefault] = useState(false)

  const peers = Array.isArray(peersState.data) ? peersState.data.filter((peer) => peer.enabled) : []
  const domains = Array.isArray(domainsState.data) ? domainsState.data.filter((domain) => domain.enabled) : []
  const routing = routingState.data || { default_peer: '', imsi_routes: [], imsi_prefix_routes: [], apn_routes: [], plmn_routes: [] }

  const refreshAll = useCallback(() => {
    routingState.refresh()
    peersState.refresh()
    domainsState.refresh()
  }, [domainsState, peersState, routingState])

  const routes = useMemo(() => {
    switch (tab) {
      case 'imsi':
        return (routing.imsi_routes || []).map((route) => ({ ...route, key: route.imsi, label: route.imsi, matchType: 'imsi' }))
      case 'imsi-prefix':
        return (routing.imsi_prefix_routes || []).map((route) => ({ ...route, key: route.prefix, label: route.prefix, matchType: 'imsi-prefix' }))
      case 'plmn':
        return (routing.plmn_routes || []).map((route) => ({ ...route, key: route.plmn, label: route.plmn, matchType: 'plmn' }))
      default:
        return (routing.apn_routes || []).map((route) => ({ ...route, key: route.apn, label: route.apn, matchType: 'apn' }))
    }
  }, [routing, tab])

  const handleDefaultPeer = useCallback(async (value) => {
    setSettingDefault(true)
    try {
      await setDefaultPeer(value)
      toast.success('Default route updated', value)
      refreshAll()
    } catch (err) {
      toast.error('Update failed', err.message)
    } finally {
      setSettingDefault(false)
    }
  }, [refreshAll, toast])

  const handleDelete = useCallback(async () => {
    if (!deleting) return
    try {
      if (deleting.matchType === 'imsi') await deleteIMSIRoute(deleting.label)
      if (deleting.matchType === 'imsi-prefix') await deleteIMSIPrefixRoute(deleting.label)
      if (deleting.matchType === 'apn') await deleteAPNRoute(deleting.label)
      if (deleting.matchType === 'plmn') await deletePLMNRoute(deleting.label)
      toast.success('Route deleted', deleting.label)
      setDeleting(null)
      refreshAll()
    } catch (err) {
      toast.error('Delete failed', err.message)
    }
  }, [deleting, refreshAll, toast])

  if (routingState.loading && !routingState.data) return <div className="loading-center"><Spinner size="md" /></div>
  if (routingState.error && !routingState.data) return (
    <div className="error-state">
      <XCircle size={28} className="error-icon" />
      <div>{routingState.error}</div>
      <button className="btn btn-ghost mt-12" onClick={refreshAll}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Routing</div>
          <div className="page-subtitle">IMSI, IMSI prefix, APN, and PLMN routing with deterministic precedence</div>
        </div>
      </div>

      <div className="chart-card mb-16">
        <div className="form-row">
          <div className="form-group" style={{ marginBottom: 0 }}>
            <label className="form-label">Default Peer</label>
            <select className="select" value={routing.default_peer || ''} disabled={settingDefault} onChange={(e) => handleDefaultPeer(e.target.value)}>
              {peers.map((peer) => (
                <option key={peer.name} value={peer.name}>{peer.name}</option>
              ))}
            </select>
          </div>
          <div className="form-group" style={{ marginBottom: 0 }}>
            <label className="form-label">Routing Precedence</label>
            <div className="input" style={{ display: 'flex', alignItems: 'center' }}>
              IMSI, IMSI prefix, APN, PLMN, default
            </div>
          </div>
        </div>
      </div>

      <div className="tabs">
        {TABS.map((item) => (
          <button key={item.id} className={`tab-btn${tab === item.id ? ' active' : ''}`} onClick={() => setTab(item.id)}>
            {item.label}
          </button>
        ))}
      </div>

      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{routes.length} {TABS.find((item) => item.id === tab)?.label} route{routes.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refreshAll}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => setEditing({ label: '', peer: routing.default_peer || peers[0]?.name || '', action_type: 'static_peer', transport_domain: domains[0]?.name || '', service: '', fqdn: '', matchType: tab })}>
            <Plus size={12} /> Add Route
          </button>
        </div>
      </div>

      {routes.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No {TABS.find((item) => item.id === tab)?.label} routes configured</div>
          <div className="text-muted text-sm">Traffic will fall through to lower-priority routing rules.</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Match</th>
                <th>Action</th>
                <th>Target</th>
                <th>Type</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {routes.map((route) => (
                  <tr key={route.key}>
                    <td className="mono" style={{ fontSize: '0.8rem' }}>{route.label}</td>
                    <td><Badge state={route.action_type || 'static_peer'} label={route.action_type || 'static_peer'} /></td>
                    <td style={{ fontWeight: 600 }}>{route.action_type === 'dns_discovery' ? `${route.transport_domain || '—'} / ${route.fqdn || '—'}` : route.peer}</td>
                    <td><Badge state={route.matchType} /></td>
                    <td>
                    <div className="flex gap-6">
                      <button className="btn-icon" onClick={() => setEditing(route)}><Edit3 size={13} /></button>
                      <button className="btn-icon danger" onClick={() => setDeleting(route)}><Trash2 size={13} /></button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editing && <RouteModal initial={editing} peers={peers} domains={domains} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refreshAll() }} />}
      {deleting && <ConfirmDeleteModal label={`${deleting.matchType} route "${deleting.label}"`} onClose={() => setDeleting(null)} onConfirm={handleDelete} />}
    </div>
  )
}

function RouteModal({ initial, peers, domains, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState(initial)
  const [submitting, setSubmitting] = useState(false)
  const set = useCallback((key, value) => setForm((current) => ({ ...current, [key]: value })), [])

  const labelByType = {
    imsi: 'IMSI',
    'imsi-prefix': 'IMSI Prefix',
    apn: 'APN',
    plmn: 'PLMN',
  }

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault()
    if (!form.label.trim()) { toast.error('Validation', `${labelByType[form.matchType]} is required.`); return }
    if ((form.action_type || 'static_peer') === 'static_peer' && !form.peer) { toast.error('Validation', 'Peer is required for static routing.'); return }
    if ((form.action_type || 'static_peer') === 'dns_discovery' && (!form.transport_domain || !form.fqdn.trim())) {
      toast.error('Validation', 'Transport domain and FQDN are required for DNS discovery.'); return
    }
    setSubmitting(true)
    try {
      const payload = {
        peer: form.peer || '',
        action_type: form.action_type || 'static_peer',
        transport_domain: form.transport_domain || '',
        fqdn: form.fqdn.trim(),
        service: form.service.trim(),
      }
      if (form.matchType === 'imsi') await upsertIMSIRoute(form.label.trim(), payload)
      if (form.matchType === 'imsi-prefix') await upsertIMSIPrefixRoute(form.label.trim(), payload)
      if (form.matchType === 'apn') await upsertAPNRoute(form.label.trim(), payload)
      if (form.matchType === 'plmn') await upsertPLMNRoute(form.label.trim(), payload)
      toast.success('Route saved', form.label.trim())
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, onSaved, toast])

  return (
    <Modal title={initial?.label ? 'Edit Route' : 'Add Route'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group">
            <label className="form-label">Route Type</label>
            <select className="select" value={form.matchType} onChange={(e) => set('matchType', e.target.value)}>
              <option value="imsi">IMSI</option>
              <option value="imsi-prefix">IMSI Prefix</option>
              <option value="apn">APN</option>
              <option value="plmn">PLMN</option>
            </select>
          </div>
          <div className="form-group">
            <label className="form-label">{labelByType[form.matchType]}</label>
            <input className="input mono" value={form.label} onChange={(e) => set('label', e.target.value)} placeholder={form.matchType === 'apn' ? 'internet.mnc001.mcc001.gprs' : '001010'} />
          </div>
          <div className="form-group">
            <label className="form-label">Action</label>
            <select
              className="select"
              value={form.action_type || 'static_peer'}
              onChange={(e) => {
                const nextAction = e.target.value
                set('action_type', nextAction)
                if (nextAction === 'static_peer' && !form.peer && peers[0]?.name) {
                  set('peer', peers[0].name)
                }
                if (nextAction === 'dns_discovery' && !form.transport_domain && domains[0]?.name) {
                  set('transport_domain', domains[0].name)
                }
              }}
            >
              <option value="static_peer">Static Peer</option>
              <option value="dns_discovery">DNS Discovery</option>
            </select>
          </div>
          {(form.action_type || 'static_peer') === 'static_peer' ? (
            <div className="form-group">
              <label className="form-label">Peer</label>
              <select className="select" value={form.peer || ''} onChange={(e) => set('peer', e.target.value)}>
                <option value="">Select peer</option>
                {peers.map((peer) => (
                  <option key={peer.name} value={peer.name}>{peer.name}</option>
                ))}
              </select>
            </div>
          ) : (
            <>
              <div className="form-group">
                <label className="form-label">Transport Domain</label>
                <select className="select" value={form.transport_domain || ''} onChange={(e) => set('transport_domain', e.target.value)}>
                  <option value="">Select domain</option>
                  {domains.map((domain) => (
                    <option key={domain.name} value={domain.name}>{domain.name}</option>
                  ))}
                </select>
              </div>
              <div className="form-group">
                <label className="form-label">FQDN</label>
                <input className="input mono" value={form.fqdn || ''} onChange={(e) => set('fqdn', e.target.value)} placeholder="topon.s8.pgw.epc.example.net" />
              </div>
              <div className="form-group">
                <label className="form-label">Service</label>
                <input className="input mono" value={form.service || ''} onChange={(e) => set('service', e.target.value)} placeholder="x-3gpp-pgw" />
              </div>
            </>
          )}
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : null}
            Save Route
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ConfirmDeleteModal({ label, onClose, onConfirm }) {
  return (
    <Modal title="Confirm Delete" onClose={onClose}>
      <div className="modal-body">
        <p>Delete {label}?</p>
        <p className="text-muted text-sm" style={{ marginTop: 6 }}>This action cannot be undone.</p>
      </div>
      <div className="modal-footer">
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-danger" onClick={onConfirm}><Trash2 size={13} /> Delete</button>
      </div>
    </Modal>
  )
}
