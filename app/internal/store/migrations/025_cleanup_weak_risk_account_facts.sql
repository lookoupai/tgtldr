with risk_account_candidates as (
    select
        f.id,
        f.space_id,
        f.chat_id,
        f.status as previous_status,
        concat_ws(
            E'\n',
            f.title,
            f.data_json::text,
            coalesce(source_messages.text_content, '')
        ) as evidence_text
    from knowledge_facts f
    left join lateral (
        select string_agg(concat_ws(' ', m.text_content, m.caption), E'\n') as text_content
        from messages m
        where m.chat_id = f.chat_id
          and m.telegram_message_id = any(f.source_message_ids)
    ) source_messages on true
    where lower(f.fact_type) = 'risk_account'
      and f.status = 'active'
),
weak_risk_account_facts as (
    select *
    from risk_account_candidates
    where evidence_text !~* '(骗子|诈骗|欺诈|骗钱|被骗|曝光|举报|黑名单|避雷|跑路|盗号|钓鱼|冒充|收款不发货|收钱不发货|拉黑|不靠谱|失联|澄清|辟谣|争议|scam|scammer|fraud|fraudster|exposed|blacklist|phishing|impersonat|chargeback)'
),
dismissed_facts as (
    update knowledge_facts f
    set status = 'dismissed',
        updated_at = now()
    from weak_risk_account_facts weak
    where f.id = weak.id
    returning
        f.id,
        f.space_id,
        f.chat_id,
        weak.previous_status,
        f.status as next_status
)
insert into knowledge_maintenance_events (
    fact_id,
    space_id,
    chat_id,
    action,
    source,
    reason,
    operator_text,
    matched_query,
    previous_status,
    next_status
)
select
    id,
    space_id,
    chat_id,
    'dismiss',
    'migration_025',
    '风险账号抽取口径收紧：该历史事实缺少明确曝光、举报、黑名单或诈骗指控证据。',
    'auto cleanup weak risk_account facts',
    'risk_account',
    previous_status,
    next_status
from dismissed_facts;
