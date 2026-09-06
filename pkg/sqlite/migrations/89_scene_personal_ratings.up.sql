CREATE TABLE `user_scene_state` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `scene_id` integer not null references `scenes` (`id`) on delete cascade,
  `rating` integer check (`rating` is null or (`rating` >= 1 and `rating` <= 100)),
  `updated_at` datetime not null,
  primary key (`user_id`, `scene_id`)
);
CREATE INDEX `user_scene_state_scene` on `user_scene_state` (`scene_id`);

-- Preserve the legacy single-user rating for the converted owner only. New
-- installations and databases without a conversion audit event no-op.
INSERT OR IGNORE INTO `user_scene_state` (`user_id`, `scene_id`, `rating`, `updated_at`)
SELECT owner.`actor_user_id`, scene.`id`, scene.`rating`, CURRENT_TIMESTAMP
FROM `scenes` scene
JOIN (
  SELECT `actor_user_id`
  FROM `user_audit_events`
  WHERE `event_type` = 'legacy_identity_converted' AND `actor_user_id` IS NOT NULL
  ORDER BY `id`
  LIMIT 1
) owner
WHERE scene.`rating` IS NOT NULL;
