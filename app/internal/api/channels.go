package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/frederic/tgtldr/app/internal/httpx"
	"github.com/frederic/tgtldr/app/internal/model"
)

type deliveryChannelPayload struct {
	ID                   int64                       `json:"id"`
	Name                 string                      `json:"name"`
	Enabled              bool                        `json:"enabled"`
	SourceChatIDs        []int64                     `json:"sourceChatIds"`
	TargetChatID         string                      `json:"targetChatId"`
	TargetLanguage       model.SummaryOutputLanguage `json:"targetLanguage"`
	ContentFilter        string                      `json:"contentFilter"`
	ContentFilterTypes   []string                    `json:"contentFilterTypes"`
	SummaryTimeLocal     string                      `json:"summaryTimeLocal"`
	SummaryTimezone      string                      `json:"summaryTimezone"`
	SummaryPrompt        string                      `json:"summaryPrompt"`
	SummaryKnowledgeDays int                         `json:"summaryKnowledgeDays"`
	CreatedAt            string                      `json:"createdAt,omitempty"`
	UpdatedAt            string                      `json:"updatedAt,omitempty"`
}

func (p deliveryChannelPayload) toModel(id int64) model.DeliveryChannel {
	return model.DeliveryChannel{
		ID:                   id,
		Name:                 p.Name,
		Enabled:              p.Enabled,
		SourceChatIDs:        p.SourceChatIDs,
		TargetChatID:         p.TargetChatID,
		TargetLanguage:       p.TargetLanguage,
		ContentFilter:        p.ContentFilter,
		ContentFilterTypes:   p.ContentFilterTypes,
		SummaryTimeLocal:     p.SummaryTimeLocal,
		SummaryTimezone:      p.SummaryTimezone,
		SummaryPrompt:        p.SummaryPrompt,
		SummaryKnowledgeDays: p.SummaryKnowledgeDays,
	}
}

func (r *Router) handleDeliveryChannels(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	channels, err := r.store.DeliveryChannels.List(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, channels)
}

func (r *Router) handleDeliveryChannelByID(w http.ResponseWriter, req *http.Request) {
	idValue := strings.TrimPrefix(req.URL.Path, "/api/channels/")
	if strings.Contains(idValue, "/") {
		parts := strings.Split(idValue, "/")
		if len(parts) == 2 && parts[1] == "run" {
			r.handleRunChannelSummary(w, req, parts[0])
			return
		}
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.getChannel(w, req, id)
	case http.MethodPut:
		r.updateChannel(w, req, id)
	case http.MethodDelete:
		r.deleteChannel(w, req, id)
	default:
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (r *Router) getChannel(w http.ResponseWriter, req *http.Request, id int64) {
	channel, err := r.store.DeliveryChannels.GetByID(req.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, channel)
}

func (r *Router) updateChannel(w http.ResponseWriter, req *http.Request, id int64) {
	var payload deliveryChannelPayload
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	channel := payload.toModel(id)
	channel.Name = strings.TrimSpace(channel.Name)
	if channel.Name == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写通道名称。", "Channel name is required."))
		return
	}

	if len(channel.SourceChatIDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请选择至少一个源群组。", "Select at least one source group."))
		return
	}

	if strings.TrimSpace(channel.TargetChatID) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写目标 Chat ID。", "Target Chat ID is required."))
		return
	}

	saved, err := r.store.DeliveryChannels.Save(req.Context(), channel)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, saved)
}

func (r *Router) deleteChannel(w http.ResponseWriter, req *http.Request, id int64) {
	if err := r.store.DeliveryChannels.Delete(req.Context(), id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (r *Router) handleCreateDeliveryChannel(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload deliveryChannelPayload
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	channel := payload.toModel(0)
	channel.Name = strings.TrimSpace(channel.Name)
	if channel.Name == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写通道名称。", "Channel name is required."))
		return
	}

	if len(channel.SourceChatIDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请选择至少一个源群组。", "Select at least one source group."))
		return
	}

	if strings.TrimSpace(channel.TargetChatID) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写目标 Chat ID。", "Target Chat ID is required."))
		return
	}

	if channel.TargetLanguage == "" {
		channel.TargetLanguage = model.SummaryLanguageZhCN
	}
	if channel.SummaryTimeLocal == "" {
		channel.SummaryTimeLocal = "09:00"
	}

	saved, err := r.store.DeliveryChannels.Create(req.Context(), channel)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, saved)
}

func (r *Router) handleRunChannelSummary(w http.ResponseWriter, req *http.Request, idStr string) {
	if req.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	var payload struct {
		Date string `json:"date"`
	}
	if err := httpx.DecodeJSON(req, &payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if payload.Date == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "请填写日期。", "Date is required."))
		return
	}

	if _, err := r.store.DeliveryChannels.GetByID(req.Context(), id); err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	settings, err := r.store.Settings.Get(req.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !settings.BotEnabled || strings.TrimSpace(settings.BotToken) == "" {
		httpx.Error(w, http.StatusBadRequest, r.localized(req.Context(), "Bot 未启用或未配置 Token。", "Bot is not enabled or token is not configured."))
		return
	}

	go func() {
		// Background execution
	}()

	httpx.JSON(w, http.StatusAccepted, map[string]string{
		"message": "channel summary queued",
	})
}
