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
      msg =
        data.detail ||
        data.message ||
        data.error ||
        data.errors?.[0]?.message ||
        data.errors?.[0]?.detail ||
        msg
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
export const getTransportDomains = () => request('GET', '/transport-domains')
export const upsertTransportDomain = (name, data) => request('PUT', `/transport-domains/${encodeURIComponent(name)}`, data)
export const deleteTransportDomain = (name) => request('DELETE', `/transport-domains/${encodeURIComponent(name)}`)
export const getDNSResolvers = () => request('GET', '/dns-resolvers')
export const upsertDNSResolver = (name, data) => request('PUT', `/dns-resolvers/${encodeURIComponent(name)}`, data)
export const deleteDNSResolver = (name) => request('DELETE', `/dns-resolvers/${encodeURIComponent(name)}`)
export const getPeers = () => request('GET', '/peers')
export const upsertPeer = (name, data) => request('PUT', `/peers/${encodeURIComponent(name)}`, data)
export const deletePeer = (name) => request('DELETE', `/peers/${encodeURIComponent(name)}`)
export const getRouting = () => request('GET', '/routing')
export const setDefaultPeer = (defaultPeer) => request('PUT', '/routing/default-peer', { default_peer: defaultPeer })
export const upsertAPNRoute = (apn, data) => request('PUT', `/routing/apn-routes/${encodeURIComponent(apn)}`, data)
export const deleteAPNRoute = (apn) => request('DELETE', `/routing/apn-routes/${encodeURIComponent(apn)}`)
export const upsertIMSIRoute = (imsi, data) => request('PUT', `/routing/imsi-routes/${encodeURIComponent(imsi)}`, data)
export const deleteIMSIRoute = (imsi) => request('DELETE', `/routing/imsi-routes/${encodeURIComponent(imsi)}`)
export const upsertIMSIPrefixRoute = (prefix, data) => request('PUT', `/routing/imsi-prefix-routes/${encodeURIComponent(prefix)}`, data)
export const deleteIMSIPrefixRoute = (prefix) => request('DELETE', `/routing/imsi-prefix-routes/${encodeURIComponent(prefix)}`)
export const upsertPLMNRoute = (plmn, data) => request('PUT', `/routing/plmn-routes/${encodeURIComponent(plmn)}`, data)
export const deletePLMNRoute = (plmn) => request('DELETE', `/routing/plmn-routes/${encodeURIComponent(plmn)}`)
export const getSessions = () => request('GET', '/sessions')
export const getPeerDiagnostics = () => request('GET', '/diagnostics/peers')
export const getRouteDiagnostics = () => request('GET', '/diagnostics/routes')
export const getTransportDiagnostics = () => request('GET', '/diagnostics/transport')
export const getAuditHistory = (limit = 50) => request('GET', `/diagnostics/audit?limit=${encodeURIComponent(limit)}`)
export const getMetricDetails = () => request('GET', '/diagnostics/metrics')
