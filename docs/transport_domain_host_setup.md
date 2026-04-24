# Transport Domain Host Setup

This document defines the initial attach-mode host preparation for transport domains.

## Scope

The proxy does not create Linux namespaces or VLAN plumbing itself in the current implementation.

The operator must prepare:

- the Linux `netns`
- the NIC or VLAN sub-interface inside that namespace
- IP addressing
- routes
- DNS reachability to that side's DNS servers

The proxy then:

- validates the configured `netns_path`
- binds GTPC and GTPU using the configured transport domain listen addresses
- exposes transport-domain readiness in the API and web UI

## Example Topology

Two operator-facing sides on one host:

- `side_a`
  - `eth0.100`
  - GTPC `192.0.2.10:2123`
  - GTPU `192.0.2.20:2152`
  - DNS `192.0.2.53:53`

- `side_b`
  - `eth0.200`
  - GTPC `198.51.100.10:2123`
  - GTPU `198.51.100.20:2152`
  - DNS `198.51.100.53:53`

## Linux Steps

Create the namespaces:

```bash
ip netns add side_a
ip netns add side_b
```

Create VLAN sub-interfaces and move them into the namespaces:

```bash
ip link add link eth0 name eth0.100 type vlan id 100
ip link add link eth0 name eth0.200 type vlan id 200
ip link set eth0.100 netns side_a
ip link set eth0.200 netns side_b
```

Bring up loopback and assign addresses:

```bash
ip -n side_a link set lo up
ip -n side_a addr add 192.0.2.10/24 dev eth0.100
ip -n side_a addr add 192.0.2.20/24 dev eth0.100
ip -n side_a link set eth0.100 up

ip -n side_b link set lo up
ip -n side_b addr add 198.51.100.10/24 dev eth0.200
ip -n side_b addr add 198.51.100.20/24 dev eth0.200
ip -n side_b link set eth0.200 up
```

Install routes:

```bash
ip -n side_a route add default via 192.0.2.1
ip -n side_b route add default via 198.51.100.1
```

Validate DNS reachability from each namespace:

```bash
ip netns exec side_a getent hosts topon.s8.pgw.epc.example.net
ip netns exec side_b getent hosts topon.s8.pgw.epc.example.net
```

## Proxy Config Mapping

Example transport domain objects:

```yaml
transport_domains:
  - name: side_a
    netns_path: /var/run/netns/side_a
    enabled: true
    gtpc_listen_host: 192.0.2.10
    gtpc_port: 2123
    gtpu_listen_host: 192.0.2.20
    gtpu_port: 2152
    gtpc_advertise_ipv4: 192.0.2.10
    gtpu_advertise_ipv4: 192.0.2.20

dns_resolvers:
  - name: side_a_primary
    transport_domain: side_a
    server: 192.0.2.53:53
    priority: 10
    timeout_ms: 1500
    attempts: 2
    search_domain: epc.example.net
    enabled: true
```

## Current Product Boundary

Current implementation:

- live add/update/delete of domains, resolvers, peers, and routes through API/UI
- GTPC and GTPU listeners are created per enabled transport domain
- each listener socket is created inside the configured Linux `netns` on Linux hosts
- request, response, and GTP-U forwarding traffic leaves through the socket owned by the selected transport domain
- DNS discovery dials the configured DNS server from the selected transport domain namespace
- diagnostics for namespace presence and active listener state

Not yet implemented:

- creating namespaces from the proxy
- moving interfaces into namespaces
- deeper host provisioning automation for VLAN, route, and interface setup
- full end-to-end verification in a real multi-namespace lab in this workspace

That means the current runtime is attach-mode, namespace-aware, and domain-owned, while host networking setup remains an external operator responsibility.
