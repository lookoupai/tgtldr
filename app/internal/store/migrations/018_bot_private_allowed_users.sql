alter table app_settings
add column if not exists bot_private_allowed_users text[] not null default '{}';
