alter table app_settings
add column if not exists bot_blocked_users text[] not null default '{}';

alter table chats
add column if not exists bot_blocked_users text[] not null default '{}';
