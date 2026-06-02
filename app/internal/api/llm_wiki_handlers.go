package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/frederic/tgtldr/app/internal/httpx"
	"github.com/frederic/tgtldr/app/internal/store"
)

func (r *Router) handleLLMWikiPages(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.llmWiki == nil {
		httpx.Error(w, http.StatusInternalServerError, "llm wiki service is not configured")
		return
	}

	query := req.URL.Query()
	filter := store.LLMWikiPageFilter{
		Query:    strings.TrimSpace(query.Get("q")),
		PageType: strings.TrimSpace(query.Get("type")),
	}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if page, err := strconv.Atoi(strings.TrimSpace(query.Get("page"))); err == nil {
		filter.Page = page
	}
	if pageSize, err := strconv.Atoi(strings.TrimSpace(query.Get("pageSize"))); err == nil {
		filter.PageSize = pageSize
	}

	result, err := r.llmWiki.SearchPages(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (r *Router) handleLLMWikiPageByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.llmWiki == nil {
		httpx.Error(w, http.StatusInternalServerError, "llm wiki service is not configured")
		return
	}

	idValue := strings.TrimPrefix(req.URL.Path, "/api/llm-wiki/pages/")
	id, err := strconv.ParseInt(strings.TrimSpace(idValue), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid llm wiki page id")
		return
	}

	page, err := r.llmWiki.GetPageByID(req.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (r *Router) handleLLMWikiRuns(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.llmWiki == nil {
		httpx.Error(w, http.StatusInternalServerError, "llm wiki service is not configured")
		return
	}

	query := req.URL.Query()
	filter := store.LLMWikiRunFilter{}
	if spaceID, err := strconv.ParseInt(strings.TrimSpace(query.Get("spaceId")), 10, 64); err == nil {
		filter.SpaceID = spaceID
	}
	if chatID, err := strconv.ParseInt(strings.TrimSpace(query.Get("chatId")), 10, 64); err == nil {
		filter.ChatID = chatID
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit"))); err == nil {
		filter.Limit = limit
	}

	result, err := r.llmWiki.ListRuns(req.Context(), filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (r *Router) handleLLMWikiReindex(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.llmWiki == nil {
		httpx.Error(w, http.StatusInternalServerError, "llm wiki service is not configured")
		return
	}

	result, err := r.llmWiki.Reindex(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}
