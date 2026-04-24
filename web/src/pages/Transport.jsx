import React, { useCallback, useMemo, useState } from 'react'
import { Cable, Plus, RefreshCw, Trash2, Edit3, XCircle, Search } from 'lucide-react'
import Spinner from '../components/Spinner.jsx'
import Modal from '../components/Modal.jsx'
import Badge from '../components/Badge.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import {
  deleteDNSResolver,
  deleteTransportDomain,
  getDNSResolvers,
  getTransportDiagnostics,
  getTransportDomains,
  upsertDNSResolver,
  upsertTransportDomain,
} from '../api/client.js'

const DEFAULT_DOMAIN = {
  name: '',
  description: '',
  netns_path: '',
  enabled: true,
  gtpc_listen_host: '',
  gtpc_port: 2123,
  gtpu_listen_host: '',
  gtpu_port: 2152,
  gtpc_advertise_ipv4: '',
  gtpc_advertise_ipv6: '',
  gtpu_advertise_ipv4: '',
  gtpu_advertise_ipv6: '',
}

const DEFAULT_RESOLVER = {
  name: '',
  transport_domain: '',
  server: '',
  priority: 100,
  timeout_ms: 2000,
  attempts: 2,
  search_domain: '',
  enabled: true,
}

export default function Transport() {
  const toast = useToast()
  const domainsState = usePoller(getTransportDomains, 5000)
  const resolversState = usePoller(getDNSResolvers, 5000)
  const diagnosticsState = usePoller(getTransportDiagnostics, 5000)
  const [editingDomain, setEditingDomain] = useState(null)
  const [editingResolver, setEditingResolver] = useState(null)
  const [deleting, setDeleting] = useState(null)

  const domains = Array.isArray(domainsState.data) ? domainsState.data : []
  const resolvers = Array.isArray(resolversState.data) ? resolversState.data : []
  const diagnostics = diagnosticsState.data || { domains: [] }
  const diagnosticMap = useMemo(() => {
    const map = {}
    ;(diagnostics.domains || []).forEach((item) => { map[item.name] = item })
    return map
  }, [diagnostics.domains])

  const refreshAll = useCallback(() => {
    domainsState.refresh()
    resolversState.refresh()
    diagnosticsState.refresh()
  }, [diagnosticsState, domainsState, resolversState])

  const handleDelete = useCallback(async () => {
    if (!deleting) return
    try {
      if (deleting.kind === 'domain') {
        await deleteTransportDomain(deleting.name)
        toast.success('Transport domain deleted', deleting.name)
      } else {
        await deleteDNSResolver(deleting.name)
        toast.success('DNS resolver deleted', deleting.name)
      }
      setDeleting(null)
      refreshAll()
    } catch (err) {
      toast.error('Delete failed', err.message)
    }
  }, [deleting, refreshAll, toast])

  if (domainsState.loading && !domainsState.data) return <div className="loading-center"><Spinner size="md" /></div>
  if (domainsState.error && !domainsState.data) return (
    <div className="error-state">
      <XCircle size={28} className="error-icon" />
      <div>{domainsState.error}</div>
      <button className="btn btn-ghost mt-12" onClick={refreshAll}><RefreshCw size={13} /> Retry</button>
    </div>
  )

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Transport</div>
          <div className="page-subtitle">Operator-facing transport domains, namespace readiness, and DNS resolver contexts</div>
        </div>
      </div>

      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{domains.length} domain{domains.length !== 1 ? 's' : ''} / {resolvers.length} resolver{resolvers.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refreshAll}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => setEditingDomain(DEFAULT_DOMAIN)}><Plus size={12} /> Add Domain</button>
          <button className="btn btn-ghost btn-sm" onClick={() => setEditingResolver({ ...DEFAULT_RESOLVER, transport_domain: domains[0]?.name || '' })}><Search size={12} /> Add Resolver</button>
        </div>
      </div>

      <div className="section-title">Transport Domains</div>
      {domains.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><Cable size={32} /></div>
          <div>No transport domains configured</div>
          <div className="text-muted text-sm">Attach-mode startup is valid. Add a domain when you are ready to bind GTPC and GTPU on an operator-facing side.</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Domain</th>
                <th>NetNS</th>
                <th>Namespace</th>
                <th>GTPC</th>
                <th>GTPU</th>
                <th>Sessions</th>
                <th>Enabled</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {domains.map((domain) => {
                const diag = diagnosticMap[domain.name] || {}
                return (
                  <tr key={domain.name}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{domain.name}</div>
                      <div className="text-muted" style={{ fontSize: '0.76rem' }}>{diag.effective ? 'effective runtime domain' : domain.description || '—'}</div>
                    </td>
                    <td className="mono" style={{ fontSize: '0.74rem', maxWidth: 220, wordBreak: 'break-all' }}>{domain.netns_path}</td>
                    <td><Badge state={diag.namespace_present ? 'active' : 'disabled'} label={diag.namespace_present ? 'present' : 'missing'} /></td>
                    <td>
                      <div className="mono" style={{ fontSize: '0.78rem' }}>{diag.gtpc_listen || `${domain.gtpc_listen_host}:${domain.gtpc_port}`}</div>
                      <div className="text-muted" style={{ fontSize: '0.74rem' }}>{diag.gtpc_socket_state || 'inactive'}</div>
                    </td>
                    <td>
                      <div className="mono" style={{ fontSize: '0.78rem' }}>{diag.gtpu_listen || `${domain.gtpu_listen_host}:${domain.gtpu_port}`}</div>
                      <div className="text-muted" style={{ fontSize: '0.74rem' }}>{diag.gtpu_socket_state || 'inactive'}</div>
                    </td>
                    <td className="mono">{diag.active_sessions || 0}</td>
                    <td><Badge state={domain.enabled ? 'enabled' : 'disabled'} /></td>
                    <td>
                      <div className="flex gap-6">
                        <button className="btn-icon" onClick={() => setEditingDomain(domain)}><Edit3 size={13} /></button>
                        <button className="btn-icon danger" onClick={() => setDeleting({ kind: 'domain', name: domain.name })}><Trash2 size={13} /></button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <div className="section-title">DNS Resolvers</div>
      {resolvers.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><Search size={32} /></div>
          <div>No DNS resolvers configured</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Domain</th>
                <th>Server</th>
                <th>Search Domain</th>
                <th>Priority</th>
                <th>Enabled</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {resolvers.map((resolver) => (
                <tr key={resolver.name}>
                  <td style={{ fontWeight: 600 }}>{resolver.name}</td>
                  <td>{resolver.transport_domain}</td>
                  <td className="mono">{resolver.server}</td>
                  <td className="mono" style={{ fontSize: '0.76rem' }}>{resolver.search_domain || '—'}</td>
                  <td className="mono">{resolver.priority}</td>
                  <td><Badge state={resolver.enabled ? 'enabled' : 'disabled'} /></td>
                  <td>
                    <div className="flex gap-6">
                      <button className="btn-icon" onClick={() => setEditingResolver(resolver)}><Edit3 size={13} /></button>
                      <button className="btn-icon danger" onClick={() => setDeleting({ kind: 'resolver', name: resolver.name })}><Trash2 size={13} /></button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {editingDomain && <DomainModal initial={editingDomain} onClose={() => setEditingDomain(null)} onSaved={() => { setEditingDomain(null); refreshAll() }} />}
      {editingResolver && <ResolverModal initial={editingResolver} domains={domains} onClose={() => setEditingResolver(null)} onSaved={() => { setEditingResolver(null); refreshAll() }} />}
      {deleting && <ConfirmDeleteModal label={`${deleting.kind} "${deleting.name}"`} onClose={() => setDeleting(null)} onConfirm={handleDelete} />}
    </div>
  )
}

function DomainModal({ initial, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState({ ...DEFAULT_DOMAIN, ...initial })
  const [submitting, setSubmitting] = useState(false)
  const set = useCallback((key, value) => setForm((current) => ({ ...current, [key]: value })), [])

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault()
    if (!form.name.trim()) { toast.error('Validation', 'Domain name is required.'); return }
    if (!form.netns_path.trim()) { toast.error('Validation', 'netns path is required.'); return }
    if (!form.gtpc_listen_host.trim() || !form.gtpu_listen_host.trim()) { toast.error('Validation', 'Listen hosts are required.'); return }
    setSubmitting(true)
    try {
      await upsertTransportDomain(form.name.trim(), {
        ...form,
        name: form.name.trim(),
        description: form.description.trim(),
        netns_path: form.netns_path.trim(),
        gtpc_listen_host: form.gtpc_listen_host.trim(),
        gtpu_listen_host: form.gtpu_listen_host.trim(),
        gtpc_port: Number(form.gtpc_port),
        gtpu_port: Number(form.gtpu_port),
        gtpc_advertise_ipv4: form.gtpc_advertise_ipv4.trim(),
        gtpc_advertise_ipv6: form.gtpc_advertise_ipv6.trim(),
        gtpu_advertise_ipv4: form.gtpu_advertise_ipv4.trim(),
        gtpu_advertise_ipv6: form.gtpu_advertise_ipv6.trim(),
        enabled: !!form.enabled,
      })
      toast.success(initial?.name ? 'Transport domain updated' : 'Transport domain created', form.name.trim())
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial?.name, onSaved, toast])

  return (
    <Modal title={initial?.name ? 'Edit Transport Domain' : 'Add Transport Domain'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group"><label className="form-label">Name</label><input className="input" value={form.name} onChange={(e) => set('name', e.target.value)} /></div>
          <div className="form-group"><label className="form-label">Description</label><input className="input" value={form.description} onChange={(e) => set('description', e.target.value)} /></div>
          <div className="form-group"><label className="form-label">NetNS Path</label><input className="input mono" value={form.netns_path} onChange={(e) => set('netns_path', e.target.value)} placeholder="/var/run/netns/home-a" /></div>
          <div className="form-row">
            <div className="form-group"><label className="form-label">GTPC Listen Host</label><input className="input mono" value={form.gtpc_listen_host} onChange={(e) => set('gtpc_listen_host', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">GTPC Port</label><input className="input mono" value={form.gtpc_port} onChange={(e) => set('gtpc_port', e.target.value)} /></div>
          </div>
          <div className="form-row">
            <div className="form-group"><label className="form-label">GTPU Listen Host</label><input className="input mono" value={form.gtpu_listen_host} onChange={(e) => set('gtpu_listen_host', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">GTPU Port</label><input className="input mono" value={form.gtpu_port} onChange={(e) => set('gtpu_port', e.target.value)} /></div>
          </div>
          <div className="form-row">
            <div className="form-group"><label className="form-label">GTPC Advertise IPv4</label><input className="input mono" value={form.gtpc_advertise_ipv4} onChange={(e) => set('gtpc_advertise_ipv4', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">GTPU Advertise IPv4</label><input className="input mono" value={form.gtpu_advertise_ipv4} onChange={(e) => set('gtpu_advertise_ipv4', e.target.value)} /></div>
          </div>
          <div className="form-row">
            <div className="form-group"><label className="form-label">GTPC Advertise IPv6</label><input className="input mono" value={form.gtpc_advertise_ipv6} onChange={(e) => set('gtpc_advertise_ipv6', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">GTPU Advertise IPv6</label><input className="input mono" value={form.gtpu_advertise_ipv6} onChange={(e) => set('gtpu_advertise_ipv6', e.target.value)} /></div>
          </div>
          <label className="checkbox-wrap"><input type="checkbox" checked={!!form.enabled} onChange={(e) => set('enabled', e.target.checked)} /><span>Enabled</span></label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? <Spinner size="sm" /> : null}Save Domain</button>
        </div>
      </form>
    </Modal>
  )
}

function ResolverModal({ initial, domains, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState({ ...DEFAULT_RESOLVER, ...initial })
  const [submitting, setSubmitting] = useState(false)
  const set = useCallback((key, value) => setForm((current) => ({ ...current, [key]: value })), [])

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault()
    if (!form.name.trim()) { toast.error('Validation', 'Resolver name is required.'); return }
    if (!form.transport_domain) { toast.error('Validation', 'Transport domain is required.'); return }
    if (!form.server.trim()) { toast.error('Validation', 'DNS server is required.'); return }
    setSubmitting(true)
    try {
      await upsertDNSResolver(form.name.trim(), {
        ...form,
        name: form.name.trim(),
        transport_domain: form.transport_domain,
        server: form.server.trim(),
        priority: Number(form.priority),
        timeout_ms: Number(form.timeout_ms),
        attempts: Number(form.attempts),
        search_domain: form.search_domain.trim(),
        enabled: !!form.enabled,
      })
      toast.success(initial?.name ? 'DNS resolver updated' : 'DNS resolver created', form.name.trim())
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial?.name, onSaved, toast])

  return (
    <Modal title={initial?.name ? 'Edit DNS Resolver' : 'Add DNS Resolver'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group"><label className="form-label">Name</label><input className="input" value={form.name} onChange={(e) => set('name', e.target.value)} /></div>
          <div className="form-group"><label className="form-label">Transport Domain</label><select className="select" value={form.transport_domain} onChange={(e) => set('transport_domain', e.target.value)}><option value="">Select domain</option>{domains.map((domain) => <option key={domain.name} value={domain.name}>{domain.name}</option>)}</select></div>
          <div className="form-group"><label className="form-label">Server</label><input className="input mono" value={form.server} onChange={(e) => set('server', e.target.value)} placeholder="192.0.2.53:53" /></div>
          <div className="form-row">
            <div className="form-group"><label className="form-label">Priority</label><input className="input mono" value={form.priority} onChange={(e) => set('priority', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">Timeout (ms)</label><input className="input mono" value={form.timeout_ms} onChange={(e) => set('timeout_ms', e.target.value)} /></div>
            <div className="form-group"><label className="form-label">Attempts</label><input className="input mono" value={form.attempts} onChange={(e) => set('attempts', e.target.value)} /></div>
          </div>
          <div className="form-group"><label className="form-label">Search Domain</label><input className="input mono" value={form.search_domain} onChange={(e) => set('search_domain', e.target.value)} placeholder="epc.mnc001.mcc001.3gppnetwork.org" /></div>
          <label className="checkbox-wrap"><input type="checkbox" checked={!!form.enabled} onChange={(e) => set('enabled', e.target.checked)} /><span>Enabled</span></label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>{submitting ? <Spinner size="sm" /> : null}Save Resolver</button>
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
