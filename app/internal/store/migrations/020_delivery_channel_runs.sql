create table if not exists delivery_channel_runs (
    id bigint primary key generated always as identity,
    channel_id bigint not null references delivery_channels(id) on delete cascade,
    summary_date date not null,
    status text not null default 'pending',
    content text not null default '',
    model text not null default '',
    generated_at timestamptz,
    delivered_at timestamptz,
    delivery_error text not null default '',
    error_message text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (channel_id, summary_date)
);

create index if not exists idx_delivery_channel_runs_channel_date
on delivery_channel_runs (channel_id, summary_date desc);
