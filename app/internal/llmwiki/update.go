package llmwiki

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
	"github.com/frederic/tgtldr/app/internal/store"
)

const (
	wikiUpdateCandidateLimit       = 6
	wikiUpdateMaxUpdates           = 8
	wikiUpdateMaxOutput            = 5000
	wikiUpdateSummaryMaxRunes      = 9000
	wikiUpdateFactEvidenceMaxRunes = 6000
	wikiUpdatePageEvidenceMaxRunes = 9000
	wikiUpdatePageMaxRunes         = 20000
)

var wikiUpdateCodeFencePattern = regexp.MustCompile("(?is)^```+\\s*(?:json)?\\s*(.*?)\\s*```+$")

type updateResponse struct {
	Updates  []pageUpdate `json:"updates"`
	LogEntry string       `json:"logEntry"`
}

type pageUpdate struct {
	Path          string  `json:"path"`
	Title         string  `json:"title"`
	Type          string  `json:"type"`
	PageType      string  `json:"pageType"`
	Content       string  `json:"content"`
	SourceFactIDs []int64 `json:"sourceFactIds"`
}

func (s *Service) UpdateFromSummary(ctx context.Context, chat model.Chat, summary model.Summary) (model.LLMWikiRun, error) {
	if s == nil || s.store == nil || s.store.LLMWiki == nil {
		return model.LLMWikiRun{}, fmt.Errorf("llm wiki service is not configured")
	}
	now := time.Now()
	run, err := s.store.LLMWiki.CreateRun(ctx, model.LLMWikiRun{
		ChatID:    chat.ID,
		SummaryID: summary.ID,
		Status:    model.LLMWikiRunStatusRunning,
		StartedAt: now,
	})
	if err != nil {
		return model.LLMWikiRun{}, err
	}
	finish := func(status model.LLMWikiRunStatus, updated int, message string) (model.LLMWikiRun, error) {
		return s.store.LLMWiki.FinishRun(ctx, run.ID, status, updated, message, time.Now())
	}

	if summary.Status != model.SummaryStatusSucceeded || strings.TrimSpace(summary.Content) == "" {
		return finish(model.LLMWikiRunStatusSucceeded, 0, "")
	}
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}
	if err := s.workspace.Ensure(); err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}

	facts, err := s.summaryFacts(ctx, chat.ID, now)
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}
	candidates, err := s.candidatePages(ctx)
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}

	client := openai.New(openai.Config{
		BaseURL: settings.OpenAIBaseURL,
		APIKey:  settings.OpenAIAPIKey,
		Model:   settings.OpenAIModel,
		Timeout: s.openAITimeout(),
		Stream:  settings.OpenAIStreamEnabled(),
	})
	resp, _, err := openai.ChatWithRetry(ctx, client, openai.ChatRequest{
		SystemPrompt: buildWikiUpdateSystemPrompt(settings.Language),
		UserPrompt:   buildWikiUpdateUserPrompt(chat, summary, facts, candidates),
		Temperature:  0.1,
		MaxOutput:    wikiUpdateMaxOutput,
	}, openai.RetryConfig{Attempts: 2})
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}

	parsed, err := parseWikiUpdateResponse(resp.Content)
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, 0, err.Error())
		return finished, firstError(err, finishErr)
	}
	updated, err := s.applyWikiUpdateResponse(parsed)
	if err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, updated, err.Error())
		return finished, firstError(err, finishErr)
	}
	if _, err := s.Reindex(ctx); err != nil {
		finished, finishErr := finish(model.LLMWikiRunStatusFailed, updated, err.Error())
		return finished, firstError(err, finishErr)
	}
	return finish(model.LLMWikiRunStatusSucceeded, updated, "")
}

func (s *Service) openAITimeout() time.Duration {
	if s.openAIRequestTimeout > 0 {
		return s.openAIRequestTimeout
	}
	return 3 * time.Minute
}

func (s *Service) summaryFacts(ctx context.Context, chatID int64, now time.Time) ([]model.KnowledgeFact, error) {
	if s.store.KnowledgeFacts == nil {
		return nil, nil
	}
	if err := s.store.KnowledgeFacts.ExpireDue(ctx, now); err != nil {
		return nil, err
	}
	return s.store.KnowledgeFacts.ListForSummaryWithFilter(ctx, store.KnowledgeSummaryFilter{
		ChatID: chatID,
		Now:    now,
	})
}

func (s *Service) candidatePages(ctx context.Context) ([]model.LLMWikiPage, error) {
	if s.store.LLMWiki == nil {
		return nil, nil
	}
	result, err := s.store.LLMWiki.SearchPages(ctx, store.LLMWikiPageFilter{
		PageSize: wikiUpdateCandidateLimit,
	})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func buildWikiUpdateSystemPrompt(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`
You maintain TGTLDR's LLM Wiki, a Markdown workspace used as long-term semantic memory for future AI summarization and Q&A.

Output ONLY valid JSON in this exact shape:
{"updates":[{"path":"spaces/general/topics/example.md","title":"Example","type":"topic|person|project|entity|daily|index|log|page","sourceFactIds":[1,2],"content":"full markdown page"}],"logEntry":"short changelog line"}

Rules:
- Update only semantic memory. Do not copy every fact verbatim.
- Prefer updating existing candidate pages over creating duplicates.
- Create new pages only when the summary introduces a durable person, project, entity, or topic.
- Every important claim should cite fact IDs like fact:#123 or message refs if available.
- Content must be complete Markdown for the target page.
- Keep pages concise and stable. Use clear headings.
- If no durable memory should change, output {"updates":[],"logEntry":""}.
`)
	}
	return strings.TrimSpace(`
你维护 TGTLDR 的 LLM Wiki。它是给后续 AI 摘要和问答使用的长期语义记忆 Markdown 工作区。

只输出合法 JSON，格式必须是：
{"updates":[{"path":"spaces/general/topics/example.md","title":"Example","type":"topic|person|project|entity|daily|index|log|page","sourceFactIds":[1,2],"content":"完整 markdown 页面"}],"logEntry":"简短变更日志"}

规则：
- 只维护长期语义记忆，不要逐条复制所有事实。
- 优先更新候选页面，避免创建重复页面。
- 只有当摘要引入长期有效的人物、项目、实体或主题时才新建页面。
- 关键判断应引用 fact:#123 这样的事实 ID；有消息来源时也可以保留消息引用。
- content 必须是目标页面的完整 Markdown。
- 页面要简洁、结构稳定，使用清晰标题。
- 如果没有值得长期保存的变化，输出 {"updates":[],"logEntry":""}。
`)
}

func buildWikiUpdateUserPrompt(chat model.Chat, summary model.Summary, facts []model.KnowledgeFact, candidates []model.LLMWikiPage) string {
	return strings.TrimSpace(fmt.Sprintf(`
Chat:
- id: %d
- title: %s

Summary:
- date: %s
- generated_at: %s

Summary content:
%s

Structured active facts:
%s

Existing candidate wiki pages:
%s
`,
		chat.ID,
		emptyPlaceholder(chat.Title),
		summary.SummaryDate,
		summary.GeneratedAt.Format(time.RFC3339),
		truncateRunes(summary.Content, wikiUpdateSummaryMaxRunes),
		formatWikiUpdateFacts(facts),
		formatWikiUpdateCandidatePages(candidates),
	))
}

func formatWikiUpdateFacts(facts []model.KnowledgeFact) string {
	if len(facts) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(facts))
	for _, fact := range facts {
		parts := []string{fmt.Sprintf("fact:#%d", fact.ID)}
		if fact.SpaceName != "" {
			parts = append(parts, "space="+fact.SpaceName)
		}
		if fact.FactType != "" {
			parts = append(parts, "type="+fact.FactType)
		}
		if fact.Title != "" {
			parts = append(parts, "title="+compactText(fact.Title))
		}
		if fact.SubjectUsername != "" {
			parts = append(parts, "subject=@"+strings.TrimPrefix(fact.SubjectUsername, "@"))
		} else if fact.SubjectSenderName != "" {
			parts = append(parts, "subject="+compactText(fact.SubjectSenderName))
		}
		if !fact.LastSeenAt.IsZero() {
			parts = append(parts, "last_seen="+fact.LastSeenAt.Format(time.RFC3339))
		}
		lines = append(lines, "- "+strings.Join(parts, "; "))
	}
	return truncateRunes(strings.Join(lines, "\n"), wikiUpdateFactEvidenceMaxRunes)
}

func formatWikiUpdateCandidatePages(pages []model.LLMWikiPage) string {
	if len(pages) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(pages)*4)
	remaining := wikiUpdatePageEvidenceMaxRunes
	for _, page := range pages {
		content := truncateRunes(page.ContentText, minInt(1600, remaining))
		lines = append(lines, fmt.Sprintf("Path: %s\nTitle: %s\nType: %s\nContent:\n%s", page.Path, page.Title, page.PageType, content))
		remaining -= len([]rune(content))
		if remaining <= 0 {
			break
		}
	}
	return strings.Join(lines, "\n\n---\n\n")
}

func parseWikiUpdateResponse(content string) (updateResponse, error) {
	cleaned := strings.TrimSpace(content)
	if match := wikiUpdateCodeFencePattern.FindStringSubmatch(cleaned); len(match) == 2 {
		cleaned = strings.TrimSpace(match[1])
	}
	var parsed updateResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return updateResponse{}, fmt.Errorf("parse llm wiki update response: %w", err)
	}
	if len(parsed.Updates) > wikiUpdateMaxUpdates {
		parsed.Updates = parsed.Updates[:wikiUpdateMaxUpdates]
	}
	return parsed, nil
}

func (s *Service) applyWikiUpdateResponse(parsed updateResponse) (int, error) {
	updated := 0
	for _, update := range parsed.Updates {
		normalized, err := normalizePageUpdate(update)
		if err != nil {
			return updated, err
		}
		if err := s.workspace.WritePage(normalized.Path, normalized.Content); err != nil {
			return updated, err
		}
		updated++
	}
	if entry := strings.TrimSpace(parsed.LogEntry); entry != "" {
		if err := s.appendLogEntry(entry); err != nil {
			return updated, err
		}
	}
	return updated, nil
}

func normalizePageUpdate(update pageUpdate) (pageUpdate, error) {
	update.Path = filepath.ToSlash(strings.TrimSpace(update.Path))
	update.Title = strings.TrimSpace(update.Title)
	update.Type = strings.TrimSpace(firstNonEmpty(update.Type, update.PageType, "page"))
	update.Content = strings.TrimSpace(update.Content)
	if update.Path == "" {
		return pageUpdate{}, fmt.Errorf("llm wiki update path is required")
	}
	if !strings.EqualFold(filepath.Ext(update.Path), ".md") {
		return pageUpdate{}, fmt.Errorf("llm wiki update path must end with .md: %s", update.Path)
	}
	if update.Content == "" {
		return pageUpdate{}, fmt.Errorf("llm wiki update content is required: %s", update.Path)
	}
	if len([]rune(update.Content)) > wikiUpdatePageMaxRunes {
		update.Content = truncateRunes(update.Content, wikiUpdatePageMaxRunes)
	}
	if !strings.HasPrefix(update.Content, "---\n") {
		update.Content = buildPageFrontmatter(update) + "\n" + update.Content
	}
	if !strings.HasSuffix(update.Content, "\n") {
		update.Content += "\n"
	}
	return update, nil
}

func buildPageFrontmatter(update pageUpdate) string {
	lines := []string{"---"}
	if update.Type != "" {
		lines = append(lines, "type: "+update.Type)
	}
	if update.Title != "" {
		lines = append(lines, "title: "+update.Title)
	}
	if len(update.SourceFactIDs) > 0 {
		values := make([]string, 0, len(update.SourceFactIDs))
		for _, id := range update.SourceFactIDs {
			if id > 0 {
				values = append(values, fmt.Sprintf("%d", id))
			}
		}
		if len(values) > 0 {
			lines = append(lines, "source_fact_ids: ["+strings.Join(values, ", ")+"]")
		}
	}
	lines = append(lines, "---")
	return strings.Join(lines, "\n")
}

func (s *Service) appendLogEntry(entry string) error {
	current, err := s.workspace.ReadPage("log.md")
	if err != nil {
		current = "---\ntype: log\ntitle: LLM Wiki Log\n---\n\n# LLM Wiki Log\n\n"
	}
	line := fmt.Sprintf("- %s %s\n", time.Now().UTC().Format(time.RFC3339), compactText(entry))
	return s.workspace.WritePage("log.md", strings.TrimRight(current, "\n")+"\n"+line)
}

func firstError(primary error, secondary error) error {
	if primary != nil {
		return primary
	}
	return secondary
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emptyPlaceholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(empty)"
	}
	return strings.TrimSpace(value)
}

func compactText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
