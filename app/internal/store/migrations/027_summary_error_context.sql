alter table summaries
add column if not exists error_context text not null default '';

alter table summaries
add column if not exists error_system_prompt text not null default '';

alter table summaries
add column if not exists error_user_prompt text not null default '';
