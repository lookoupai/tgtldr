package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
	"github.com/frederic/tgtldr/app/internal/telegramfmt"
)

const (
	BotIntentQuery       = "query"
	BotIntentFactUpsert  = "fact_upsert"
	BotIntentMaintenance = "maintenance"
	BotIntentCorrection  = "correction"
	BotIntentIgnore      = "ignore"

	botIntentMaxOutput     = 700
	BotIntentMinConfidence = 0.75
)

type BotIntent struct {
	Intent            string  `json:"intent"`
	Confidence        float64 `json:"confidence"`
	FactType          string  `json:"factType"`
	Subject           string  `json:"subject"`
	Query             string  `json:"query"`
	Item              string  `json:"item"`
	Action            string  `json:"action"`
	Replacement       string  `json:"replacement"`
	Reason            string  `json:"reason"`
	NeedsConfirmation bool    `json:"needsConfirmation"`
	SourceText        string  `json:"-"`
}

func (s *Service) ClassifyBotIntentText(ctx context.Context, text string) (BotIntent, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return BotIntent{Intent: BotIntentIgnore, Confidence: 1}, nil
	}
	if intent, ok := directBotIntent(trimmed); ok {
		return intent, nil
	}

	if s.store == nil || s.store.Settings == nil {
		return BotIntent{}, fmt.Errorf("knowledge service is not configured")
	}
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return BotIntent{}, err
	}
	client := openai.New(openai.Config{
		BaseURL: settings.OpenAIBaseURL,
		APIKey:  settings.OpenAIAPIKey,
		Model:   settings.OpenAIModel,
		Timeout: s.openAITimeout,
		Stream:  settings.OpenAIStreamEnabled(),
	})
	resp, err := client.Chat(ctx, openai.ChatRequest{
		SystemPrompt: buildBotIntentSystemPrompt(settings.Language),
		UserPrompt:   trimmed,
		Temperature:  0,
		MaxOutput:    botIntentMaxOutput,
	})
	if err != nil {
		return BotIntent{}, err
	}
	intent, err := parseBotIntent(resp.Content)
	if err != nil {
		return BotIntent{}, err
	}
	intent.SourceText = trimmed
	return intent, nil
}

func directBotIntent(text string) (BotIntent, bool) {
	if instruction, ok := parseDirectInlineFactText(text); ok {
		intent := BotIntent{
			Intent:     BotIntentFactUpsert,
			Confidence: 1,
			FactType:   instruction.FactType,
			Subject:    inlineInstructionSubject(instruction),
			Item:       instruction.Item,
			Query:      instruction.SubjectUsername,
			SourceText: text,
		}
		if instruction.FactType == "risk_account" {
			intent.Action = "cleared"
			intent.Reason = "用户澄清该账号不是风险账号"
		}
		return intent, true
	}
	if instruction, ok := parseDirectMaintenanceText(text); ok {
		return BotIntent{
			Intent:            BotIntentMaintenance,
			Confidence:        1,
			FactType:          instruction.TargetType,
			Subject:           instruction.TargetUser,
			Query:             instruction.TargetQuery,
			Action:            instruction.Action,
			Replacement:       instruction.Replacement,
			Reason:            instruction.Reason,
			NeedsConfirmation: true,
			SourceText:        text,
		}, true
	}
	return BotIntent{}, false
}

func inlineInstructionSubject(instruction inlineFactInstruction) string {
	if instruction.SubjectUsername != "" {
		return "@" + instruction.SubjectUsername
	}
	return strings.TrimSpace(instruction.SubjectName)
}

func buildBotIntentSystemPrompt(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`
You classify one Telegram bot natural-language message into an executable knowledge-base intent.
Output ONLY valid JSON in this exact shape:
{"intent":"query|fact_upsert|maintenance|correction|ignore","confidence":0.0,"factType":"demand|supply|skill|solution|resource|risk|risk_account|event|","subject":"@username or display name","query":"keyword for search or matching","item":"item/topic/resource/service when asserting a fact","action":"expire|dismiss|restore|cleared|reported|correct|","replacement":"replacement for correction","reason":"short reason","needsConfirmation":true}

Rules:
- Use "query" when the user asks what, who, whether, where, how, availability, or asks the bot to look something up.
- Use "fact_upsert" only when the message asserts reusable information, e.g. "@alice sells Telegram accounts", "Bob does API cards", "Carol can help with Rust".
- Use factType "supply" for sellers, providers, offers, services, available accounts/cards/resources.
- Use factType "demand" for needs, buying, seeking, looking for.
- Use "maintenance" when the message says an existing fact is no longer valid, should be ignored, is not risky, is cleared, or should be restored.
- Use "correction" when the message replaces a previously wrong item/topic with a corrected one.
- Use "ignore" for greetings, jokes, insults, vague chat, or insufficiently actionable text.
- For risk_account: new reported risk requires explicit exposure, report, blacklist, scam/fraud accusation, impersonation, payment-not-delivered, or runaway wording. Clarifications like "not risky" should use action "cleared" and needsConfirmation true.
- If intent is fact_upsert or maintenance and the subject/item is ambiguous, set confidence below 0.75 and needsConfirmation true.
- Do not invent usernames, services, products, accusations, or facts not present in the message.
`)
	}
	return strings.TrimSpace(`
你负责把一条发给 Telegram Bot 的自然语言消息分类成可执行的知识库意图。
只输出合法 JSON，格式必须是：
{"intent":"query|fact_upsert|maintenance|correction|ignore","confidence":0.0,"factType":"demand|supply|skill|solution|resource|risk|risk_account|event|","subject":"@用户名或显示名","query":"用于查询或匹配的关键词","item":"断言事实里的物品/主题/资源/服务","action":"expire|dismiss|restore|cleared|reported|correct|","replacement":"纠错后的内容","reason":"简短原因","needsConfirmation":true}

规则：
- 用户在问“是什么、做什么、谁、是否、哪里、怎么、有没有、查一下”时，用 query。
- 只有用户明确陈述可复用事实时才用 fact_upsert，例如“@alice 卖 Telegram 账号”“Bob 搞 API 卡”“Carol 会 Rust”。
- 卖家、供应商、提供服务、出售账号/卡/资源，用 factType supply。
- 需求、求购、想买、寻找，用 factType demand。
- 说已有事实失效、不需要了、卖完了、忽略、不是风险账号、已澄清、恢复有效，用 maintenance。
- 明确把旧事实替换成新事实时，用 correction。
- 问候、玩笑、辱骂、纯闲聊、含糊不可执行内容，用 ignore。
- risk_account 新增风险必须有明确曝光、举报、拉黑、骗子/诈骗、冒充、收款不发货、跑路等指控；“不是风险账号/已澄清”用 action cleared 且 needsConfirmation true。
- fact_upsert 或 maintenance 如果主体或物品不明确，confidence 低于 0.75，并设置 needsConfirmation true。
- 不要编造消息中不存在的用户名、服务、商品、指控或事实。
`)
}

func parseBotIntent(raw string) (BotIntent, error) {
	cleaned := strings.TrimSpace(raw)
	if match := codeFencePattern.FindStringSubmatch(cleaned); len(match) == 2 {
		cleaned = strings.TrimSpace(match[1])
	}
	var intent BotIntent
	if err := json.Unmarshal([]byte(cleaned), &intent); err != nil {
		return BotIntent{}, fmt.Errorf("parse bot intent: %w", err)
	}
	return normalizeBotIntent(intent), nil
}

func normalizeBotIntent(intent BotIntent) BotIntent {
	intent.Intent = normalizeBotIntentName(intent.Intent)
	intent.FactType = normalizeStatusUpdateFactType(intent.FactType)
	intent.Subject = compactText(intent.Subject)
	intent.Query = compactText(intent.Query)
	intent.Item = compactText(intent.Item)
	intent.Action = normalizeBotIntentAction(intent.Action)
	intent.Replacement = compactText(intent.Replacement)
	intent.Reason = compactText(intent.Reason)
	if intent.Confidence <= 0 || intent.Confidence > 1 {
		intent.Confidence = 0.5
	}
	if intent.Intent == BotIntentFactUpsert && intent.Query == "" {
		intent.Query = firstNonEmpty(intent.Item, normalizeBotIntentSubjectUsername(intent.Subject), intent.Subject)
	}
	if intent.Intent == BotIntentMaintenance && intent.Query == "" {
		intent.Query = firstNonEmpty(intent.Item, intent.Subject)
	}
	return intent
}

func normalizeBotIntentName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case BotIntentQuery:
		return BotIntentQuery
	case BotIntentFactUpsert, "fact", "upsert", "record", "assertion":
		return BotIntentFactUpsert
	case BotIntentMaintenance, "maintain", "update", "status_update":
		return BotIntentMaintenance
	case BotIntentCorrection, "correct":
		return BotIntentCorrection
	default:
		return BotIntentIgnore
	}
}

func normalizeBotIntentAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "expire", "expired", "resolve", "resolved", "no_longer_needed", "sold_out", "paused":
		return "expire"
	case "dismiss", "ignore", "remove", "delete", "forget":
		return "dismiss"
	case "restore", "active", "resume":
		return "restore"
	case "cleared", "clear", "not_risky", "not_risk_account":
		return "cleared"
	case "reported", "report", "risk":
		return "reported"
	case "correct", "correction", "replace":
		return "correct"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeBotIntentSubjectUsername(subject string) string {
	return telegramfmt.Username(subject)
}

func (intent BotIntent) LowConfidence() bool {
	return intent.Confidence > 0 && intent.Confidence < BotIntentMinConfidence
}
