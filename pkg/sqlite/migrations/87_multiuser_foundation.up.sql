CREATE TABLE `users` (
  `id` integer not null primary key autoincrement,
  `username` varchar(255) not null,
  `normalized_username` varchar(255) not null,
  `password_hash` varchar(255),
  `role` varchar(20) not null check (`role` in ('USER', 'MODERATOR', 'ADMIN')),
  `status` varchar(40) not null check (`status` in ('ACTIVE', 'DISABLED', 'PASSWORD_CHANGE_REQUIRED')),
  `created_at` datetime not null,
  `updated_at` datetime not null
);

CREATE UNIQUE INDEX `users_normalized_username_unique` on `users` (`normalized_username`);

CREATE TABLE `user_sessions` (
  `id` varchar(64) not null primary key,
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `secret_hash` varchar(255) not null,
  `csrf_hash` varchar(255) not null,
  `created_at` datetime not null,
  `last_seen_at` datetime not null,
  `idle_expires_at` datetime not null,
  `absolute_expires_at` datetime not null,
  `revoked_at` datetime,
  `user_agent` text,
  `remote_address` text
);

CREATE UNIQUE INDEX `user_sessions_secret_hash_unique` on `user_sessions` (`secret_hash`);
CREATE INDEX `user_sessions_user_id` on `user_sessions` (`user_id`);
CREATE INDEX `user_sessions_expiry` on `user_sessions` (`idle_expires_at`, `absolute_expires_at`);

CREATE TABLE `user_api_tokens` (
  `id` varchar(64) not null primary key,
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `name` varchar(255) not null,
  `secret_hash` varchar(255) not null,
  `scopes_json` text,
  `created_at` datetime not null,
  `expires_at` datetime not null,
  `last_used_at` datetime,
  `revoked_at` datetime
);

CREATE UNIQUE INDEX `user_api_tokens_secret_hash_unique` on `user_api_tokens` (`secret_hash`);
CREATE INDEX `user_api_tokens_user_id` on `user_api_tokens` (`user_id`);
CREATE INDEX `user_api_tokens_expiry` on `user_api_tokens` (`expires_at`);

CREATE TABLE `user_preferences` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `key` varchar(255) not null,
  `value_json` text not null,
  `updated_at` datetime not null,
  primary key (`user_id`, `key`)
);

CREATE TABLE `user_audit_events` (
  `id` integer not null primary key autoincrement,
  `occurred_at` datetime not null,
  `actor_user_id` integer references `users` (`id`) on delete set null,
  `event_type` varchar(100) not null,
  `target_type` varchar(100),
  `target_id` varchar(255),
  `result` varchar(40) not null,
  `details_json` text
);

CREATE INDEX `user_audit_events_occurred_at` on `user_audit_events` (`occurred_at`);
CREATE INDEX `user_audit_events_actor` on `user_audit_events` (`actor_user_id`, `occurred_at`);

ALTER TABLE `saved_filters` ADD COLUMN `user_id` integer references `users` (`id`) on delete cascade;
CREATE INDEX `saved_filters_user_id` on `saved_filters` (`user_id`);

CREATE TABLE `user_scene_activity` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `scene_id` integer not null references `scenes` (`id`) on delete cascade,
  `resume_time` real,
  `play_duration` real,
  `updated_at` datetime not null,
  primary key (`user_id`, `scene_id`)
);

CREATE TABLE `user_scene_history` (
  `id` integer not null primary key autoincrement,
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `scene_id` integer not null references `scenes` (`id`) on delete cascade,
  `kind` varchar(20) not null check (`kind` in ('PLAY', 'O')),
  `occurred_at` datetime not null
);
CREATE INDEX `user_scene_history_owner_scene_kind` on `user_scene_history` (`user_id`, `scene_id`, `kind`, `occurred_at`);

CREATE TABLE `user_image_activity` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `image_id` integer not null references `images` (`id`) on delete cascade,
  `o_count` integer not null default 0 check (`o_count` >= 0),
  `updated_at` datetime not null,
  primary key (`user_id`, `image_id`)
);

CREATE TABLE `user_performer_state` (
  `user_id` integer not null references `users` (`id`) on delete cascade,
  `performer_id` integer not null references `performers` (`id`) on delete cascade,
  `favorite` boolean not null default 0,
  `rating` integer check (`rating` is null or (`rating` >= 1 and `rating` <= 100)),
  `updated_at` datetime not null,
  primary key (`user_id`, `performer_id`)
);
CREATE INDEX `user_performer_state_performer` on `user_performer_state` (`performer_id`);
