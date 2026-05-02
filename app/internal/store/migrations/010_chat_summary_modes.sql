alter table chats
add column if not exists summary_mode text not null default 'channel';

alter table chats
add column if not exists topic_groups jsonb not null default '[]'::jsonb;
