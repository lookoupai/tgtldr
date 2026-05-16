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
    '风险账号库',
    '记录群内明确曝光、举报、澄清和争议的账号身份风险，区分可变用户名与稳定身份 ID。',
    true,
    '{}',
    '{
      "types": {
        "risk_account": {
          "label": "风险账号",
          "fields": {
            "reported_account_username": "string",
            "reported_account_id": "string",
            "reported_account_name": "string",
            "reporter": "string",
            "risk_type": "string",
            "allegation": "string",
            "evidence": "string",
            "status": "reported|confirmed|disputed|cleared",
            "mitigation": "string"
          }
        }
      }
    }'::jsonb,
    '只抽取明确账号身份风险：某账号被点名曝光、举报、拉黑、指控诈骗/冒充/跑路/收款不发货，或围绕这类曝光的澄清和争议。不记录玩笑、辱骂、猜测或没有对象的泛泛提醒。不要因为账号本人发布敏感、不正规、灰产、博彩、成人、交易、广告或争议话题内容，就把发言者记为风险账号；当前空间只有 risk_account，遇到这类普通发言必须跳过。reported_account_username 记录被举报的 @username；reported_account_id 只有在消息中明确出现稳定数字 ID，或可从被举报账号本人消息确认时才填写；reported_account_name 记录被举报账号显示名；reporter 记录举报/曝光来源；evidence 必须写明举报/曝光/澄清依据。subjectMessageRef 指向举报、曝光或澄清消息，让事实主体代表信息来源，不要指向被举报账号的普通聊天消息。status 默认 reported；多方证据或明确结论才用 confirmed；出现反驳或未证实时用 disputed；明确澄清时用 cleared。回答时必须说明证据状态，不要把可变 @username 当成稳定身份。',
    '摘要附加时只列出风险账号、证据状态、风险类型、举报来源和规避建议；除非 status 为 confirmed，否则使用“有人举报/群内曾曝光/存在争议”的措辞，并保留置信度和事实 ID。',
    0.8,
    180,
    true
where not exists (
    select 1
    from knowledge_spaces
    where name = '风险账号库'
);
