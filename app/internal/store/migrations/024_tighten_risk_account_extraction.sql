update knowledge_spaces
set
    description = '记录群内明确曝光、举报、澄清和争议的账号身份风险，区分可变用户名与稳定身份 ID。',
    extract_prompt = '只抽取明确账号身份风险：某账号被点名曝光、举报、拉黑、指控诈骗/冒充/跑路/收款不发货，或围绕这类曝光的澄清和争议。不记录玩笑、辱骂、猜测或没有对象的泛泛提醒。不要因为账号本人发布敏感、不正规、灰产、博彩、成人、交易、广告或争议话题内容，就把发言者记为风险账号；当前空间只有 risk_account，遇到这类普通发言必须跳过。reported_account_username 记录被举报的 @username；reported_account_id 只有在消息中明确出现稳定数字 ID，或可从被举报账号本人消息确认时才填写；reported_account_name 记录被举报账号显示名；reporter 记录举报/曝光来源；evidence 必须写明举报/曝光/澄清依据。subjectMessageRef 指向举报、曝光或澄清消息，让事实主体代表信息来源，不要指向被举报账号的普通聊天消息。status 默认 reported；多方证据或明确结论才用 confirmed；出现反驳或未证实时用 disputed；明确澄清时用 cleared。回答时必须说明证据状态，不要把可变 @username 当成稳定身份。'
where name = '风险账号库';

update knowledge_spaces
set extract_prompt = '只记录未来可能复用的信息。覆盖需求、供应、技能、教程方法、工具资源、风险避坑、风险账号、活动机会。技能画像必须基于用户自述、作品、持续高质量回答或明确承诺，不能凭一句闲聊推断。风险账号请用 risk_account，但只记录明确账号身份风险：某账号被点名曝光、举报、拉黑、指控诈骗/冒充/跑路/收款不发货，或围绕这类曝光的澄清和争议。不要因为账号本人发布敏感、不正规、灰产、博彩、成人、交易、广告或争议话题内容，就把发言者记为风险账号；这类内容如果有可复用避坑价值，只能用 risk，不能用 risk_account。risk_account 的 reported_account_username 记录被举报的 @username；reported_account_id 只有在消息中明确出现稳定数字 ID 或可从被举报账号本人消息确认时才填写；reporter 记录举报/曝光来源；evidence 必须写明举报/曝光/澄清依据；status 默认 reported，只有多方证据或明确结论时才用 confirmed，出现澄清或争议时用 disputed/cleared；subjectMessageRef 指向举报/曝光/澄清消息，不要指向被举报账号的普通聊天消息；不要把可变 @username 当成稳定身份。状态变更请用 status_update，target_type 填 demand/supply/skill/help_offer 等旧事实类型，target_query 填要失效的物品或主题，action 使用 resolved、expired、sold_out、paused、no_longer_needed 等英文短语。不要记录玩笑、猜测、纯闲聊、临时情绪或无证据结论。'
where name = '通用群聊知识库';
