-- auth_users dropped device columns; date_actived renamed; users.authorization_ttl_days.
CALL teamgram_drop_column('auth_users', 'layer');
CALL teamgram_drop_column('auth_users', 'device_model');
CALL teamgram_drop_column('auth_users', 'platform');
CALL teamgram_drop_column('auth_users', 'system_version');
CALL teamgram_drop_column('auth_users', 'api_id');
CALL teamgram_drop_column('auth_users', 'app_name');
CALL teamgram_drop_column('auth_users', 'app_version');
CALL teamgram_drop_column('auth_users', 'ip');
CALL teamgram_drop_column('auth_users', 'country');
CALL teamgram_drop_column('auth_users', 'region');
CALL teamgram_change_column('auth_users', 'date_actived', 'ALTER TABLE `auth_users` CHANGE `date_actived` `date_active` BIGINT NOT NULL DEFAULT ''0''');
CALL teamgram_add_column('users', 'authorization_ttl_days', 'ALTER TABLE `users` ADD `authorization_ttl_days` INT NOT NULL DEFAULT ''180'' AFTER `personal_channel_id`');
