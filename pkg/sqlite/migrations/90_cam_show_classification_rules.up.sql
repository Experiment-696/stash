CREATE TABLE IF NOT EXISTS cam_show_classification_rules (
  id integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  name varchar(255) NOT NULL,
  pattern text NOT NULL,
  target varchar(20) NOT NULL CHECK (target IN ('BASENAME','RELATIVE_PATH')),
  category varchar(40) NOT NULL,
  enabled boolean NOT NULL DEFAULT 1,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS cam_show_classification_rules_name_unique ON cam_show_classification_rules(name COLLATE NOCASE);
CREATE TABLE IF NOT EXISTS cam_show_classification_rule_tags (
  rule_id integer NOT NULL REFERENCES cam_show_classification_rules(id) ON DELETE CASCADE,
  tag_id integer NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY(rule_id, tag_id)
);
CREATE INDEX IF NOT EXISTS cam_show_classification_rule_tags_tag ON cam_show_classification_rule_tags(tag_id, rule_id);
