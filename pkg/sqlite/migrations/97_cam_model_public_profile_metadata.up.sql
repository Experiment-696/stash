ALTER TABLE cam_models ADD COLUMN location text;
ALTER TABLE cam_models ADD COLUMN age integer CHECK (age IS NULL OR (age >= 18 AND age <= 120));

