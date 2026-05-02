alter table app_settings
add column if not exists summary_output_language text not null default 'zh-CN';

alter table chats
add column if not exists summary_language text not null default '';

update app_settings
set summary_output_language = 'zh-CN'
where trim(summary_output_language) = '';

