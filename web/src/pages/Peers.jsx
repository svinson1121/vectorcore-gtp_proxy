import React, { useCallback, useMemo, useState } from 'react'
import { Plus, Trash2, Edit3, RefreshCw, XCircle } from 'lucide-react'
import Badge from '../components/Badge.jsx'
import Spinner from '../components/Spinner.jsx'
import Modal from '../components/Modal.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { deletePeer, getPeers, getStatusPeers, getTransportDomains, upsertPeer } from '../api/client.js'

const DEFAULT_PEER = {
  name: '',
  address: '',
  enabled: true,
  description: '',
  transport_domain: '',
}

export default function Peers() {
  const toast = useToast()
  const peersState = usePoller(getPeers, 5000)
  const statusState = usePoller(getStatusPeers, 5000)
  const domainsState = usePoller(getTransportDomains, 5000)
  const [editing, setEditing] = useState(null)
  const [deleting, setDeleting] = useState(null)
  const [busy, setBusy] = useState(false)

  const peers = Array.isArray(peersState.data) ? peersState.data : []
  const domains = Array.isArray(domainsState.data) ? domainsState.data.filter((domain) => domain.enabled) : []
  const statusMap = useMemo(() => {
    const map = {}
    const items = Array.isArray(statusState.data) ? statusState.data : []
    items.forEach((item) => { map[item.name] = item })
    return map
  }, [statusState.data])

  const refreshAll = useCallback(() => {
    peersState.refresh()
    statusState.refresh()
    domainsState.refresh()
  }, [domainsState, peersState, statusState])

  const handleDelete = useCallback(async () => {
    if (!deleting) return
    setBusy(true)
    try {
      await deletePeer(deleting.name)
      toast.success('Peer deleted', deleting.name)
      setDeleting(null)
      refreshAll()
    } catch (err) {
      toast.error('Delete failed', err.message)
    } finally {
      setBusy(false)
    }
  }, [deleting, refreshAll, toast])

  if (peersState.loading && !peersState.data) return <div className="loading-center"><Spinner size="md" /></div>
  if (peersState.error && !peersState.data) return (
    <div className="error-state">
      <XCircle size={28} className="error-icon" />
      <div>{peersState.error}</div>
      <button className="btn btn-ghost mt-12" onClick={refreshAll}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Peers</div>
          <div className="page-subtitle">Home-side GTP-C peers used for APN routing</div>
        </div>
      </div>

      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{peers.length} peer{peers.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refreshAll}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => setEditing(DEFAULT_PEER)}>
            <Plus size={12} /> Add Peer
          </button>
        </div>
      </div>

      {peers.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No peers configured</div>
          <div className="text-muted text-sm">Add a home-side GTP-C peer before routing traffic.</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Address</th>
                <th>Description</th>
                <th>Transport Domain</th>
                <th>Routes</th>
                <th>Enabled</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {peers.map((peer) => {
                const status = statusMap[peer.name]
                return (
                  <tr key={peer.name}>
                    <td style={{ fontWeight: 600 }}>{peer.name}</td>
                    <td className="mono" style={{ fontSize: '0.8rem' }}>{peer.address}</td>
                    <td className="text-muted" style={{ fontSize: '0.82rem' }}>{peer.description || '—'}</td>
                    <td>{peer.transport_domain || '—'}</td>
                    <td className="mono">{status?.route_count ?? 0}</td>
                    <td><Badge state={peer.enabled ? 'enabled' : 'disabled'} /></td>
                    <td>
                      <div className="flex gap-6">
                        <button className="btn-icon" onClick={() => setEditing(peer)}><Edit3 size={13} /></button>
                        <button className="btn-icon danger" onClick={() => setDeleting(peer)}><Trash2 size={13} /></button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {editing && (
        <PeerModal
          initial={editing}
          domains={domains}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            refreshAll()
          }}
        />
      )}
      {deleting && (
        <ConfirmDeleteModal
          label={`peer "${deleting.name}"`}
          loading={busy}
          onClose={() => setDeleting(null)}
          onConfirm={handleDelete}
        />
      )}
    </div>
  )
}

function PeerModal({ initial, domains, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState({ ...DEFAULT_PEER, ...initial })
  const [submitting, setSubmitting] = useState(false)
  const set = useCallback((key, value) => setForm((current) => ({ ...current, [key]: value })), [])

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault()
    if (!form.name.trim()) { toast.error('Validation', 'Peer name is required.'); return }
    if (!form.address.trim()) { toast.error('Validation', 'Peer address is required.'); return }

    setSubmitting(true)
    try {
      await upsertPeer(form.name.trim(), {
        name: form.name.trim(),
        address: form.address.trim(),
        enabled: !!form.enabled,
        description: form.description.trim(),
        transport_domain: form.transport_domain || '',
      })
      toast.success(initial?.name ? 'Peer updated' : 'Peer created', form.name.trim())
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial?.name, onSaved, toast])

  return (
    <Modal title={initial?.name ? 'Edit Peer' : 'Add Peer'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group">
            <label className="form-label">Peer Name</label>
            <input className="input" value={form.name} onChange={(e) => set('name', e.target.value)} />
          </div>
          <div className="form-group">
            <label className="form-label">GTP-C Address</label>
            <input className="input mono" value={form.address} onChange={(e) => set('address', e.target.value)} placeholder="192.0.2.10:2123" />
          </div>
          <div className="form-group">
            <label className="form-label">Description</label>
            <input className="input" value={form.description} onChange={(e) => set('description', e.target.value)} />
          </div>
          <div className="form-group">
            <label className="form-label">Transport Domain</label>
            <select className="select" value={form.transport_domain || ''} onChange={(e) => set('transport_domain', e.target.value)}>
              <option value="">Select domain</option>
              {domains.map((domain) => (
                <option key={domain.name} value={domain.name}>{domain.name}</option>
              ))}
            </select>
          </div>
          <label className="checkbox-wrap">
            <input type="checkbox" checked={!!form.enabled} onChange={(e) => set('enabled', e.target.checked)} />
            <span>Enabled</span>
          </label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : null}
            {initial?.name ? 'Save Changes' : 'Add Peer'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ConfirmDeleteModal({ label, onClose, onConfirm, loading }) {
  return (
    <Modal title="Confirm Delete" onClose={onClose}>
      <div className="modal-body">
        <p>Delete {label}?</p>
        <p className="text-muted text-sm" style={{ marginTop: 6 }}>This action cannot be undone.</p>
      </div>
      <div className="modal-footer">
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-danger" onClick={onConfirm} disabled={loading}>
          {loading ? <Spinner size="sm" /> : <Trash2 size={13} />} Delete
        </button>
      </div>
    </Modal>
  )
}
