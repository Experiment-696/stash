ALTER TABLE cam_shows ADD COLUMN show_type varchar(48) NOT NULL DEFAULT 'CUSTOM_VIDEO'
  CHECK (show_type IN ('LIVE_PUBLIC','LIVE_GROUP_TICKET_MULTIUSER','LIVE_PRIVATE','LIVE_EXCLUSIVE_PRIVATE','CUSTOM_VIDEO','PRIVATE_CALL'));
ALTER TABLE cam_shows ADD COLUMN captured_timezone varchar(64);
ALTER TABLE cam_shows ADD COLUMN captured_precision varchar(16)
  CHECK (captured_precision IS NULL OR captured_precision IN ('DATE','MINUTE','SECOND'));
ALTER TABLE cam_shows ADD COLUMN duration_override_seconds real
  CHECK (duration_override_seconds IS NULL OR duration_override_seconds >= 0);
ALTER TABLE cam_shows ADD COLUMN duration_override_reason text;

UPDATE cam_shows SET show_type = CASE
  WHEN upper(category) IN ('LIVE','LIVE CAPTURE','LIVE_PUBLIC') THEN 'LIVE_PUBLIC'
  WHEN upper(category) IN ('LIVE GROUP','GROUP','TICKET','MULTIUSER') THEN 'LIVE_GROUP_TICKET_MULTIUSER'
  WHEN upper(category) = 'LIVE_PRIVATE' THEN 'LIVE_PRIVATE'
  WHEN upper(category) = 'LIVE_EXCLUSIVE_PRIVATE' THEN 'LIVE_EXCLUSIVE_PRIVATE'
  WHEN upper(category) = 'PRIVATE_CALL' THEN 'PRIVATE_CALL'
  ELSE 'CUSTOM_VIDEO' END;

CREATE TRIGGER cam_shows_duration_override_insert
BEFORE INSERT ON cam_shows
WHEN NEW.duration_override_seconds IS NOT NULL AND trim(COALESCE(NEW.duration_override_reason,'')) = ''
BEGIN SELECT RAISE(ABORT,'duration override reason is required'); END;
CREATE TRIGGER cam_shows_duration_override_update
BEFORE UPDATE OF duration_override_seconds,duration_override_reason ON cam_shows
WHEN NEW.duration_override_seconds IS NOT NULL AND trim(COALESCE(NEW.duration_override_reason,'')) = ''
BEGIN SELECT RAISE(ABORT,'duration override reason is required'); END;

CREATE TABLE cam_show_sites (
  show_id integer NOT NULL REFERENCES cam_shows(id) ON DELETE CASCADE,
  site_id integer NOT NULL REFERENCES cam_sites(id) ON DELETE CASCADE,
  created_at datetime NOT NULL,
  PRIMARY KEY(show_id,site_id)
);
CREATE INDEX cam_show_sites_site ON cam_show_sites(site_id,show_id);
INSERT INTO cam_show_sites(show_id,site_id,created_at)
  SELECT id,site_id,created_at FROM cam_shows WHERE site_id IS NOT NULL;

CREATE TABLE cam_show_links (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  show_id integer NOT NULL REFERENCES cam_shows(id) ON DELETE CASCADE,
  site_id integer REFERENCES cam_sites(id) ON DELETE SET NULL,
  link_type varchar(16) NOT NULL CHECK (link_type IN ('SOURCE','SHOW')),
  url text NOT NULL,
  label varchar(255),
  source varchar(40) NOT NULL DEFAULT 'MANUAL',
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  UNIQUE(show_id,link_type,url)
);
CREATE INDEX cam_show_links_show ON cam_show_links(show_id,id);
INSERT INTO cam_show_links(show_id,site_id,link_type,url,label,source,created_at,updated_at)
  SELECT id,site_id,'SOURCE',source_url,NULL,'LEGACY',created_at,updated_at
  FROM cam_shows WHERE source_url IS NOT NULL AND trim(source_url) <> '';

CREATE TABLE cam_model_social_profiles (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  platform varchar(40) NOT NULL,
  icon text,
  handle varchar(255) NOT NULL,
  url text NOT NULL,
  status varchar(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE','UNKNOWN')),
  valid_from datetime,
  valid_to datetime,
  source varchar(40) NOT NULL DEFAULT 'MANUAL',
  confidence real CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
  provenance text,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from),
  UNIQUE(model_id,platform,handle,url,valid_from)
);
CREATE INDEX cam_model_social_profiles_model ON cam_model_social_profiles(model_id,status,platform,id);
