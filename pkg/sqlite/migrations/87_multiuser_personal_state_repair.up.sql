-- Forward repair for databases stamped schema 86 before the personal-state
-- and activity tables were appended to migration 86 during live staging.
CREATE TABLE IF NOT EXISTS `user_scene_activity` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `scene_id` integer not null references `scenes` (`id`) on delete cascade,
  `resume_time` real,
  `play_duration` real,
  `updated_at` datetime not null,
  primary key (`user_id`, `scene_id`)
);

CREATE TABLE IF NOT EXISTS `user_scene_history` (
  `id` integer not null primary key autoincrement,
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `scene_id` integer not null references `scenes` (`id`) on delete cascade,
  `kind` varchar(20) not null check (`kind` in ('PLAY', 'O')),
  `occurred_at` datetime not null
);
CREATE INDEX IF NOT EXISTS `user_scene_history_owner_scene_kind` on `user_scene_history` (`user_id`, `scene_id`, `kind`, `occurred_at`);

CREATE TABLE IF NOT EXISTS `user_image_activity` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `image_id` integer not null references `images` (`id`) on delete cascade,
  `o_count` integer not null default 0 check (`o_count` >= 0),
  `updated_at` datetime not null,
  primary key (`user_id`, `image_id`)
);

CREATE TABLE IF NOT EXISTS `user_performer_state` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `performer_id` integer not null references `performers` (`id`) on delete cascade,
  `favorite` boolean not null default 0,
  `rating` integer check (`rating` is null or (`rating` >= 1 and `rating` <= 100)),
  `updated_at` datetime not null,
  primary key (`user_id`, `performer_id`)
);
CREATE INDEX IF NOT EXISTS `user_performer_state_performer` on `user_performer_state` (`performer_id`);

-- v86's legacy conversion audit event identifies the one account that owned
-- the former global performer state. Preserve that state without assigning it
-- to unrelated Admin accounts. Fresh databases and repaired retries no-op.
INSERT OR IGNORE INTO `user_performer_state` (`user_id`, `performer_id`, `favorite`, `rating`, `updated_at`)
SELECT owner.`actor_user_id`, performer.`id`, performer.`favorite`, performer.`rating`, CURRENT_TIMESTAMP
FROM `performers` performer
JOIN (
  SELECT `actor_user_id`
  FROM `user_audit_events`
  WHERE `event_type` = 'legacy_identity_converted' AND `actor_user_id` IS NOT NULL
  ORDER BY `id`
  LIMIT 1
) owner
WHERE performer.`favorite` = 1 OR performer.`rating` IS NOT NULL;
