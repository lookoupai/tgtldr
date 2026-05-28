alter table app_settings
add column if not exists bot_ignore_messages_from_bots boolean not null default true;
