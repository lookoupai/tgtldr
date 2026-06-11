package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
	"github.com/frederic/tgtldr/app/internal/telegramfmt"
)

const inlineFactConfidence = 0.95

type InlineFactSource struct {
	ChatID         int64
	MessageID      int64
	SenderID       int64
	SenderName     string
	SenderUsername string
	MessageTime    time.Time
}

type InlineFactResult struct {
	Facts []model.KnowledgeFact `json:"facts"`
}

type inlineFactInstruction struct {
	FactType        string
	SubjectUsername string
	SubjectName     string
	Item            string
	SourceText      string
}

type inlineFactVerb struct {
	factType string
	verb     string
}

var inlineFactVerbs = []inlineFactVerb{
	{factType: "supply", verb: "供应"},
	{factType: "supply", verb: "出售"},
	{factType: "supply", verb: "售卖"},
	{factType: "supply", verb: "提供"},
	{factType: "supply", verb: "转让"},
	{factType: "supply", verb: "卖"},
	{factType: "demand", verb: "求购"},
	{factType: "demand", verb: "需要"},
	{factType: "demand", verb: "想买"},
	{factType: "demand", verb: "购买"},
	{factType: "demand", verb: "买"},
	{factType: "demand", verb: "收"},
}

func (s *Service) RecordInlineFactText(ctx context.Context, text string, source InlineFactSource) (InlineFactResult, error) {
	instruction, ok := parseDirectInlineFactText(text)
	if !ok {
		return InlineFactResult{}, nil
	}
	return s.recordInlineFactInstruction(ctx, instruction, source)
}

func (s *Service) RecordBotIntentFact(ctx context.Context, intent BotIntent, source InlineFactSource) (InlineFactResult, error) {
	if intent.Intent != BotIntentFactUpsert || intent.LowConfidence() {
		return InlineFactResult{}, nil
	}
	instruction, ok := inlineFactInstructionFromBotIntent(intent)
	if !ok {
		return InlineFactResult{}, nil
	}
	return s.recordInlineFactInstruction(ctx, instruction, source)
}

func inlineFactInstructionFromBotIntent(intent BotIntent) (inlineFactInstruction, bool) {
	factType := normalizeStatusUpdateFactType(intent.FactType)
	switch factType {
	case "demand", "supply", "skill", "risk_account":
	default:
		return inlineFactInstruction{}, false
	}

	subjectUsername := telegramfmt.Username(intent.Subject)
	subjectName := strings.TrimSpace(intent.Subject)
	if subjectUsername != "" {
		subjectName = "@" + subjectUsername
	}
	if subjectName == "" {
		return inlineFactInstruction{}, false
	}

	item := strings.TrimSpace(intent.Item)
	if factType != "risk_account" && !validInlineFactItem(item) {
		return inlineFactInstruction{}, false
	}
	return inlineFactInstruction{
		FactType:        factType,
		SubjectUsername: subjectUsername,
		SubjectName:     subjectName,
		Item:            item,
		SourceText:      strings.TrimSpace(firstNonEmpty(intent.SourceText, intent.Reason, intent.Item, intent.Query)),
	}, true
}

func (s *Service) recordInlineFactInstruction(ctx context.Context, instruction inlineFactInstruction, source InlineFactSource) (InlineFactResult, error) {
	if source.ChatID <= 0 {
		return InlineFactResult{}, nil
	}
	if s.store == nil || s.store.KnowledgeSpaces == nil || s.store.KnowledgeFacts == nil {
		return InlineFactResult{}, fmt.Errorf("knowledge service is not configured")
	}

	space, ok, err := s.selectInlineFactSpace(ctx, source.ChatID, instruction.FactType)
	if err != nil {
		return InlineFactResult{}, err
	}
	if !ok {
		return InlineFactResult{}, nil
	}

	fact, err := s.buildInlineFact(space, instruction, source)
	if err != nil {
		return InlineFactResult{}, err
	}
	if err := s.store.KnowledgeFacts.UpsertMany(ctx, []model.KnowledgeFact{fact}); err != nil {
		return InlineFactResult{}, err
	}

	facts, err := s.findRecordedInlineFacts(ctx, space, source.ChatID, instruction)
	if err != nil {
		return InlineFactResult{}, err
	}
	if len(facts) == 0 {
		facts = []model.KnowledgeFact{fact}
	}
	return InlineFactResult{Facts: facts}, nil
}

func parseDirectInlineFactText(text string) (inlineFactInstruction, bool) {
	trimmed := compactText(text)
	username, subjectName, rest, ok := splitLeadingInlineMention(trimmed)
	if !ok {
		return inlineFactInstruction{}, false
	}

	if inlineRiskCleared(rest) && !LooksLikeQuestionText(rest) {
		return inlineFactInstruction{
			FactType:        "risk_account",
			SubjectUsername: username,
			SubjectName:     subjectName,
			SourceText:      trimmed,
		}, true
	}

	rest = trimInlineCopula(rest)
	for _, verb := range inlineFactVerbs {
		if !strings.HasPrefix(rest, verb.verb) {
			continue
		}
		item := trimInlineItem(strings.TrimPrefix(rest, verb.verb))
		if !validInlineFactItem(item) {
			return inlineFactInstruction{}, false
		}
		return inlineFactInstruction{
			FactType:        verb.factType,
			SubjectUsername: username,
			SubjectName:     subjectName,
			Item:            item,
			SourceText:      trimmed,
		}, true
	}
	return inlineFactInstruction{}, false
}

func splitLeadingInlineMention(text string) (string, string, string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "@") {
		return "", "", "", false
	}
	end := 1
	for end < len(trimmed) && inlineUsernameByte(trimmed[end]) {
		end++
	}
	if end <= 1 {
		return "", "", "", false
	}
	username := telegramfmt.Username(trimmed[:end])
	if username == "" {
		return "", "", "", false
	}
	return username, "@" + username, strings.TrimSpace(trimmed[end:]), true
}

func inlineUsernameByte(value byte) bool {
	return value == '_' ||
		('a' <= value && value <= 'z') ||
		('A' <= value && value <= 'Z') ||
		('0' <= value && value <= '9')
}

func inlineRiskCleared(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return containsAny(lower, []string{
		"不是风险账号",
		"非风险账号",
		"不是骗子",
		"不是 scammer",
		"not a scammer",
		"not scammer",
		"not risk account",
		"已澄清",
		"澄清了",
	})
}

func trimInlineCopula(text string) string {
	trimmed := strings.TrimSpace(text)
	for _, prefix := range []string{"是一个", "是个", "是"} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return trimmed
}

func trimInlineItem(text string) string {
	item := strings.TrimSpace(text)
	for _, prefix := range []string{"的是", "的", "为"} {
		if strings.HasPrefix(item, prefix) {
			item = strings.TrimSpace(strings.TrimPrefix(item, prefix))
		}
	}
	item = strings.Trim(item, " \t\r\n，,。.;；：:!?！？")
	item = strings.TrimSuffix(item, "的")
	return strings.Trim(item, " \t\r\n，,。.;；：:!?！？")
}

func validInlineFactItem(item string) bool {
	if strings.TrimSpace(item) == "" {
		return false
	}
	normalized := strings.ToLower(item)
	for _, marker := range []string{"什么", "吗", "？", "?", "谁", "where", "what", "how"} {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	return true
}

func (s *Service) selectInlineFactSpace(ctx context.Context, chatID int64, factType string) (model.KnowledgeSpace, bool, error) {
	spaces, err := s.store.KnowledgeSpaces.List(ctx)
	if err != nil {
		return model.KnowledgeSpace{}, false, err
	}

	var selected model.KnowledgeSpace
	selectedScore := 0
	for _, space := range spaces {
		if !space.Enabled || !spaceAppliesToChat(space, chatID) || !knowledgeSpaceSupportsFactType(space, factType) {
			continue
		}
		score := inlineFactSpaceScore(space, factType)
		if selected.ID == 0 || score > selectedScore || score == selectedScore && space.ID < selected.ID {
			selected = space
			selectedScore = score
		}
	}
	return selected, selected.ID > 0, nil
}

func knowledgeSpaceSupportsFactType(space model.KnowledgeSpace, factType string) bool {
	var schema struct {
		Types map[string]json.RawMessage `json:"types"`
	}
	if err := json.Unmarshal([]byte(space.SchemaJSON), &schema); err != nil {
		return false
	}
	_, ok := schema.Types[strings.TrimSpace(factType)]
	return ok
}

func inlineFactSpaceScore(space model.KnowledgeSpace, factType string) int {
	name := strings.ToLower(strings.TrimSpace(space.Name))
	switch strings.ToLower(strings.TrimSpace(factType)) {
	case "risk_account":
		if strings.Contains(space.Name, "风险账号") || strings.Contains(name, "risk") {
			return 4
		}
	case "supply", "demand":
		if strings.Contains(space.Name, "供需") || strings.Contains(name, "market") {
			return 4
		}
	}
	if strings.TrimSpace(space.Name) == defaultGeneralKnowledgeSpaceName {
		return 2
	}
	return 1
}

func (s *Service) buildInlineFact(space model.KnowledgeSpace, instruction inlineFactInstruction, source InlineFactSource) (model.KnowledgeFact, error) {
	seenAt := source.MessageTime
	if seenAt.IsZero() {
		seenAt = s.now()
	}
	expiresAt := seenAt.Add(time.Duration(space.RetentionDays) * 24 * time.Hour)

	data, err := inlineFactData(instruction, source)
	if err != nil {
		return model.KnowledgeFact{}, err
	}
	fact := model.KnowledgeFact{
		SpaceID:          space.ID,
		SpaceName:        space.Name,
		ChatID:           source.ChatID,
		FactType:         instruction.FactType,
		Title:            inlineFactTitle(instruction),
		DataJSON:         data,
		Confidence:       inlineFactConfidence,
		Status:           model.KnowledgeFactStatusActive,
		SourceMessageIDs: inlineFactSourceMessageIDs(source),
		FirstSeenAt:      seenAt,
		LastSeenAt:       seenAt,
		ExpiresAt:        &expiresAt,
	}
	if instruction.FactType == "risk_account" {
		fact.SubjectSenderID = source.SenderID
		fact.SubjectSenderName = strings.TrimSpace(source.SenderName)
		fact.SubjectUsername = strings.TrimPrefix(strings.TrimSpace(source.SenderUsername), "@")
		return fact, nil
	}
	fact.SubjectSenderName = instruction.SubjectName
	fact.SubjectUsername = instruction.SubjectUsername
	return fact, nil
}

func inlineFactData(instruction inlineFactInstruction, source InlineFactSource) (string, error) {
	var payload map[string]string
	switch instruction.FactType {
	case "risk_account":
		payload = map[string]string{
			"reported_account_username": instruction.SubjectUsername,
			"reported_account_id":       "",
			"reported_account_name":     instruction.SubjectName,
			"reporter":                  inlineFactSourceLabel(source),
			"risk_type":                 "clarification",
			"allegation":                "不是风险账号",
			"evidence":                  instruction.SourceText,
			"status":                    "cleared",
			"mitigation":                "",
		}
	case "demand", "supply":
		payload = map[string]string{
			"item":     instruction.Item,
			"contact":  instruction.SubjectName,
			"status":   "active",
			"evidence": instruction.SourceText,
		}
	case "skill":
		payload = map[string]string{
			"area":     instruction.Item,
			"evidence": instruction.SourceText,
			"level":    "",
		}
	default:
		payload = map[string]string{"evidence": instruction.SourceText}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func inlineFactSourceLabel(source InlineFactSource) string {
	if name := strings.TrimSpace(source.SenderName); name != "" {
		return name
	}
	if username := telegramfmt.Username(source.SenderUsername); username != "" {
		return "@" + username
	}
	if source.SenderID > 0 {
		return strconv.FormatInt(source.SenderID, 10)
	}
	return ""
}

func inlineFactTitle(instruction inlineFactInstruction) string {
	switch instruction.FactType {
	case "risk_account":
		return "账号 " + instruction.SubjectName + " 被澄清不是风险账号"
	case "demand":
		return "需求 " + instruction.Item
	case "supply":
		return "供应 " + instruction.Item
	case "skill":
		return "技能 " + instruction.Item
	default:
		return instruction.Item
	}
}

func inlineFactSourceMessageIDs(source InlineFactSource) []int {
	if source.MessageID <= 0 {
		return nil
	}
	return []int{int(source.MessageID)}
}

func (s *Service) findRecordedInlineFacts(ctx context.Context, space model.KnowledgeSpace, chatID int64, instruction inlineFactInstruction) ([]model.KnowledgeFact, error) {
	query := firstNonEmpty(instruction.SubjectUsername, instruction.SubjectName, instruction.Item)
	facts, err := s.store.KnowledgeFacts.List(ctx, store.KnowledgeFactFilter{
		SpaceID:  space.ID,
		ChatID:   chatID,
		Status:   model.KnowledgeFactStatusActive,
		FactType: instruction.FactType,
		Query:    query,
		Limit:    20,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.KnowledgeFact, 0, len(facts))
	for _, fact := range facts {
		if inlineFactMatchesInstruction(fact, instruction) {
			out = append(out, fact)
		}
	}
	return out, nil
}

func inlineFactMatchesInstruction(fact model.KnowledgeFact, instruction inlineFactInstruction) bool {
	if !strings.EqualFold(fact.FactType, instruction.FactType) {
		return false
	}
	if strings.TrimSpace(fact.Title) != inlineFactTitle(instruction) {
		return false
	}
	if instruction.FactType == "risk_account" {
		target := strings.ToLower(fact.DataJSON + " " + fact.Title)
		return strings.Contains(target, strings.ToLower(firstNonEmpty(instruction.SubjectUsername, instruction.SubjectName)))
	}
	if instruction.SubjectUsername != "" && strings.EqualFold(strings.TrimPrefix(fact.SubjectUsername, "@"), instruction.SubjectUsername) {
		return true
	}
	return instruction.SubjectName != "" && strings.EqualFold(fact.SubjectSenderName, instruction.SubjectName)
}
