package model

import (
	"strings"
	"time"
)

type DeliveryMode string
type SummaryMode string

const (
	DeliveryModeDashboard DeliveryMode = "dashboard"
	DeliveryModeBot       DeliveryMode = "bot"
	SummaryModeChannel    SummaryMode  = "channel"
	SummaryModeChatTopic  SummaryMode  = "chat_topic"
)

type SummaryStatus string

const (
	SummaryStatusPending   SummaryStatus = "pending"
	SummaryStatusRunning   SummaryStatus = "running"
	SummaryStatusSucceeded SummaryStatus = "succeeded"
	SummaryStatusFailed    SummaryStatus = "failed"
)

type OutputMode string
type Language string
type SummaryOutputLanguage string

const (
	OutputModeAuto       OutputMode            = "auto"
	OutputModeManual     OutputMode            = "manual"
	LanguageZhCN         Language              = "zh-CN"
	LanguageEN           Language              = "en"
	SummaryLanguageZhCN  SummaryOutputLanguage = "zh-CN"
	SummaryLanguageEN    SummaryOutputLanguage = "en"
	SummaryLanguageRU    SummaryOutputLanguage = "ru"
	SummaryLanguageAR    SummaryOutputLanguage = "ar"
	DefaultOpenAIBaseURL                       = "https://api.openai.com/v1"
)

func NormalizeLanguage(language Language) Language {
	if language == LanguageEN {
		return LanguageEN
	}
	return LanguageZhCN
}

func NormalizeSummaryOutputLanguage(language SummaryOutputLanguage) SummaryOutputLanguage {
	switch strings.TrimSpace(string(language)) {
	case string(SummaryLanguageEN):
		return SummaryLanguageEN
	case string(SummaryLanguageRU):
		return SummaryLanguageRU
	case string(SummaryLanguageAR):
		return SummaryLanguageAR
	case "", string(SummaryLanguageZhCN):
		return SummaryLanguageZhCN
	default:
		return SummaryOutputLanguage(strings.TrimSpace(string(language)))
	}
}

func NormalizeOptionalSummaryOutputLanguage(language SummaryOutputLanguage) SummaryOutputLanguage {
	trimmed := strings.TrimSpace(string(language))
	if trimmed == "" {
		return ""
	}
	return NormalizeSummaryOutputLanguage(SummaryOutputLanguage(trimmed))
}

func ResolveSummaryOutputLanguage(settings AppSettings, chat Chat) SummaryOutputLanguage {
	if language := NormalizeOptionalSummaryOutputLanguage(chat.SummaryLanguage); language != "" {
		return language
	}
	return NormalizeSummaryOutputLanguage(settings.SummaryOutputLanguage)
}

func NormalizeSummaryMode(mode SummaryMode) SummaryMode {
	if mode == SummaryModeChatTopic {
		return SummaryModeChatTopic
	}
	return SummaryModeChannel
}

type AppSettings struct {
	ID                    int64                 `json:"id"`
	TelegramAPIID         int                   `json:"telegramApiId"`
	TelegramAPIHash       string                `json:"telegramApiHash,omitempty"`
	OpenAIBaseURL         string                `json:"openAIBaseUrl"`
	OpenAIAPIKey          string                `json:"openAIApiKey,omitempty"`
	OpenAIModel           string                `json:"openAIModel"`
	OpenAITemperature     float64               `json:"openAITemperature"`
	OpenAIOutputMode      OutputMode            `json:"openAIOutputMode"`
	OpenAIMaxOutputToken  int                   `json:"openAIMaxOutputTokens"`
	SummaryParallelism    int                   `json:"summaryParallelism"`
	DefaultTimezone       string                `json:"defaultTimezone"`
	Language              Language              `json:"language"`
	SummaryOutputLanguage SummaryOutputLanguage `json:"summaryOutputLanguage"`
	BotEnabled            bool                  `json:"botEnabled"`
	BotToken              string                `json:"botToken,omitempty"`
	BotTargetChatID       string                `json:"botTargetChatId"`
	CreatedAt             time.Time             `json:"createdAt"`
	UpdatedAt             time.Time             `json:"updatedAt"`
}

func (s AppSettings) Sanitized() AppSettings {
	s.TelegramAPIHash = redactSecret(s.TelegramAPIHash)
	s.OpenAIAPIKey = redactSecret(s.OpenAIAPIKey)
	s.BotToken = redactSecret(s.BotToken)
	return s
}

type LocalAuth struct {
	PasswordHash      string    `json:"-"`
	PasswordUpdatedAt time.Time `json:"passwordUpdatedAt"`
	SessionVersion    int       `json:"sessionVersion"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type LocalSession struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"sessionId"`
	SessionVersion int       `json:"sessionVersion"`
	ExpiresAt      time.Time `json:"expiresAt"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type TelegramAuth struct {
	ID              int64     `json:"id"`
	PhoneNumber     string    `json:"phoneNumber"`
	TelegramUserID  int64     `json:"telegramUserId"`
	TelegramName    string    `json:"telegramName"`
	TelegramHandle  string    `json:"telegramHandle"`
	SessionData     []byte    `json:"-"`
	Status          string    `json:"status"`
	LastConnectedAt time.Time `json:"lastConnectedAt"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Chat struct {
	ID               int64                 `json:"id"`
	TelegramChatID   int64                 `json:"telegramChatId"`
	TelegramAccess   int64                 `json:"telegramAccessHash"`
	Title            string                `json:"title"`
	Username         string                `json:"username"`
	ChatType         string                `json:"chatType"`
	Enabled          bool                  `json:"enabled"`
	SummaryEnabled   bool                  `json:"summaryEnabled"`
	SummaryContext   string                `json:"summaryContext"`
	SummaryPrompt    string                `json:"summaryPrompt"`
	SummaryMode      SummaryMode           `json:"summaryMode"`
	SummaryLanguage  SummaryOutputLanguage `json:"summaryLanguage"`
	TopicGroups      []TopicGroup          `json:"topicGroups"`
	SummaryTimeLocal string                `json:"summaryTimeLocal"`
	SummaryTimezone  string                `json:"summaryTimezone"`
	DeliveryMode     DeliveryMode          `json:"deliveryMode"`
	ModelOverride    string                `json:"modelOverride"`
	KeepBotMessages  bool                  `json:"keepBotMessages"`
	FilteredSenders  []string              `json:"filteredSenders"`
	FilteredKeywords []string              `json:"filteredKeywords"`
	BotChatID        string                `json:"botChatId"`
	BotInteraction   bool                  `json:"botInteractionEnabled"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

type TopicGroup struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type KnowledgeSpace struct {
	ID                  int64     `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	Enabled             bool      `json:"enabled"`
	ChatIDs             []int64   `json:"chatIds"`
	SchemaJSON          string    `json:"schemaJson"`
	ExtractPrompt       string    `json:"extractPrompt"`
	SummaryPrompt       string    `json:"summaryPrompt"`
	ConfidenceThreshold float64   `json:"confidenceThreshold"`
	RetentionDays       int       `json:"retentionDays"`
	IncludeInSummary    bool      `json:"includeInSummary"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type KnowledgeFactStatus string

const (
	KnowledgeFactStatusActive    KnowledgeFactStatus = "active"
	KnowledgeFactStatusExpired   KnowledgeFactStatus = "expired"
	KnowledgeFactStatusDismissed KnowledgeFactStatus = "dismissed"
)

type KnowledgeFact struct {
	ID                int64               `json:"id"`
	SpaceID           int64               `json:"spaceId"`
	SpaceName         string              `json:"spaceName,omitempty"`
	ChatID            int64               `json:"chatId"`
	ChatTitle         string              `json:"chatTitle,omitempty"`
	FactType          string              `json:"factType"`
	Title             string              `json:"title"`
	DataJSON          string              `json:"dataJson"`
	SubjectSenderID   int64               `json:"subjectSenderId"`
	SubjectSenderName string              `json:"subjectSenderName"`
	SubjectUsername   string              `json:"subjectUsername"`
	Confidence        float64             `json:"confidence"`
	Status            KnowledgeFactStatus `json:"status"`
	SourceMessageIDs  []int               `json:"sourceMessageIds"`
	FirstSeenAt       time.Time           `json:"firstSeenAt"`
	LastSeenAt        time.Time           `json:"lastSeenAt"`
	ExpiresAt         *time.Time          `json:"expiresAt,omitempty"`
	CreatedAt         time.Time           `json:"createdAt"`
	UpdatedAt         time.Time           `json:"updatedAt"`
}

type KnowledgeSubject struct {
	Key               string          `json:"key"`
	DisplayName       string          `json:"displayName"`
	SubjectSenderID   int64           `json:"subjectSenderId"`
	SubjectSenderName string          `json:"subjectSenderName"`
	SubjectUsername   string          `json:"subjectUsername"`
	FactCount         int             `json:"factCount"`
	FactTypes         []string        `json:"factTypes"`
	ChatTitles        []string        `json:"chatTitles"`
	LastSeenAt        time.Time       `json:"lastSeenAt"`
	Facts             []KnowledgeFact `json:"facts"`
}

type KnowledgeRunStatus string

const (
	KnowledgeRunStatusPending   KnowledgeRunStatus = "pending"
	KnowledgeRunStatusRunning   KnowledgeRunStatus = "running"
	KnowledgeRunStatusSucceeded KnowledgeRunStatus = "succeeded"
	KnowledgeRunStatusFailed    KnowledgeRunStatus = "failed"
)

type KnowledgeRun struct {
	ID                int64              `json:"id"`
	SpaceID           int64              `json:"spaceId"`
	ChatID            int64              `json:"chatId"`
	RangeStart        time.Time          `json:"rangeStart"`
	RangeEnd          time.Time          `json:"rangeEnd"`
	Status            KnowledgeRunStatus `json:"status"`
	InputMessageCount int                `json:"inputMessageCount"`
	ExtractedCount    int                `json:"extractedCount"`
	ErrorMessage      string             `json:"errorMessage"`
	StartedAt         time.Time          `json:"startedAt"`
	FinishedAt        *time.Time         `json:"finishedAt,omitempty"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

type KnowledgeMaintenanceEvent struct {
	ID             int64               `json:"id"`
	FactID         int64               `json:"factId"`
	FactTitle      string              `json:"factTitle"`
	SpaceID        int64               `json:"spaceId"`
	SpaceName      string              `json:"spaceName"`
	ChatID         int64               `json:"chatId"`
	ChatTitle      string              `json:"chatTitle"`
	Action         string              `json:"action"`
	Source         string              `json:"source"`
	Reason         string              `json:"reason"`
	OperatorText   string              `json:"operatorText"`
	MatchedQuery   string              `json:"matchedQuery"`
	PreviousStatus KnowledgeFactStatus `json:"previousStatus"`
	NextStatus     KnowledgeFactStatus `json:"nextStatus"`
	CreatedAt      time.Time           `json:"createdAt"`
}

type Message struct {
	ID                int64     `json:"id"`
	ChatID            int64     `json:"chatId"`
	TelegramMessageID int       `json:"telegramMessageId"`
	TelegramSenderID  int64     `json:"telegramSenderId"`
	SenderName        string    `json:"senderName"`
	SenderUsername    string    `json:"senderUsername"`
	SenderIsBot       bool      `json:"senderIsBot"`
	TextContent       string    `json:"textContent"`
	Caption           string    `json:"caption"`
	MessageType       string    `json:"messageType"`
	MediaKind         string    `json:"mediaKind"`
	ReplyToMessageID  int       `json:"replyToMessageId"`
	MessageTime       time.Time `json:"messageTime"`
	RawJSON           string    `json:"rawJson"`
	CreatedAt         time.Time `json:"createdAt"`
}

func (m Message) SummaryText() string {
	text := m.TextContent
	if text == "" {
		text = m.Caption
	}
	return text
}

type Summary struct {
	ID                 int64         `json:"id"`
	ChatID             int64         `json:"chatId"`
	SummaryDate        string        `json:"summaryDate"`
	Status             SummaryStatus `json:"status"`
	Content            string        `json:"content"`
	Model              string        `json:"model"`
	SourceMessageCount int           `json:"sourceMessageCount"`
	ChunkCount         int           `json:"chunkCount"`
	GeneratedAt        time.Time     `json:"generatedAt"`
	DeliveredAt        *time.Time    `json:"deliveredAt,omitempty"`
	DeliveryError      string        `json:"deliveryError"`
	ErrorMessage       string        `json:"errorMessage"`
	MatchSnippet       string        `json:"matchSnippet,omitempty"`
	MatchedFields      []string      `json:"matchedFields,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type SummaryListResponse struct {
	Items    []Summary `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}

type SummaryStats struct {
	Total           int `json:"total"`
	SuccessCount    int `json:"successCount"`
	ProcessingCount int `json:"processingCount"`
	FailedCount     int `json:"failedCount"`
}

type SummaryContextPreview struct {
	SummaryID        int64                 `json:"summaryId"`
	ChatID           int64                 `json:"chatId"`
	SummaryDate      string                `json:"summaryDate"`
	Model            string                `json:"model"`
	SystemPrompt     string                `json:"systemPrompt"`
	FinalPrompt      string                `json:"finalPrompt"`
	MessageCount     int                   `json:"messageCount"`
	ChunkCount       int                   `json:"chunkCount"`
	Chunks           []SummaryContextChunk `json:"chunks"`
	FinalInputNotice string                `json:"finalInputNotice"`
	PreviewNotice    string                `json:"previewNotice"`
}

type SummaryContextChunk struct {
	Index        int    `json:"index"`
	MessageCount int    `json:"messageCount"`
	Content      string `json:"content"`
}

type HistoryBackfillStatus string

const (
	HistoryBackfillStatusPending   HistoryBackfillStatus = "pending"
	HistoryBackfillStatusRunning   HistoryBackfillStatus = "running"
	HistoryBackfillStatusSucceeded HistoryBackfillStatus = "succeeded"
	HistoryBackfillStatusFailed    HistoryBackfillStatus = "failed"
)

type HistoryBackfillTask struct {
	ID            string                `json:"id"`
	ChatID        int64                 `json:"chatId"`
	ChatTitle     string                `json:"chatTitle"`
	FromDate      string                `json:"fromDate"`
	ToDate        string                `json:"toDate"`
	Status        HistoryBackfillStatus `json:"status"`
	ImportedCount int                   `json:"importedCount"`
	ErrorMessage  string                `json:"errorMessage"`
	CreatedAt     time.Time             `json:"createdAt"`
	UpdatedAt     time.Time             `json:"updatedAt"`
	CompletedAt   *time.Time            `json:"completedAt,omitempty"`
}

type Bootstrap struct {
	SettingsConfigured bool          `json:"settingsConfigured"`
	PasswordConfigured bool          `json:"passwordConfigured"`
	Authenticated      bool          `json:"authenticated"`
	TelegramAuthorized bool          `json:"telegramAuthorized"`
	EnabledChatCount   int           `json:"enabledChatCount"`
	BotEnabled         bool          `json:"botEnabled"`
	Settings           AppSettings   `json:"settings"`
	Auth               *TelegramAuth `json:"auth,omitempty"`
}

type AuthStep string

const (
	AuthStepIdle     AuthStep = "idle"
	AuthStepCode     AuthStep = "code"
	AuthStepPassword AuthStep = "password"
	AuthStepDone     AuthStep = "done"
)

type AuthSessionState struct {
	Step        AuthStep  `json:"step"`
	PhoneNumber string    `json:"phoneNumber"`
	CodeHash    string    `json:"-"`
	Deadline    time.Time `json:"deadline"`
}

func redactSecret(secret string) string {
	if len(secret) <= 4 {
		return ""
	}
	return secret[:2] + "****" + secret[len(secret)-2:]
}
