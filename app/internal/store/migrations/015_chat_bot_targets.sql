alter table chats
add column if not exists bot_chat_id text not null default '';

alter table chats
add column if not exists bot_interaction_enabled boolean not null default false;
