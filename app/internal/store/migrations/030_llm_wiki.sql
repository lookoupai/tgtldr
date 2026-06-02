create table if not exists llm_wiki_pages (
    id bigint primary key generated always as identity,
    space_id bigint not null default 0,
    path text not null unique,
    title text not null default '',
    page_type text not null default 'page',
    content_hash text not null default '',
    content_text text not null default '',
    source_fact_ids bigint[] not null default '{}',
    source_message_refs jsonb not null default '[]'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    indexed_at timestamptz not null default now()
);

create index if not exists idx_llm_wiki_pages_space_type_updated
on llm_wiki_pages (space_id, page_type, updated_at desc);

create extension if not exists pg_trgm;

create index if not exists idx_llm_wiki_pages_title_trgm
on llm_wiki_pages
using gin (title gin_trgm_ops);

create index if not exists idx_llm_wiki_pages_path_trgm
on llm_wiki_pages
using gin (path gin_trgm_ops);

create index if not exists idx_llm_wiki_pages_content_trgm
on llm_wiki_pages
using gin (content_text gin_trgm_ops);

create table if not exists llm_wiki_runs (
    id bigint primary key generated always as identity,
    space_id bigint not null default 0,
    chat_id bigint not null default 0,
    summary_id bigint not null default 0,
    range_start timestamptz,
    range_end timestamptz,
    status text not null default 'pending',
    updated_page_count integer not null default 0,
    error_message text not null default '',
    started_at timestamptz not null default now(),
    finished_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_llm_wiki_runs_created
on llm_wiki_runs (created_at desc);
