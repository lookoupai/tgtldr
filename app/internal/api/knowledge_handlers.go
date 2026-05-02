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
		Status: model.KnowledgeFactStatus(strings.TrimSpace(query.Get("status"))),
		Query:  strings.TrimSpace(query.Get("q")),
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

func (r *Router) handleKnowledgeSubjects(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := req.URL.Query()
	filter := store.KnowledgeSubjectFilter{
		Query: strings.TrimSpace(query.Get("q")),
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

	item, err := r.store.KnowledgeFacts.UpdateStatus(req.Context(), id, status)
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
