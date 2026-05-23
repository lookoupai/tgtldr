package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/bot"
	"github.com/frederic/tgtldr/app/internal/botquery"
	"github.com/frederic/tgtldr/app/internal/httpx"
	"github.com/frederic/tgtldr/app/internal/knowledge"
	"github.com/frederic/tgtldr/app/internal/localauth"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/scheduler"
	"github.com/frederic/tgtldr/app/internal/store"
	telegramsvc "github.com/frederic/tgtldr/app/internal/telegram"
)

type Router struct {
	store     *store.Store
	bot       *bot.Service
	knowledge *knowledge.Service
	telegram  *telegramsvc.Service
	scheduler *scheduler.Service
	auth      *localauth.Service
	origin    string
	timeout   time.Duration
}

func New(
	store *store.Store,
	telegram *telegramsvc.Service,
	scheduler *scheduler.Service,
	botService *bot.Service,
	knowledgeService *knowledge.Service,
	origin string,
	timeout time.Duration,
) *Router {
	return &Router{
		store:     store,
		bot:       botService,
		knowledge: knowledgeService,
		telegram:  telegram,
		scheduler: scheduler,
		auth:      localauth.NewService(store),
		origin:    origin,
		timeout:   timeout,
	}
}

func (r *Router) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", r.handleHealth)
	mux.HandleFunc("/api/bootstrap", r.handleBootstrap)
	mux.HandleFunc("/api/auth/login", r.handleLogin)
	mux.HandleFunc("/api/auth/logout", r.handleLogout)
	mux.HandleFunc("/api/auth/setup-password", r.handleSetupPassword)
	mux.HandleFunc("/api/auth/change-password", r.handleChangePassword)
	mux.HandleFunc("/api/settings", r.handleSettings)
	mux.HandleFunc("/api/openai/test", r.handleOpenAITest)
	mux.HandleFunc("/api/bot/status", r.handleBotStatus)
	mux.HandleFunc("/api/bot/target-chat/resolve", r.handleResolveBotTargetChat)
	mux.HandleFunc("/api/telegram/auth/start", r.handleStartAuth)
	mux.HandleFunc("/api/telegram/auth/code", r.handleVerifyCode)
	mux.HandleFunc("/api/telegram/auth/password", r.handleVerifyPassword)
	mux.HandleFunc("/api/telegram/chats/sync", r.handleSyncChats)
	mux.HandleFunc("/api/chats", r.handleChats)
	mux.HandleFunc("/api/chats/", r.handleChatByID)
	mux.HandleFunc("/api/knowledge/query", r.handleKnowledgeQuery)
	mux.HandleFunc("/api/knowledge/query/natural", r.handleNaturalKnowledgeQuery)
	mux.HandleFunc("/api/knowledge/query/send", r.handleSendKnowledgeQuery)
	mux.HandleFunc("/api/knowledge/maintenance/preview", r.handlePreviewKnowledgeMaintenance)
	mux.HandleFunc("/api/knowledge/maintenance/apply", r.handleApplyKnowledgeMaintenance)
	mux.HandleFunc("/api/knowledge/maintenance-events", r.handleKnowledgeMaintenanceEvents)
	mux.HandleFunc("/api/knowledge/runs", r.handleKnowledgeRuns)
	mux.HandleFunc("/api/knowledge/subjects", r.handleKnowledgeSubjects)
	mux.HandleFunc("/api/knowledge/spaces", r.handleKnowledgeSpaces)
	mux.HandleFunc("/api/knowledge/spaces/", r.handleKnowledgeSpaceByID)
	mux.HandleFunc("/api/knowledge/facts", r.handleKnowledgeFacts)
	mux.HandleFunc("/api/knowledge/facts/", r.handleKnowledgeFactByID)
	mux.HandleFunc("/api/history-backfills", r.handleStartHistoryBackfill)
	mux.HandleFunc("/api/history-backfills/", r.handleHistoryBackfillByID)
	mux.HandleFunc("/api/summaries", r.handleSummaries)
	mux.HandleFunc("/api/summaries/stats", r.handleSummaryStats)
	mux.HandleFunc("/api/summaries/", r.handleSummaryByID)
	mux.HandleFunc("/api/summaries/context-preview", r.handleSummaryContextPreview)
	mux.HandleFunc("/api/summaries/run", r.handleRunSummary)
	mux.HandleFunc("/api/channels", r.handleDeliveryChannels)
	mux.HandleFunc("/api/channels/create", r.handleCreateDeliveryChannel)
	mux.HandleFunc("/api/channels/", r.handleDeliveryChannelByID)

	return r.withMiddleware(mux)
}

func (r *Router) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if origin := allowedOrigin(req.Header.Get("Origin"), r.origin); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), r.timeout)
		defer cancel()
		authorizedReq, ok := r.authorizeRequest(w, req.WithContext(ctx))
		if !ok {
			return
		}
		next.ServeHTTP(w, authorizedReq)
	})
}

func allowedOrigin(requestOrigin, configuredOrigin string) string {
	switch requestOrigin {
	case "", configuredOrigin, "http://localhost:3000", "http://127.0.0.1:3000":
		if requestOrigin != "" {
			return requestOrigin
		}
		return configuredOrigin
	default:
		return ""
	}
}

func preservedSecret(incoming string, current string) string {
	trimmed := strings.TrimSpace(incoming)
	if trimmed == "" {
		return current
	}

	sanitized := redactSecret(current)
	if sanitized != "" && trimmed == sanitized {
		return current
	}
	return incoming
}

func redactSecret(secret string) string {
	if len(secret) <= 4 {
		return ""
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}

func settingsConfigured(settings model.AppSettings) bool {
	return settings.TelegramAPIID != 0 &&
		strings.TrimSpace(settings.TelegramAPIHash) != "" &&
		strings.TrimSpace(settings.OpenAIAPIKey) != "" &&
		strings.TrimSpace(settings.OpenAIModel) != ""
}

func (r *Router) currentLanguage(ctx context.Context) model.Language {
	settings, err := r.store.Settings.Get(ctx)
	if err != nil {
		return model.LanguageZhCN
	}
	return model.NormalizeLanguage(settings.Language)
}

func (r *Router) localized(ctx context.Context, zh string, en string) string {
	if r.currentLanguage(ctx) == model.LanguageEN {
		return en
	}
	return zh
}

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) handleBootstrap(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	passwordConfigured, err := r.auth.PasswordConfigured(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	language := r.currentLanguage(req.Context())
	authenticated := currentSessionID(req.Context()) != ""
	if !authenticated {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"settingsConfigured": false,
			"passwordConfigured": passwordConfigured,
			"authenticated":      false,
			"telegramAuthorized": false,
			"enabledChatCount":   0,
			"botEnabled":         false,
			"language":           language,
		})
		return
	}

	settings, err := r.store.Settings.Get(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	auth, err := r.telegram.BootstrapAuth(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	count, err := r.store.Chats.CountEnabled(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload := map[string]any{
		"settingsConfigured": settingsConfigured(settings),
		"passwordConfigured": passwordConfigured,
		"authenticated":      authenticated,
		"telegramAuthorized": auth != nil && auth.Status == "authorized",
		"enabledChatCount":   count,
		"botEnabled":         settings.BotEnabled,
		"language":           model.NormalizeLanguage(settings.Language),
		"settings":           settings.Sanitized(),
		"auth":               auth,
		"pendingAuth":        r.telegram.PendingAuthState(),
	}
	httpx.JSON(w, http.StatusOK, payload)
}

func (r *Router) handleSettings(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut && req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if req.Method == http.MethodGet {
		settings, err := r.store.Settings.Get(req.Context())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, settings.Sanitized())
		return
	}

	var payload model.AppSettings
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	current, err := r.store.Settings.Get(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	wasConfigured := settingsConfigured(current)
	payload.TelegramAPIHash = preservedSecret(payload.TelegramAPIHash, current.TelegramAPIHash)
	payload.OpenAIAPIKey = preservedSecret(payload.OpenAIAPIKey, current.OpenAIAPIKey)
	payload.BotToken = preservedSecret(payload.BotToken, current.BotToken)
	if payload.TelegramAPIID == 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写 Telegram API ID。", "Enter Telegram API ID."))
		return
	}
	if strings.TrimSpace(payload.TelegramAPIHash) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写 Telegram API Hash。", "Enter Telegram API Hash."))
		return
	}
	if strings.TrimSpace(payload.OpenAIAPIKey) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写 OpenAI API Key。", "Enter OpenAI API Key."))
		return
	}
	if strings.TrimSpace(payload.OpenAIBaseURL) == "" {
		payload.OpenAIBaseURL = model.DefaultOpenAIBaseURL
	}
	if strings.TrimSpace(payload.OpenAIModel) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写 Model。", "Enter Model."))
		return
	}
	if payload.OpenAIRequestMode == "" {
		payload.OpenAIRequestMode = model.OpenAIRequestModeStream
	}
	if payload.OpenAIRequestMode != model.OpenAIRequestModeStream && payload.OpenAIRequestMode != model.OpenAIRequestModeNonStream {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "调用方式必须是 stream 或 non_stream。", "Request mode must be stream or non_stream."))
		return
	}
	if payload.OpenAITemperature < 0 || payload.OpenAITemperature > 2 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "Temperature 必须在 0.0 到 2.0 之间。", "Temperature must be between 0.0 and 2.0."))
		return
	}
	if strings.TrimSpace(payload.DefaultTimezone) == "" {
		payload.DefaultTimezone = "Asia/Shanghai"
	}
	if payload.Language == "" {
		payload.Language = model.LanguageZhCN
	}
	if payload.Language != model.LanguageZhCN && payload.Language != model.LanguageEN {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "语言必须是 zh-CN 或 en。", "Language must be zh-CN or en."))
		return
	}
	payload.SummaryOutputLanguage = model.NormalizeSummaryOutputLanguage(payload.SummaryOutputLanguage)
	if payload.OpenAIOutputMode == "" {
		payload.OpenAIOutputMode = model.OutputModeAuto
	}
	if payload.OpenAIOutputMode != model.OutputModeAuto && payload.OpenAIOutputMode != model.OutputModeManual {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "输出长度模式必须是 auto 或 manual。", "Output length mode must be auto or manual."))
		return
	}
	if payload.OpenAIOutputMode == model.OutputModeManual && payload.OpenAIMaxOutputToken <= 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "自定义输出长度时必须填写 Max Output Tokens。", "Max Output Tokens is required when output length is custom."))
		return
	}
	if payload.SummaryParallelism <= 0 {
		payload.SummaryParallelism = 2
	}
	if payload.SummaryParallelism < 1 || payload.SummaryParallelism > 6 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "摘要并行度必须在 1 到 6 之间。", "Summary parallelism must be between 1 and 6."))
		return
	}
	if payload.SummaryRetryLimit < 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "摘要重试次数不能小于 0。", "Summary retry limit cannot be less than 0."))
		return
	}
	if payload.SummaryRetryBackoffBaseMinutes <= 0 {
		payload.SummaryRetryBackoffBaseMinutes = model.DefaultSummaryRetryBackoffBaseMinutes
	}
	if payload.SummaryRetryBackoffMultiplier <= 0 {
		payload.SummaryRetryBackoffMultiplier = model.DefaultSummaryRetryBackoffMultiplier
	}
	if payload.SummaryRetryBackoffMultiplier < 1 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "摘要重试倍率不能小于 1。", "Summary retry multiplier cannot be less than 1."))
		return
	}
	if payload.BotEnabled && strings.TrimSpace(payload.BotToken) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "启用 Bot 推送时必须填写 Bot Token。", "Bot Token is required when Bot delivery is enabled."))
		return
	}

	saved, err := r.store.Settings.Save(req.Context(), payload)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := botquery.SyncBotCommands(req.Context(), r.bot, saved); err != nil {
		fmt.Printf("sync bot commands: %v\n", err)
	}
	if !wasConfigured && settingsConfigured(saved) && r.knowledge != nil {
		if _, _, err := r.knowledge.EnsureDefaultGeneralSpace(req.Context()); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	httpx.JSON(w, http.StatusOK, saved.Sanitized())
}

func (r *Router) handleResolveBotTargetChat(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload struct {
		BotToken string `json:"botToken"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	botToken := strings.TrimSpace(payload.BotToken)
	if botToken == "" {
		settings, err := r.store.Settings.Get(req.Context())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		botToken = strings.TrimSpace(settings.BotToken)
	}
	if botToken == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请先填写 Bot Token。", "Enter the Bot Token first."))
		return
	}

	self, err := r.bot.GetMe(req.Context(), botToken)
	if err != nil {
		status := http.StatusBadGateway
		var botErr *bot.APIError
		if errors.As(err, &botErr) && botErr.StatusCode >= 400 && botErr.StatusCode < 500 {
			status = http.StatusBadRequest
		}
		httpx.Error(w, status, err.Error())
		return
	}

	candidates := make([]bot.TargetChatCandidate, 0)
	if r.store.BotTargetChats != nil {
		stored, err := r.store.BotTargetChats.ListByBot(req.Context(), self.ID, 20)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		candidates = modelBotTargetCandidates(stored)
	}

	liveCandidates, err := r.bot.ResolveTargetChats(req.Context(), botToken, 0)
	if err != nil && len(candidates) == 0 {
		status := http.StatusBadGateway
		var botErr *bot.APIError
		if errors.As(err, &botErr) && botErr.StatusCode >= 400 && botErr.StatusCode < 500 {
			status = http.StatusBadRequest
		}
		httpx.Error(w, status, err.Error())
		return
	}
	candidates = mergeBotTargetChatCandidates(candidates, liveCandidates)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"candidates": candidates,
	})
}

func modelBotTargetCandidates(items []model.BotTargetChatCandidate) []bot.TargetChatCandidate {
	out := make([]bot.TargetChatCandidate, 0, len(items))
	for _, item := range items {
		out = append(out, bot.TargetChatCandidate{
			ChatID:       strings.TrimSpace(item.ChatID),
			ChatType:     strings.TrimSpace(item.ChatType),
			Title:        strings.TrimSpace(item.Title),
			Username:     strings.TrimSpace(item.Username),
			FromUserID:   item.FromUserID,
			FromUsername: strings.TrimSpace(item.FromUsername),
		})
	}
	return out
}

func mergeBotTargetChatCandidates(primary []bot.TargetChatCandidate, secondary []bot.TargetChatCandidate) []bot.TargetChatCandidate {
	out := make([]bot.TargetChatCandidate, 0, len(primary)+len(secondary))
	seen := make(map[string]bool)
	for _, candidate := range append(primary, secondary...) {
		chatID := strings.TrimSpace(candidate.ChatID)
		if chatID == "" || seen[chatID] {
			continue
		}
		seen[chatID] = true
		candidate.ChatID = chatID
		out = append(out, candidate)
	}
	return out
}

func (r *Router) handleBotStatus(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	settings, err := r.store.Settings.Get(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if r.bot == nil {
		httpx.Error(w, http.StatusInternalServerError, "bot service is not configured")
		return
	}
	if req.Method == http.MethodPost {
		if err := botquery.SyncBotCommands(req.Context(), r.bot, settings); err != nil {
			httpx.Error(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	status := map[string]any{
		"enabled":          settings.BotEnabled,
		"tokenConfigured":  strings.TrimSpace(settings.BotToken) != "",
		"targetChatId":     strings.TrimSpace(settings.BotTargetChatID),
		"commandsSynced":   false,
		"expectedCommands": botquery.BotCommands(settings.Language),
	}
	if r.store.BotRuntime != nil {
		runtimeState, err := r.store.BotRuntime.Get(req.Context())
		if err == nil {
			status["runtime"] = runtimeState
			status["lastPollAt"] = runtimeState.LastPollAt
			status["lastUpdateAt"] = runtimeState.LastUpdateAt
			status["lastHandledAt"] = runtimeState.LastHandledAt
			status["lastError"] = runtimeState.LastError
			if runtimeState.BotUsername != "" {
				status["runtimeBotUsername"] = runtimeState.BotUsername
			}
		}
	}
	if !settings.BotEnabled || strings.TrimSpace(settings.BotToken) == "" {
		httpx.JSON(w, http.StatusOK, status)
		return
	}

	self, err := r.bot.GetMe(req.Context(), settings.BotToken)
	if err != nil {
		status["error"] = err.Error()
		httpx.JSON(w, http.StatusOK, status)
		return
	}
	status["botId"] = self.ID
	status["username"] = strings.TrimSpace(self.Username)

	commands, err := r.bot.GetMyCommands(req.Context(), settings.BotToken)
	if err != nil {
		status["error"] = err.Error()
		httpx.JSON(w, http.StatusOK, status)
		return
	}
	status["commands"] = commands
	status["commandsSynced"] = botquery.CommandsEqual(commands, botquery.BotCommands(settings.Language))
	httpx.JSON(w, http.StatusOK, status)
}

func (r *Router) handleStartAuth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		PhoneNumber string `json:"phoneNumber"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	state, err := r.telegram.StartAuth(req.Context(), payload.PhoneNumber)
	if err != nil {
		if floodErr, ok := asFloodWaitError(err); ok {
			httpx.ErrorWithCode(w, http.StatusTooManyRequests, floodErr.Error(), "telegram_flood_wait", floodErr.RetryAfterSeconds())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}

func (r *Router) handleVerifyCode(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	state, err := r.telegram.VerifyCode(req.Context(), payload.Code)
	if err != nil {
		if errors.Is(err, telegramsvc.ErrPasswordNeeded) {
			httpx.JSON(w, http.StatusAccepted, state)
			return
		}
		if floodErr, ok := asFloodWaitError(err); ok {
			httpx.ErrorWithCode(w, http.StatusTooManyRequests, floodErr.Error(), "telegram_flood_wait", floodErr.RetryAfterSeconds())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}

func (r *Router) handleVerifyPassword(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	state, err := r.telegram.VerifyPassword(req.Context(), payload.Password)
	if err != nil {
		if floodErr, ok := asFloodWaitError(err); ok {
			httpx.ErrorWithCode(w, http.StatusTooManyRequests, floodErr.Error(), "telegram_flood_wait", floodErr.RetryAfterSeconds())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, state)
}

func (r *Router) handleSyncChats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := r.telegram.SyncChats(req.Context()); err != nil {
		if floodErr, ok := asFloodWaitError(err); ok {
			httpx.ErrorWithCode(w, http.StatusTooManyRequests, floodErr.Error(), "telegram_flood_wait", floodErr.RetryAfterSeconds())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	chats, err := r.store.Chats.List(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, chats)
}

func asFloodWaitError(err error) (*telegramsvc.FloodWaitError, bool) {
	var floodErr *telegramsvc.FloodWaitError
	if !errors.As(err, &floodErr) {
		return nil, false
	}
	return floodErr, true
}

func (r *Router) handleChats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	chats, err := r.store.Chats.List(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, chats)
}

func (r *Router) handleChatByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idValue := strings.TrimPrefix(req.URL.Path, "/api/chats/")
	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}

	current, err := r.store.Chats.GetByID(req.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	var payload struct {
		Enabled              bool                        `json:"enabled"`
		SummaryEnabled       bool                        `json:"summaryEnabled"`
		SummaryContext       string                      `json:"summaryContext"`
		SummaryPrompt        string                      `json:"summaryPrompt"`
		SummaryMode          model.SummaryMode           `json:"summaryMode"`
		SummaryLanguage      model.SummaryOutputLanguage `json:"summaryLanguage"`
		TopicGroups          []model.TopicGroup          `json:"topicGroups"`
		SummaryTimeLocal     string                      `json:"summaryTimeLocal"`
		SummaryKnowledgeDays int                         `json:"summaryKnowledgeDays"`
		DeliveryMode         model.DeliveryMode          `json:"deliveryMode"`
		ModelOverride        string                      `json:"modelOverride"`
		KeepBotMessages      bool                        `json:"keepBotMessages"`
		FilteredSenders      []string                    `json:"filteredSenders"`
		FilteredKeywords     []string                    `json:"filteredKeywords"`
		BotChatID            string                      `json:"botChatId"`
		BotInteraction       bool                        `json:"botInteractionEnabled"`
		BotAllowedUsers      []string                    `json:"botAllowedUsers"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	current.Enabled = payload.Enabled
	current.SummaryEnabled = payload.SummaryEnabled
	current.SummaryContext = payload.SummaryContext
	current.SummaryPrompt = payload.SummaryPrompt
	current.SummaryMode = model.NormalizeSummaryMode(payload.SummaryMode)
	current.SummaryLanguage = model.NormalizeOptionalSummaryOutputLanguage(payload.SummaryLanguage)
	current.TopicGroups = compactTopicGroups(payload.TopicGroups)
	current.SummaryTimeLocal = payload.SummaryTimeLocal
	current.SummaryKnowledgeDays = payload.SummaryKnowledgeDays
	current.DeliveryMode = payload.DeliveryMode
	current.ModelOverride = payload.ModelOverride
	current.KeepBotMessages = payload.KeepBotMessages
	current.FilteredSenders = compactStrings(payload.FilteredSenders)
	current.FilteredKeywords = compactStrings(payload.FilteredKeywords)
	current.BotChatID = strings.TrimSpace(payload.BotChatID)
	current.BotInteraction = payload.BotInteraction
	current.BotAllowedUsers = compactStrings(payload.BotAllowedUsers)

	saved, err := r.store.Chats.Save(req.Context(), current)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, saved)
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func compactTopicGroups(values []model.TopicGroup) []model.TopicGroup {
	out := make([]model.TopicGroup, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		out = append(out, model.TopicGroup{
			Name:        name,
			Description: strings.TrimSpace(value.Description),
		})
	}
	return out
}

func (r *Router) handleSummaries(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	params := store.SummaryListParams{
		Query:    strings.TrimSpace(req.URL.Query().Get("q")),
		Status:   strings.TrimSpace(req.URL.Query().Get("status")),
		Delivery: strings.TrimSpace(req.URL.Query().Get("delivery")),
		DateFrom: strings.TrimSpace(req.URL.Query().Get("dateFrom")),
		DateTo:   strings.TrimSpace(req.URL.Query().Get("dateTo")),
	}

	if chatIDValue := strings.TrimSpace(req.URL.Query().Get("chatId")); chatIDValue != "" {
		chatID, err := strconv.ParseInt(chatIDValue, 10, 64)
		if err != nil || chatID < 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid chatId")
			return
		}
		params.ChatID = chatID
	}

	if pageValue := strings.TrimSpace(req.URL.Query().Get("page")); pageValue != "" {
		page, err := strconv.Atoi(pageValue)
		if err != nil || page < 1 {
			httpx.Error(w, http.StatusBadRequest, "invalid page")
			return
		}
		params.Page = page
	}

	if pageSizeValue := strings.TrimSpace(req.URL.Query().Get("pageSize")); pageSizeValue != "" {
		pageSize, err := strconv.Atoi(pageSizeValue)
		if err != nil || pageSize < 1 {
			httpx.Error(w, http.StatusBadRequest, "invalid pageSize")
			return
		}
		params.PageSize = pageSize
	}

	summaries, err := r.store.Summaries.Search(req.Context(), params)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, summaries)
}

func (r *Router) handleSummaryStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats, err := r.store.Summaries.Stats(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, stats)
}

func (r *Router) handleSummaryContextPreview(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idValue := strings.TrimSpace(req.URL.Query().Get("id"))
	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id == 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid summary id")
		return
	}

	item, err := r.store.Summaries.GetByID(req.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	preview, err := r.scheduler.ContextPreview(req.Context(), item)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, preview)
}

func (r *Router) handleRunSummary(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		ChatID int64  `json:"chatId"`
		Date   string `json:"date"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	chat, err := r.store.Chats.GetByID(req.Context(), payload.ChatID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	started, err := r.scheduler.RunNowAsync(req.Context(), chat, payload.Date)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	message := fmt.Sprintf("summary queued for chat %d on %s", payload.ChatID, payload.Date)
	if !started {
		message = fmt.Sprintf("summary already running for chat %d on %s", payload.ChatID, payload.Date)
	}

	httpx.JSON(w, http.StatusAccepted, map[string]string{
		"message": message,
	})
}

func (r *Router) handleSummaryByID(w http.ResponseWriter, req *http.Request) {
	trimmed := strings.TrimPrefix(req.URL.Path, "/api/summaries/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 || parts[1] != "retry-delivery" {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	summaryID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || summaryID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid summary id")
		return
	}

	if err := r.scheduler.RetryDelivery(req.Context(), summaryID); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("delivery retried for summary %d", summaryID),
	})
}
