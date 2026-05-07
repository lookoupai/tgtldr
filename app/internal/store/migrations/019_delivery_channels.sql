create table if not exists delivery_channels (
    id bigint primary key generated always as identity,
    name text not null,
    enabled boolean not null default true,
    source_chat_ids bigint[] not null default '{}',
    target_chat_id text not null default '',
    target_language text not null default 'zh-CN',
    content_filter text not null default '',
    content_filter_types text[] not null default '{}',
    summary_time_local text not null default '09:00',
    summary_timezone text not null default '',
    summary_prompt text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_delivery_channels_enabled on delivery_channels (enabled);
