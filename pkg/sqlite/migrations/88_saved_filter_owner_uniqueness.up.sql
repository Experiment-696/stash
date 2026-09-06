DROP INDEX IF EXISTS `index_saved_filters_on_mode_name_unique`;

CREATE UNIQUE INDEX `index_saved_filters_on_owner_mode_name_unique`
ON `saved_filters` (`user_id`, `mode`, `name`);
