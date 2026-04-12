const BASE = '/api/v1'

async function request(method, path, body) {
  const options = { method, headers: {} }
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json'
    options.body = JSON.stringify(body)
  }

  const res = await fetch(`${BASE}${path}`, options)
  if (res.status === 204) return null
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = await res.json()
      msg = data.detail || data.message || data.error || msg
    } catch {
      // ignore parse errors
    }
    throw new Error(msg)
  }

  const text = await res.text()
  if (!text) return null
  return JSON.parse(text)
}

export const getStatus = () => request('GET', '/status')
export const getStatusPeers = () => request('GET', '/status/peers')
export const getConfig = () => request('GET', '/config')
export const getPeers = () => request('GET', '/peers')
export const upsertPeer = (name, data) => request('PUT', `/peers/${encodeURIComponent(name)}`, data)
export const deletePeer = (name) => request('DELETE', `/peers/${encodeURIComponent(name)}`)
export const getRouting = () => request('GET', '/routing')
export const setDefaultPeer = (defaultPeer) => request('PUT', '/routing/default-peer', { default_peer: defaultPeer })
export const upsertAPNRoute = (apn, peer) => request('PUT', `/routing/apn-routes/${encodeURIComponent(apn)}`, { peer })
export const deleteAPNRoute = (apn) => request('DELETE', `/routing/apn-routes/${encodeURIComponent(apn)}`)
export const upsertIMSIRoute = (imsi, peer) => request('PUT', `/routing/imsi-routes/${encodeURIComponent(imsi)}`, { peer })
export const deleteIMSIRoute = (imsi) => request('DELETE', `/routing/imsi-routes/${encodeURIComponent(imsi)}`)
export const upsertIMSIPrefixRoute = (prefix, peer) => request('PUT', `/routing/imsi-prefix-routes/${encodeURIComponent(prefix)}`, { peer })
export const deleteIMSIPrefixRoute = (prefix) => request('DELETE', `/routing/imsi-prefix-routes/${encodeURIComponent(prefix)}`)
export const upsertPLMNRoute = (plmn, peer) => request('PUT', `/routing/plmn-routes/${encodeURIComponent(plmn)}`, { peer })
export const deletePLMNRoute = (plmn) => request('DELETE', `/routing/plmn-routes/${encodeURIComponent(plmn)}`)
export const getSessions = () => request('GET', '/sessions')
export const getPeerDiagnostics = () => request('GET', '/diagnostics/peers')
export const getRouteDiagnostics = () => request('GET', '/diagnostics/routes')
export const getAuditHistory = (limit = 50) => request('GET', `/diagnostics/audit?limit=${encodeURIComponent(limit)}`)
export const getMetricDetails = () => request('GET', '/diagnostics/metrics')
