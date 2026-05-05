alter table chats
add column if not exists bot_allowed_users text[] not null default '{}';

create table if not exists bot_runtime_state (
    id integer primary key default 1,
    bot_username text not null default '',
    last_poll_at timestamptz,
    last_update_at timestamptz,
    last_handled_at timestamptz,
    last_error text not null default '',
    updated_at timestamptz not null default now(),
    constraint bot_runtime_state_singleton check (id = 1)
);

insert into bot_runtime_state (id)
values (1)
on conflict (id) do nothing;
