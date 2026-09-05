ALTER TABLE cam_shows ADD COLUMN rate real CHECK (rate IS NULL OR rate >= 0);
ALTER TABLE cam_shows ADD COLUMN extras text;
ALTER TABLE cam_shows ADD COLUMN request text;
UPDATE cam_shows SET extras = notes WHERE notes IS NOT NULL AND trim(notes) <> '';

CREATE TABLE cam_show_user_state (
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  show_id integer NOT NULL REFERENCES cam_shows(id) ON DELETE CASCADE,
  rating integer CHECK (rating IS NULL OR (rating >= 1 AND rating <= 100)),
  updated_at datetime NOT NULL,
  PRIMARY KEY(user_id, show_id)
);
CREATE INDEX cam_show_user_state_show ON cam_show_user_state(show_id);
