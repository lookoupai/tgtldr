insert into knowledge_spaces (
    name,
    description,
    enabled,
    chat_ids,
    schema_json,
    extract_prompt,
    summary_prompt,
    confidence_threshold,
    retention_days,
    include_in_summary
)
select
    '供需频道',
    '从群消息中识别需求、供应和可匹配线索，适合多群供需汇总通道。',
    true,
    '{}',
    '{
      "types": {
        "demand": {
          "label": "需求",
          "fields": {
            "item": "string",
            "quantity": "string",
            "budget": "string",
            "location": "string",
            "deadline": "string"
          }
        },
        "supply": {
          "label": "供应",
          "fields": {
            "item": "string",
            "quantity": "string",
            "price": "string",
            "location": "string"
          }
        },
        "status_update": {
          "label": "状态变更",
          "fields": {
            "target_type": "string",
            "target_query": "string",
            "action": "string",
            "reason": "string",
            "target_user": "string"
          }
        }
      }
    }'::jsonb,
    '优先抽取明确表达买、卖、求购、出售、转让、拼单、采购的信息。只记录可联系的供需事实；纯闲聊、行情讨论、机器人自动回复、没有联系人或没有明确交易意图的信息不要记录。若消息表示已买到、卖完、暂停、不再需要，请用 status_update 标记旧事实。',
    '摘要附加时按需求和供应分组，并保留可联系用户、置信度和事实 ID；不展示 status_update。',
    0.75,
    30,
    true
where not exists (
    select 1
    from knowledge_spaces
    where name = '供需频道'
);
