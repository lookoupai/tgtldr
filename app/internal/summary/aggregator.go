package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/clock"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
	"github.com/frederic/tgtldr/app/internal/store"
	"golang.org/x/sync/errgroup"
)

type Aggregator struct {
	store         *store.Store
	clock         clock.Clock
	openAITimeout time.Duration
}

func NewAggregator(st *store.Store, c clock.Clock, openAITimeout time.Duration) *Aggregator {
	return &Aggregator{store: st, clock: c, openAITimeout: openAITimeout}
}

type AggregatedSummaryResult struct {
	ChannelID         int64
	SummaryDate       string
	Content           string
	Model             string
	SourceChatIDs     []int64
	SourceMessageIDs  []int
	FilteredFactTypes []string
	Status            model.SummaryStatus
	ErrorMessage      string
	GeneratedAt       time.Time
}

func (a *Aggregator) RunAggregatedSummary(ctx context.Context, channel model.DeliveryChannel, date string) (AggregatedSummaryResult, error) {
	settings, err := a.store.Settings.Get(ctx)
	if err != nil {
		return AggregatedSummaryResult{}, err
	}

	timezone := resolveChannelTimezone(channel, settings.DefaultTimezone)
	location, err := loadLocation(timezone)
	if err != nil {
		return AggregatedSummaryResult{}, err
	}
	start, end, err := dayRange(date, timezone)
	if err != nil {
		return AggregatedSummaryResult{}, err
	}

	allMessages, messageLookup, err := a.aggregateMessages(ctx, channel.SourceChatIDs, start, end)
	if err != nil {
		return AggregatedSummaryResult{}, err
	}

	if len(allMessages) == 0 {
		result := AggregatedSummaryResult{
			ChannelID:     channel.ID,
			SummaryDate:   date,
			Content:       emptySummaryContent(channel.TargetLanguage),
			Status:        model.SummaryStatusSucceeded,
			GeneratedAt:   a.clock.Now(),
			SourceChatIDs: channel.SourceChatIDs,
		}
		if err := a.appendKnowledgeFacts(ctx, &result, channel, end); err != nil {
			return AggregatedSummaryResult{}, err
		}
		return result, nil
	}

	filteredMessages := allMessages
	if channel.ContentFilter != "" && len(channel.ContentFilterTypes) > 0 {
		filteredMessages = a.filterMessagesByContent(ctx, allMessages, channel.ContentFilter, channel.ContentFilterTypes, settings)
	}

	result := AggregatedSummaryResult{
		ChannelID:         channel.ID,
		SummaryDate:       date,
		Status:            model.SummaryStatusSucceeded,
		GeneratedAt:       a.clock.Now(),
		SourceChatIDs:     channel.SourceChatIDs,
		FilteredFactTypes: channel.ContentFilterTypes,
	}

	client := openai.New(openai.Config{
		BaseURL: settings.OpenAIBaseURL,
		APIKey:  settings.OpenAIAPIKey,
		Model:   settings.OpenAIModel,
		Timeout: a.openAITimeout,
		Stream:  settings.OpenAIStreamEnabled(),
	})

	stagePrompt := buildAggregatedStagePrompt(channel.TargetLanguage, channel.SummaryPrompt, channel.ContentFilter, channel.ContentFilterTypes)
	finalPrompt := buildAggregatedFinalPrompt(channel.TargetLanguage, channel.SummaryPrompt, channel.ContentFilter, channel.ContentFilterTypes)
	budget := resolveSummaryBudget(settings, settings.OpenAIModel, stagePrompt)
	chunks := SplitMessages(filteredMessages, budget.ChunkTokenBudget)

	partials := make([]string, len(chunks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(budget.Parallelism)

	for index, chunk := range chunks {
		index := index
		chunk := chunk
		group.Go(func() error {
			transcript := BuildTranscript(chunk.Messages, messageLookup, location, channel.TargetLanguage)
			resp, err := chatOpenAIForSummary(groupCtx, client, openai.ChatRequest{
				SystemPrompt: stagePrompt,
				UserPrompt:   transcript,
				Temperature:  settings.OpenAITemperature,
				MaxOutput:    budget.StageRequestMax,
			}, summaryOpenAICallContext{
				Kind:                 "aggregated_summary",
				Stage:                "chunk",
				ChannelID:            channel.ID,
				SummaryDate:          date,
				Timezone:             timezone,
				Model:                settings.OpenAIModel,
				BaseURL:              settings.OpenAIBaseURL,
				RequestMode:          model.NormalizeOpenAIRequestMode(settings.OpenAIRequestMode),
				Temperature:          settings.OpenAITemperature,
				MaxOutput:            budget.StageRequestMax,
				Parallelism:          budget.Parallelism,
				ChunkIndex:           index,
				ChunkCount:           len(chunks),
				SourceMessageCount:   len(allMessages),
				ChunkMessageCount:    len(chunk.Messages),
				FilteredMessageCount: len(filteredMessages),
				SourceChatIDs:        channel.SourceChatIDs,
				ContentFilterEnabled: channel.ContentFilter != "" && len(channel.ContentFilterTypes) > 0,
				FilteredFactTypes:    channel.ContentFilterTypes,
				InputRunes:           len([]rune(transcript)),
			})
			if err != nil {
				return err
			}
			partials[index] = strings.TrimSpace(resp.Content)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		result.Status = model.SummaryStatusFailed
		result.ErrorMessage = err.Error()
		return result, nil
	}

	finalInput := strings.Join(partials, "\n\n---\n\n")
	finalResp, err := chatOpenAIForSummary(ctx, client, openai.ChatRequest{
		SystemPrompt: finalPrompt,
		UserPrompt:   finalInput,
		Temperature:  settings.OpenAITemperature,
		MaxOutput:    budget.FinalRequestMax,
	}, summaryOpenAICallContext{
		Kind:                 "aggregated_summary",
		Stage:                "final",
		ChannelID:            channel.ID,
		SummaryDate:          date,
		Timezone:             timezone,
		Model:                settings.OpenAIModel,
		BaseURL:              settings.OpenAIBaseURL,
		RequestMode:          model.NormalizeOpenAIRequestMode(settings.OpenAIRequestMode),
		Temperature:          settings.OpenAITemperature,
		MaxOutput:            budget.FinalRequestMax,
		Parallelism:          budget.Parallelism,
		ChunkCount:           len(chunks),
		SourceMessageCount:   len(allMessages),
		FilteredMessageCount: len(filteredMessages),
		SourceChatIDs:        channel.SourceChatIDs,
		ContentFilterEnabled: channel.ContentFilter != "" && len(channel.ContentFilterTypes) > 0,
		FilteredFactTypes:    channel.ContentFilterTypes,
		InputRunes:           len([]rune(finalInput)),
	})
	if err != nil {
		result.Status = model.SummaryStatusFailed
		result.ErrorMessage = err.Error()
		return result, nil
	}

	result.Content = sanitizeSummaryInternalReferences(strings.TrimSpace(finalResp.Content))
	result.Content = sanitizeSummaryUserLinks(result.Content, filteredMessages, messageLookup)
	result.Model = finalResp.Model
	if err := a.appendKnowledgeFacts(ctx, &result, channel, end); err != nil {
		return AggregatedSummaryResult{}, err
	}
	result.Content, err = appendSourceMessageLinks(ctx, a.store, result.Content, filteredMessages, messageLookup, channel.TargetLanguage)
	if err != nil {
		return AggregatedSummaryResult{}, err
	}
	return result, nil
}

func (a *Aggregator) appendKnowledgeFacts(ctx context.Context, result *AggregatedSummaryResult, channel model.DeliveryChannel, before time.Time) error {
	if a.store == nil || a.store.KnowledgeFacts == nil || result == nil || result.Status != model.SummaryStatusSucceeded {
		return nil
	}
	content, err := appendKnowledgeFactsForChats(ctx, a.store, a.clock.Now(), result.Content, result.SourceChatIDs, channel.TargetLanguage, channel.SummaryKnowledgeDays, before, nil)
	if err != nil {
		return err
	}
	result.Content = content
	return nil
}

func (a *Aggregator) aggregateMessages(ctx context.Context, chatIDs []int64, start, end time.Time) ([]model.Message, map[int]model.Message, error) {
	allMessages := make([]model.Message, 0)
	messageLookup := make(map[int]model.Message)

	for _, chatID := range chatIDs {
		chat, err := a.store.Chats.GetByID(ctx, chatID)
		if err != nil {
			return nil, nil, fmt.Errorf("get source chat %d: %w", chatID, err)
		}
		messages, err := a.store.Messages.ListForRange(ctx, chatID, start, end)
		if err != nil {
			return nil, nil, fmt.Errorf("get messages for chat %d: %w", chatID, err)
		}
		allMessages = appendAggregatedMessages(allMessages, messageLookup, messages, chat)
	}

	missingReplyIDs := make([]int, 0)
	for _, message := range allMessages {
		if message.ReplyToMessageID == 0 {
			continue
		}
		if _, ok := messageLookup[message.ReplyToMessageID]; ok {
			continue
		}
		missingReplyIDs = append(missingReplyIDs, message.ReplyToMessageID)
	}

	if len(missingReplyIDs) > 0 {
		for _, chatID := range chatIDs {
			referenced, err := a.store.Messages.LookupByTelegramIDs(ctx, chatID, uniqueInts(missingReplyIDs))
			if err != nil {
				continue
			}
			for messageID, message := range referenced {
				messageLookup[messageID] = message
			}
		}
	}

	if err := enrichSenderUsernames(ctx, a.store.Messages, allMessages, messageLookup); err != nil {
		return nil, nil, err
	}

	return allMessages, messageLookup, nil
}

func appendAggregatedMessages(allMessages []model.Message, messageLookup map[int]model.Message, messages []model.Message, chat model.Chat) []model.Message {
	for _, message := range messages {
		messageLookup[message.TelegramMessageID] = message
	}
	return append(allMessages, filterMessagesForSummary(messages, chat)...)
}

func (a *Aggregator) filterMessagesByContent(ctx context.Context, messages []model.Message, filter string, factTypes []string, settings model.AppSettings) []model.Message {
	if filter == "" || len(factTypes) == 0 {
		return messages
	}

	filtered := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		filtered = append(filtered, message)
	}
	return filtered
}

func resolveChannelTimezone(channel model.DeliveryChannel, fallback string) string {
	if timezone := strings.TrimSpace(channel.SummaryTimezone); timezone != "" {
		return timezone
	}
	if timezone := strings.TrimSpace(fallback); timezone != "" {
		return timezone
	}
	return time.Local.String()
}

func buildAggregatedStagePrompt(language model.SummaryOutputLanguage, extraPrompt string, filter string, factTypes []string) string {
	base := stagePromptBase(language)
	if filter != "" && len(factTypes) > 0 {
		base = buildFilteredStagePromptBase(language, filter, factTypes)
	}
	return buildAggregatedSystemPrompt(language, base, extraPrompt, filter, factTypes)
}

func buildAggregatedFinalPrompt(language model.SummaryOutputLanguage, extraPrompt string, filter string, factTypes []string) string {
	base := finalPromptBase(language)
	if filter != "" && len(factTypes) > 0 {
		base = buildFilteredFinalPromptBase(language, filter, factTypes)
	}
	return buildAggregatedSystemPrompt(language, base, extraPrompt, filter, factTypes)
}

func buildFilteredStagePromptBase(language model.SummaryOutputLanguage, filter string, factTypes []string) string {
	typeList := strings.Join(factTypes, ", ")
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's filtered stage summarizer. You will read messages from multiple Telegram groups and extract ONLY specific types of information.

Focus areas: ` + typeList + `

Rules:
- Extract only information related to the focus areas above
- Ignore unrelated discussions, greetings, and off-topic content
- Preserve the source group context for each item
- Group similar items across different source groups
- Maintain clear attribution of which group each piece of information came from

Prioritize:
- Clear, actionable information matching the focus areas
- Items with explicit details (who, what, when, where)
- Cross-group patterns and comparisons

` + outputLanguageInstruction(language) + ` and use this structure:

## Main Items
- List the main items matching the focus areas

## Grouped by Source
### Group: <name>
- Items from this group

## Cross-Group Patterns
- Patterns or comparisons across groups
`
	}
	return `
你是 TGTLDR 的过滤型阶段摘要器。你将阅读来自多个 Telegram 群组的消息，并只提取特定类型的信息。

关注类型：` + typeList + `

规则：
- 只提取与关注类型相关的信息
- 忽略无关讨论、问候和跑题内容
- 保留每条信息的来源群组上下文
- 对不同来源群组的相似内容进行分组
- 明确标注每条信息来自哪个群组

优先关注：
- 符合关注类型的清晰、可操作信息
- 带有明确细节（谁、什么、何时、何地）的内容
- 跨群组的模式和对比

请使用中文输出，并按以下结构整理：

## 主要内容
- 列出符合关注类型的主要内容

## 按来源群组分组
### 群组：<名称>
- 该群组的相关内容

## 跨群组模式
- 跨群组的模式或对比
`
}

func buildFilteredFinalPromptBase(language model.SummaryOutputLanguage, filter string, factTypes []string) string {
	typeList := strings.Join(factTypes, ", ")
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's filtered final summarizer. You will receive stage summaries from multiple Telegram groups and create a consolidated digest focused on specific information types.

Focus areas: ` + typeList + `

Rules:
- Merge similar items across groups while preserving source attribution
- Prioritize actionable information over discussion
- Clearly mark which group each item came from
- Highlight cross-group patterns and comparisons

` + outputLanguageInstruction(language) + ` and use this format:

## Summary by Type
### <Type name>
- Items of this type with source attribution

## Cross-Group Highlights
- Notable patterns or comparisons across groups

## Active Items
- Current valid items requiring attention
`
	}
	return `
你是 TGTLDR 的过滤型最终摘要器。你会收到来自多个 Telegram 群组的阶段摘要，需要整理成专注于特定类型信息的汇总摘要。

关注类型：` + typeList + `

规则：
- 合并不同群组的相似内容，同时保留来源标注
- 优先提取可操作信息而非讨论过程
- 明确标注每条信息来自哪个群组
- 突出跨群组的模式和对比

请使用中文输出，并按以下格式：

## 按类型汇总
### <类型名称>
- 该类型的内容及来源群组

## 跨群组要点
- 值得关注的跨群组模式或对比

## 当前有效项
- 需要关注的当前有效信息
`
}

func buildAggregatedSystemPrompt(language model.SummaryOutputLanguage, base string, extraPrompt string, filter string, factTypes []string) string {
	sections := []string{strings.TrimSpace(base), preserveUserLinkInstruction(language)}

	if extraPrompt != "" {
		sections = append(sections, sectionLabel(language, extraPromptLabel(language))+"\n"+strings.TrimSpace(extraPrompt))
	}

	return strings.Join(sections, "\n\n")
}

func uniqueInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
