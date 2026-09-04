CREATE TABLE IF NOT EXISTS cam_sites (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  name varchar(255) NOT NULL,
  base_url text,
  external_key varchar(255),
  icon text,
  enabled boolean NOT NULL DEFAULT 1,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS cam_sites_name_unique ON cam_sites(name COLLATE NOCASE);
CREATE UNIQUE INDEX IF NOT EXISTS cam_sites_external_key_unique ON cam_sites(external_key) WHERE external_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS cam_models (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  display_name varchar(255) NOT NULL,
  image text,
  notes text,
  status varchar(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','UNKNOWN')),
  performer_id integer REFERENCES performers(id) ON DELETE SET NULL,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL
);
CREATE INDEX IF NOT EXISTS cam_models_performer ON cam_models(performer_id);
CREATE INDEX IF NOT EXISTS cam_models_display_name ON cam_models(display_name COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS cam_shows (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  scene_id integer NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
  category varchar(40) NOT NULL,
  site_id integer REFERENCES cam_sites(id) ON DELETE SET NULL,
  show_date date,
  captured_at datetime,
  source_url text,
  title_override text,
  notes text,
  external_id varchar(255),
  sync_state varchar(20) NOT NULL DEFAULT 'LOCAL' CHECK (sync_state IN ('LOCAL','SYNCED','PENDING','CONFLICT')),
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS cam_shows_scene_unique ON cam_shows(scene_id);
CREATE INDEX IF NOT EXISTS cam_shows_site_date ON cam_shows(site_id, show_date);
CREATE UNIQUE INDEX IF NOT EXISTS cam_shows_site_external_unique ON cam_shows(site_id, external_id) WHERE site_id IS NOT NULL AND external_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS cam_show_models (
  show_id integer NOT NULL REFERENCES cam_shows(id) ON DELETE CASCADE,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  billing_order integer NOT NULL DEFAULT 0 CHECK (billing_order >= 0),
  participation_role varchar(40),
  PRIMARY KEY(show_id, model_id)
);
CREATE INDEX IF NOT EXISTS cam_show_models_model ON cam_show_models(model_id, show_id);

CREATE TABLE IF NOT EXISTS cam_model_accounts (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  site_id integer NOT NULL REFERENCES cam_sites(id) ON DELETE CASCADE,
  handle varchar(255) NOT NULL,
  normalized_handle varchar(255) NOT NULL,
  profile_url text,
  external_account_id varchar(255),
  status varchar(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','UNKNOWN')),
  first_seen_at datetime,
  last_seen_at datetime,
  valid_from datetime,
  valid_to datetime,
  last_synced_at datetime,
  source varchar(40) NOT NULL DEFAULT 'MANUAL',
  confidence real CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from)
);
CREATE INDEX IF NOT EXISTS cam_model_accounts_model ON cam_model_accounts(model_id);
CREATE INDEX IF NOT EXISTS cam_model_accounts_site_handle_history ON cam_model_accounts(site_id, normalized_handle, valid_from, valid_to);
CREATE UNIQUE INDEX IF NOT EXISTS cam_model_accounts_active_site_handle ON cam_model_accounts(site_id, normalized_handle) WHERE valid_to IS NULL AND status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS cam_model_aliases (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  account_id integer REFERENCES cam_model_accounts(id) ON DELETE SET NULL,
  site_id integer REFERENCES cam_sites(id) ON DELETE SET NULL,
  alias varchar(255) NOT NULL,
  normalized_alias varchar(255) NOT NULL,
  valid_from datetime,
  valid_to datetime,
  is_current boolean NOT NULL DEFAULT 1,
  source varchar(40) NOT NULL DEFAULT 'MANUAL',
  confidence real CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
  last_verified_at datetime,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from)
);
CREATE INDEX IF NOT EXISTS cam_model_aliases_model ON cam_model_aliases(model_id);
CREATE INDEX IF NOT EXISTS cam_model_aliases_lookup ON cam_model_aliases(site_id, normalized_alias, is_current);

CREATE TABLE IF NOT EXISTS cam_model_user_state (
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  favorite boolean NOT NULL DEFAULT 0,
  rating integer CHECK (rating IS NULL OR (rating >= 1 AND rating <= 100)),
  updated_at datetime NOT NULL,
  PRIMARY KEY(user_id, model_id)
);
CREATE INDEX IF NOT EXISTS cam_model_user_state_model ON cam_model_user_state(model_id);

CREATE TABLE IF NOT EXISTS cam_sync_changes (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  provider varchar(255) NOT NULL,
  external_event_id varchar(255),
  entity_type varchar(40) NOT NULL,
  entity_id integer,
  proposed_change_json text NOT NULL CHECK (json_valid(proposed_change_json)),
  status varchar(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','APPROVED','REJECTED','APPLIED')),
  reviewed_by integer REFERENCES users(id) ON DELETE SET NULL,
  reviewed_at datetime,
  created_at datetime NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS cam_sync_changes_provider_event_unique ON cam_sync_changes(provider, external_event_id) WHERE external_event_id IS NOT NULL;
