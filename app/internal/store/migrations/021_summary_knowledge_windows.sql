alter table chats
add column if not exists summary_knowledge_days integer not null default 0;

alter table delivery_channels
add column if not exists summary_knowledge_days integer not null default 0;
