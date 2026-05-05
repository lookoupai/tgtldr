create table if not exists bot_target_chat_candidates (
    bot_id bigint not null,
    chat_id text not null,
    from_user_id bigint not null,
    chat_type text not null default '',
    title text not null default '',
    username text not null default '',
    from_username text not null default '',
    message_date timestamptz not null default now(),
    update_id bigint not null default 0,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (bot_id, chat_id, from_user_id)
);

create index if not exists idx_bot_target_chat_candidates_user
on bot_target_chat_candidates (bot_id, from_user_id, message_date desc, update_id desc);
