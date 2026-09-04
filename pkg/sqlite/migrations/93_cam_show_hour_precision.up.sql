ALTER TABLE cam_shows RENAME COLUMN captured_precision TO captured_precision_v92;
ALTER TABLE cam_shows ADD COLUMN captured_precision varchar(16)
  CHECK (captured_precision IS NULL OR captured_precision IN ('DATE','HOUR','MINUTE','SECOND'));
UPDATE cam_shows SET captured_precision = captured_precision_v92;
ALTER TABLE cam_shows DROP COLUMN captured_precision_v92;
