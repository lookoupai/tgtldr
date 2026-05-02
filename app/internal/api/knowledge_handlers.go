package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/httpx"
	"github.com/frederic/tgtldr/app/internal/knowledge"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
)

func (r *Router) handleKnowledgeSpaces(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		items, err := r.store.KnowledgeSpaces.List(req.Context())
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, items)
	case http.MethodPost:
		var payload model.KnowledgeSpace
		if err := httpx.DecodeJSON(req, &payload); err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(payload.Name) == "" {
			httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写知识空间名称。", "Enter knowledge space name."))
			return
		}
		saved, err := r.store.KnowledgeSpaces.Create(req.Context(), payload)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusCreated, saved)
	default:
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (r *Router) handleKnowledgeSpaceByID(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/knowledge/spaces/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && parts[1] == "run" {
		r.handleKnowledgeRun(w, req, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if req.Method != http.MethodGet && req.Method != http.MethodPut {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid knowledge space id")
		return
	}

	current, err := r.store.KnowledgeSpaces.GetByID(req.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if store.IsNotFound(err) {
			status = http.StatusNotFound
		}
		httpx.Error(w, status, err.Error())
		return
	}
	if req.Method == http.MethodGet {
		httpx.JSON(w, http.StatusOK, current)
		return
	}

	var payload model.KnowledgeSpace
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(payload.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写知识空间名称。", "Enter knowledge space name."))
		return
	}
	payload.ID = current.ID
	saved, err := r.store.KnowledgeSpaces.Save(req.Context(), payload)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, saved)
}

func (r *Router) handleKnowledgeRun(w http.ResponseWriter, req *http.Request, idValue string) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid knowledge space id")
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
	if payload.ChatID <= 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请选择要抽取的群组。", "Choose a chat to extract."))
		return
	}
	if strings.TrimSpace(payload.Date) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请选择抽取日期。", "Choose extraction date."))
		return
	}
	run, err := r.knowledge.RunDailyExtraction(req.Context(), knowledge.RunRequest{
		SpaceID: id,
		ChatID:  payload.ChatID,
		Date:    strings.TrimSpace(payload.Date),
	})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, run)
}

func (r *Router) handleKnowledgeFacts(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := req.URL.Query()
	filter := store.KnowledgeFactFilter{
		Status:   model.KnowledgeFactStatus(strings.TrimSpace(query.Get("status"))),
		FactType: knowledgeFactTypeParam(query.Get("type"), query.Get("factType")),
		Query:    strings.TrimSpace(query.Get("q")),
	}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		filter.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		filter.Limit = limit
	}

	if err := r.store.KnowledgeFacts.ExpireDue(req.Context(), time.Now()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := r.store.KnowledgeFacts.List(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (r *Router) handleKnowledgeRuns(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := req.URL.Query()
	filter := store.KnowledgeRunFilter{}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		filter.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		filter.Limit = limit
	}

	items, err := r.store.KnowledgeRuns.List(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (r *Router) handleKnowledgeMaintenanceEvents(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := req.URL.Query()
	filter := store.KnowledgeMaintenanceEventFilter{}
	if factID, err := strconv.ParseInt(strings.TrimSpace(query.Get("factId")), 10, 64); err == nil {
		filter.FactID = factID
	}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		filter.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		filter.Limit = limit
	}

	items, err := r.store.KnowledgeMaintenanceEvents.List(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (r *Router) handleKnowledgeSubjects(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := req.URL.Query()
	filter := store.KnowledgeSubjectFilter{
		FactType: knowledgeFactTypeParam(query.Get("type"), query.Get("factType")),
		Query:    strings.TrimSpace(query.Get("q")),
	}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		filter.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		filter.Limit = limit
	}

	if err := r.store.KnowledgeFacts.ExpireDue(req.Context(), time.Now()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := r.store.KnowledgeFacts.ListSubjects(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (r *Router) handleKnowledgeQuery(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := r.buildKnowledgeQueryResult(req, knowledgeQueryRequestFromQuery(req))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, result)
}

func (r *Router) handleSendKnowledgeQuery(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload knowledgeQueryRequest
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := r.buildKnowledgeQueryResult(req, payload)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	settings, err := r.store.Settings.Get(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !settings.BotEnabled {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "Bot 推送未启用。", "Bot delivery is disabled."))
		return
	}
	if strings.TrimSpace(settings.BotToken) == "" || strings.TrimSpace(settings.BotTargetChatID) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "Bot Token 或目标 Chat ID 未配置。", "Bot token or target chat id is not configured."))
		return
	}
	if r.bot == nil {
		httpx.Error(w, http.StatusInternalServerError, "bot service is not configured")
		return
	}

	if err := r.bot.SendMessageWithLanguage(req.Context(), settings.BotToken, settings.BotTargetChatID, result.Content, r.currentLanguage(req.Context())); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"message":  r.localized(req.Context(), "知识查询结果已发送。", "Knowledge query result sent."),
		"query":    result.Query,
		"factType": result.FactType,
		"facts":    result.Facts,
		"subjects": result.Subjects,
		"content":  result.Content,
	})
}

type knowledgeQueryRequest struct {
	Query          string `json:"q"`
	FactType       string `json:"type"`
	LegacyFactType string `json:"factType"`
	SpaceID        int64  `json:"spaceId"`
	ChatID         int64  `json:"chatId"`
	Limit          int    `json:"limit"`
}

type knowledgeQueryResult struct {
	Query    string                   `json:"query"`
	FactType string                   `json:"factType"`
	Facts    []model.KnowledgeFact    `json:"facts"`
	Subjects []model.KnowledgeSubject `json:"subjects"`
	Content  string                   `json:"content"`
}

func knowledgeQueryRequestFromQuery(req *http.Request) knowledgeQueryRequest {
	query := req.URL.Query()
	payload := knowledgeQueryRequest{
		Query:    strings.TrimSpace(query.Get("q")),
		FactType: knowledgeFactTypeParam(query.Get("type"), query.Get("factType")),
	}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		payload.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		payload.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		payload.Limit = limit
	}
	return payload
}

func (r *Router) buildKnowledgeQueryResult(req *http.Request, payload knowledgeQueryRequest) (knowledgeQueryResult, error) {
	limit := normalizeKnowledgeQueryLimit(payload.Limit)
	filter := store.KnowledgeFactFilter{
		SpaceID:  payload.SpaceID,
		ChatID:   payload.ChatID,
		Status:   model.KnowledgeFactStatusActive,
		FactType: knowledgeFactTypeParam(payload.FactType, payload.LegacyFactType),
		Query:    strings.TrimSpace(payload.Query),
		Limit:    limit,
	}

	if err := r.store.KnowledgeFacts.ExpireDue(req.Context(), time.Now()); err != nil {
		return knowledgeQueryResult{}, err
	}
	facts, err := r.store.KnowledgeFacts.List(req.Context(), filter)
	if err != nil {
		return knowledgeQueryResult{}, err
	}
	subjects, err := r.store.KnowledgeFacts.ListSubjects(req.Context(), store.KnowledgeSubjectFilter{
		SpaceID:  filter.SpaceID,
		ChatID:   filter.ChatID,
		FactType: filter.FactType,
		Query:    filter.Query,
		Limit:    limit,
	})
	if err != nil {
		return knowledgeQueryResult{}, err
	}

	return knowledgeQueryResult{
		Query:    filter.Query,
		FactType: filter.FactType,
		Facts:    facts,
		Subjects: subjects,
		Content:  knowledge.FormatQueryResult(r.currentLanguage(req.Context()), filter.Query, filter.FactType, facts, subjects),
	}, nil
}

func normalizeKnowledgeQueryLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (r *Router) handleKnowledgeFactByID(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/knowledge/facts/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "status" {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid knowledge fact id")
		return
	}

	var payload struct {
		Status model.KnowledgeFactStatus `json:"status"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	status := normalizeKnowledgeFactStatusForUpdate(payload.Status)
	if status == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "状态只能设置为 active 或 dismissed。", "Status can only be set to active or dismissed."))
		return
	}

	var item model.KnowledgeFact
	if r.knowledge != nil {
		item, err = r.knowledge.UpdateFactStatus(req.Context(), id, status, knowledge.MaintenanceSourceWeb, "", "", "")
	} else {
		item, err = r.store.KnowledgeFacts.UpdateStatus(req.Context(), id, status)
	}
	if err != nil {
		statusCode := http.StatusInternalServerError
		if store.IsNotFound(err) {
			statusCode = http.StatusNotFound
		}
		httpx.Error(w, statusCode, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func normalizeKnowledgeFactStatusForUpdate(status model.KnowledgeFactStatus) model.KnowledgeFactStatus {
	switch status {
	case model.KnowledgeFactStatusActive, model.KnowledgeFactStatusDismissed:
		return status
	default:
		return ""
	}
}

func knowledgeFactTypeParam(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
