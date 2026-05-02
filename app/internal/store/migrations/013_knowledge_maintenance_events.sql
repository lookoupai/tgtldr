create table if not exists knowledge_maintenance_events (
    id bigint primary key generated always as identity,
    fact_id bigint references knowledge_facts(id) on delete set null,
    space_id bigint not null references knowledge_spaces(id) on delete cascade,
    chat_id bigint not null references chats(id) on delete cascade,
    action text not null,
    source text not null,
    reason text not null default '',
    operator_text text not null default '',
    matched_query text not null default '',
    previous_status text not null default '',
    next_status text not null default '',
    created_at timestamptz not null default now()
);

create index if not exists idx_knowledge_maintenance_events_created
on knowledge_maintenance_events (created_at desc, id desc);

create index if not exists idx_knowledge_maintenance_events_fact
on knowledge_maintenance_events (fact_id, created_at desc);

create index if not exists idx_knowledge_maintenance_events_space
on knowledge_maintenance_events (space_id, created_at desc);

create index if not exists idx_knowledge_maintenance_events_chat
on knowledge_maintenance_events (chat_id, created_at desc);
