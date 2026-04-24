CREATE TABLE IF NOT EXISTS transport_domains (
	name TEXT PRIMARY KEY,
	description TEXT NOT NULL DEFAULT '',
	netns_path TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	gtpc_listen_host TEXT NOT NULL,
	gtpc_port INTEGER NOT NULL,
	gtpu_listen_host TEXT NOT NULL,
	gtpu_port INTEGER NOT NULL,
	gtpc_advertise_ipv4 TEXT NOT NULL DEFAULT '',
	gtpc_advertise_ipv6 TEXT NOT NULL DEFAULT '',
	gtpu_advertise_ipv4 TEXT NOT NULL DEFAULT '',
	gtpu_advertise_ipv6 TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS dns_resolvers (
	name TEXT PRIMARY KEY,
	transport_domain TEXT NOT NULL,
	server TEXT NOT NULL,
	priority INTEGER NOT NULL DEFAULT 100,
	timeout_ms INTEGER NOT NULL DEFAULT 2000,
	attempts INTEGER NOT NULL DEFAULT 2,
	search_domain TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE peers_v2 (
	name TEXT PRIMARY KEY,
	address TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	description TEXT NOT NULL DEFAULT '',
	transport_domain TEXT NOT NULL DEFAULT ''
);

INSERT INTO peers_v2(name, address, enabled, description, transport_domain)
SELECT name, address, enabled, description, ''
FROM peers;

DROP TABLE peers;

ALTER TABLE peers_v2 RENAME TO peers;
