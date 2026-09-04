CREATE TABLE cam_completed_recording_imports (
  id varchar(64) NOT NULL PRIMARY KEY,
  scene_id integer NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
  show_id integer NOT NULL REFERENCES cam_shows(id) ON DELETE CASCADE,
  site_id integer NOT NULL REFERENCES cam_sites(id) ON DELETE RESTRICT,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE RESTRICT,
  configured_root_id varchar(64) NOT NULL,
  relative_path_hash varchar(64) NOT NULL,
  fingerprint_size integer NOT NULL,
  fingerprint_mtime_ns integer NOT NULL,
  fingerprint_mode integer NOT NULL,
  fingerprint_device integer NOT NULL,
  fingerprint_inode integer NOT NULL,
  parser_version varchar(80) NOT NULL,
  captured_at datetime NOT NULL,
  captured_timezone varchar(64) NOT NULL,
  captured_precision varchar(16) NOT NULL
    CHECK (captured_precision IN ('DATE','MINUTE','SECOND')),
  match_state varchar(24) NOT NULL CHECK (match_state = 'EXACT_CURRENT'),
  outcome varchar(24) NOT NULL CHECK (outcome = 'APPLIED'),
  created_at datetime NOT NULL,
  UNIQUE(configured_root_id,relative_path_hash,fingerprint_size,fingerprint_mtime_ns,
    fingerprint_mode,fingerprint_device,fingerprint_inode,parser_version,scene_id)
);
CREATE INDEX cam_completed_recording_imports_show
  ON cam_completed_recording_imports(show_id,id);

CREATE TABLE cam_completed_recording_audits (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  actor_user_id varchar(64) NOT NULL,
  preview_id varchar(64) NOT NULL,
  candidate_id varchar(64) NOT NULL,
  relative_path_hash varchar(64) NOT NULL,
  outcome varchar(32) NOT NULL CHECK (outcome IN
    ('APPLIED','ALREADY_APPLIED','SKIPPED','CHANGED_SINCE_PREVIEW','REVIEW_REQUIRED')),
  review_reason_code varchar(40) CHECK (review_reason_code IS NULL OR review_reason_code IN
    ('MULTIPLE_SCENES','MULTIPLE_SITES','MULTIPLE_ALIASES','HISTORICAL_ALIAS_REUSED')),
  redacted_reason varchar(512),
  scene_id integer,
  site_id integer,
  model_id integer,
  created_at datetime NOT NULL
);
CREATE INDEX cam_completed_recording_audits_candidate
  ON cam_completed_recording_audits(candidate_id,id);
CREATE INDEX cam_completed_recording_audits_created
  ON cam_completed_recording_audits(created_at,id);
