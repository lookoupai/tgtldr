create table if not exists knowledge_spaces (
    id bigint primary key generated always as identity,
    name text not null,
    description text not null default '',
    enabled boolean not null default true,
    chat_ids bigint[] not null default '{}',
    schema_json jsonb not null default '{}'::jsonb,
    extract_prompt text not null default '',
    summary_prompt text not null default '',
    confidence_threshold double precision not null default 0.75,
    retention_days integer not null default 30,
    include_in_summary boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists knowledge_facts (
    id bigint primary key generated always as identity,
    space_id bigint not null references knowledge_spaces(id) on delete cascade,
    chat_id bigint not null references chats(id) on delete cascade,
    fact_type text not null,
    title text not null,
    data_json jsonb not null default '{}'::jsonb,
    subject_sender_id bigint not null default 0,
    subject_sender_name text not null default '',
    subject_username text not null default '',
    confidence double precision not null default 0,
    status text not null default 'active',
    source_message_ids integer[] not null default '{}',
    first_seen_at timestamptz not null default now(),
    last_seen_at timestamptz not null default now(),
    expires_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_knowledge_facts_space_status
on knowledge_facts (space_id, status, updated_at desc);

create index if not exists idx_knowledge_facts_chat_updated
on knowledge_facts (chat_id, updated_at desc);

create unique index if not exists idx_knowledge_facts_dedupe
on knowledge_facts (space_id, chat_id, fact_type, title, subject_sender_id, source_message_ids);

create table if not exists knowledge_runs (
    id bigint primary key generated always as identity,
    space_id bigint not null references knowledge_spaces(id) on delete cascade,
    chat_id bigint not null references chats(id) on delete cascade,
    range_start timestamptz not null,
    range_end timestamptz not null,
    status text not null default 'pending',
    input_message_count integer not null default 0,
    extracted_count integer not null default 0,
    error_message text not null default '',
    started_at timestamptz not null default now(),
    finished_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_knowledge_runs_space_created
on knowledge_runs (space_id, created_at desc);
