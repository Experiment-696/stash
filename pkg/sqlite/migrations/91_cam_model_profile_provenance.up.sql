CREATE TABLE IF NOT EXISTS cam_model_profile_provenance (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  model_id integer NOT NULL REFERENCES cam_models(id) ON DELETE CASCADE,
  account_id integer REFERENCES cam_model_accounts(id) ON DELETE SET NULL,
  provider varchar(255) NOT NULL,
  evidence_key varchar(255) NOT NULL,
  provider_record_id varchar(255),
  source_url text,
  observed_at datetime NOT NULL,
  payload_json text NOT NULL CHECK (json_valid(payload_json)),
  confidence real CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
  review_state varchar(20) NOT NULL DEFAULT 'PENDING'
    CHECK (review_state IN ('PENDING','APPROVED','REJECTED')),
  reviewed_by integer REFERENCES users(id) ON DELETE SET NULL,
  reviewed_at datetime,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  UNIQUE(provider, evidence_key)
);

CREATE INDEX IF NOT EXISTS cam_model_profile_provenance_model
  ON cam_model_profile_provenance(model_id, observed_at, id);
CREATE INDEX IF NOT EXISTS cam_model_profile_provenance_review
  ON cam_model_profile_provenance(review_state, observed_at, id);
