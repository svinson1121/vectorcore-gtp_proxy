ALTER TABLE apn_routes ADD COLUMN action_type TEXT NOT NULL DEFAULT 'static_peer';
ALTER TABLE apn_routes ADD COLUMN transport_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE apn_routes ADD COLUMN fqdn TEXT NOT NULL DEFAULT '';
ALTER TABLE apn_routes ADD COLUMN service TEXT NOT NULL DEFAULT '';

ALTER TABLE imsi_routes ADD COLUMN action_type TEXT NOT NULL DEFAULT 'static_peer';
ALTER TABLE imsi_routes ADD COLUMN transport_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE imsi_routes ADD COLUMN fqdn TEXT NOT NULL DEFAULT '';
ALTER TABLE imsi_routes ADD COLUMN service TEXT NOT NULL DEFAULT '';

ALTER TABLE imsi_prefix_routes ADD COLUMN action_type TEXT NOT NULL DEFAULT 'static_peer';
ALTER TABLE imsi_prefix_routes ADD COLUMN transport_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE imsi_prefix_routes ADD COLUMN fqdn TEXT NOT NULL DEFAULT '';
ALTER TABLE imsi_prefix_routes ADD COLUMN service TEXT NOT NULL DEFAULT '';

ALTER TABLE plmn_routes ADD COLUMN action_type TEXT NOT NULL DEFAULT 'static_peer';
ALTER TABLE plmn_routes ADD COLUMN transport_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE plmn_routes ADD COLUMN fqdn TEXT NOT NULL DEFAULT '';
ALTER TABLE plmn_routes ADD COLUMN service TEXT NOT NULL DEFAULT '';
