create index if not exists idx_knowledge_runs_range
on knowledge_runs (space_id, chat_id, range_start, range_end, created_at desc);
