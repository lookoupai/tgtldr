export type AuthStep = "idle" | "code" | "password" | "done";
export type Language = "zh-CN" | "en";
export type SummaryOutputLanguage = "zh-CN" | "en" | "ru" | "ar" | string;
export type OpenAIRequestMode = "stream" | "non_stream";

export type AppSettings = {
  id: number;
  telegramApiId: number;
  telegramApiHash?: string;
  openAIBaseUrl: string;
  openAIApiKey?: string;
  openAIModel: string;
  openAIRequestMode: OpenAIRequestMode;
  openAITemperature: number;
  openAIOutputMode: "auto" | "manual";
  openAIMaxOutputTokens: number;
  summaryParallelism: number;
  summaryRetryLimit: number;
  summaryRetryBackoffBaseMinutes: number;
  summaryRetryBackoffMultiplier: number;
  defaultTimezone: string;
  language: Language;
  summaryOutputLanguage: SummaryOutputLanguage;
  botEnabled: boolean;
  botToken?: string;
  botTargetChatId: string;
  botIgnoreMessagesFromBots: boolean;
  botPrivateAllowedUsers: string[];
  botBlockedUsers: string[];
};

export type PendingAuth = {
  step: AuthStep;
  phoneNumber: string;
  deadline: string;
};

export type TelegramAuth = {
  id: number;
  phoneNumber: string;
  telegramUserId: number;
  telegramName: string;
  telegramHandle: string;
  status: string;
  lastConnectedAt: string;
};

export type Bootstrap = {
  settingsConfigured: boolean;
  passwordConfigured: boolean;
  authenticated: boolean;
  telegramAuthorized: boolean;
  enabledChatCount: number;
  botEnabled: boolean;
  language: Language;
  settings: AppSettings;
  auth?: TelegramAuth;
  pendingAuth?: PendingAuth;
};

export type AuthStatus = {
  status: string;
};

export type BotTargetChatCandidate = {
  chatId: string;
  chatType: string;
  title?: string;
  username?: string;
  fromUserId?: number;
  fromUsername?: string;
};

export type BotTargetChatResolveResult = {
  candidates: BotTargetChatCandidate[];
};

export type OpenAITestResult = {
  ok: boolean;
  model: string;
};

export type BotCommand = {
  command: string;
  description: string;
};

export type BotStatus = {
  enabled: boolean;
  tokenConfigured: boolean;
  targetChatId: string;
  botId?: number;
  username?: string;
  commandsSynced: boolean;
  commands?: BotCommand[];
  expectedCommands: BotCommand[];
  error?: string;
  runtime?: BotRuntimeState;
  lastPollAt?: string | null;
  lastUpdateAt?: string | null;
  lastHandledAt?: string | null;
  lastError?: string;
  runtimeBotUsername?: string;
};

export type BotRuntimeState = {
  id: number;
  botUsername: string;
  lastPollAt?: string | null;
  lastUpdateAt?: string | null;
  lastHandledAt?: string | null;
  lastError: string;
  updatedAt: string;
};

export type DeliveryMode = "dashboard" | "bot";
export type SummaryMode = "channel" | "chat_topic";

export type TopicGroup = {
  name: string;
  description: string;
};

export type Chat = {
  id: number;
  telegramChatId: number;
  title: string;
  username: string;
  chatType: string;
  enabled: boolean;
  summaryEnabled: boolean;
  summaryContext: string;
  summaryPrompt: string;
  summaryMode: SummaryMode;
  summaryLanguage: SummaryOutputLanguage;
  topicGroups: TopicGroup[];
  summaryTimeLocal: string;
  summaryKnowledgeDays: number;
  deliveryMode: DeliveryMode;
  modelOverride: string;
  keepBotMessages: boolean;
  filteredSenders: string[];
  filteredKeywords: string[];
  botChatId: string;
  botInteractionEnabled: boolean;
  botAllowedUsers: string[];
  botBlockedUsers: string[];
};

export type HistoryBackfillTask = {
  id: string;
  chatId: number;
  chatTitle: string;
  fromDate: string;
  toDate: string;
  status: "pending" | "running" | "succeeded" | "failed";
  importedCount: number;
  errorMessage: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
};

export type Summary = {
  id: number;
  chatId: number;
  summaryDate: string;
  status: "pending" | "running" | "succeeded" | "failed";
  content: string;
  model: string;
  sourceMessageCount: number;
  chunkCount: number;
  generatedAt: string;
  deliveredAt?: string;
  deliveryError: string;
  errorMessage: string;
  errorContext: string;
  errorSystemPrompt: string;
  errorUserPrompt: string;
  retryCount: number;
  nextRetryAt?: string;
  matchSnippet?: string;
  matchedFields?: string[];
};

export type SummarySearchFilters = {
  q?: string;
  chatId?: string;
  status?: "all" | Summary["status"];
  delivery?: "all" | "sent" | "pending" | "failed" | "disabled";
  dateFrom?: string;
  dateTo?: string;
  page?: number;
  pageSize?: number;
};

export type SummaryListResponse = {
  items: Summary[];
  total: number;
  page: number;
  pageSize: number;
};

export type SummaryStats = {
  total: number;
  successCount: number;
  processingCount: number;
  failedCount: number;
};

export type SummaryContextChunk = {
  index: number;
  messageCount: number;
  content: string;
};

export type SummaryContextPreview = {
  summaryId: number;
  chatId: number;
  summaryDate: string;
  model: string;
  systemPrompt: string;
  finalPrompt: string;
  messageCount: number;
  chunkCount: number;
  chunks: SummaryContextChunk[];
  finalInputNotice: string;
  previewNotice: string;
};

export type KnowledgeSpace = {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  chatIds: number[];
  schemaJson: string;
  extractPrompt: string;
  summaryPrompt: string;
  confidenceThreshold: number;
  retentionDays: number;
  includeInSummary: boolean;
  createdAt: string;
  updatedAt: string;
};

export type KnowledgeFact = {
  id: number;
  spaceId: number;
  spaceName?: string;
  chatId: number;
  chatTitle?: string;
  factType: string;
  title: string;
  dataJson: string;
  subjectSenderId: number;
  subjectSenderName: string;
  subjectUsername: string;
  confidence: number;
  status: "active" | "expired" | "dismissed";
  sourceMessageIds: number[];
  firstSeenAt: string;
  lastSeenAt: string;
  expiresAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type Message = {
  id: number;
  chatId: number;
  telegramMessageId: number;
  telegramSenderId: number;
  senderName: string;
  senderUsername: string;
  senderIsBot: boolean;
  textContent: string;
  caption: string;
  messageType: string;
  mediaKind: string;
  replyToMessageId: number;
  messageTime: string;
  rawJson: string;
  createdAt: string;
};

export type KnowledgeFactSources = {
  fact: KnowledgeFact;
  messages: Message[];
};

export type KnowledgeSubject = {
  key: string;
  displayName: string;
  subjectSenderId: number;
  subjectSenderName: string;
  subjectUsername: string;
  factCount: number;
  factTypes: string[];
  chatTitles: string[];
  lastSeenAt: string;
  facts: KnowledgeFact[];
};

export type KnowledgeQueryResult = {
  query: string;
  factType: string;
  facts: KnowledgeFact[];
  subjects: KnowledgeSubject[];
  content: string;
  message?: string;
};

export type KnowledgeMaintenanceResult = {
  action: string;
  targetType: string;
  targetQuery: string;
  targetUser: string;
  replacement?: string;
  reason: string;
  matchedFacts: KnowledgeFact[];
  updatedFacts: KnowledgeFact[];
};

export type KnowledgeRun = {
  id: number;
  spaceId: number;
  chatId: number;
  rangeStart: string;
  rangeEnd: string;
  status: "pending" | "running" | "succeeded" | "failed";
  inputMessageCount: number;
  extractedCount: number;
  errorMessage: string;
  startedAt: string;
  finishedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type LLMWikiMessageRef = {
  chatId: number;
  telegramMessageId: number;
};

export type LLMWikiPage = {
  id: number;
  spaceId: number;
  path: string;
  title: string;
  pageType: string;
  contentHash: string;
  contentText?: string;
  sourceFactIds: number[];
  sourceMessageRefs: LLMWikiMessageRef[];
  createdAt: string;
  updatedAt: string;
  indexedAt: string;
};

export type LLMWikiPageListResponse = {
  items: LLMWikiPage[];
  total: number;
  page: number;
  pageSize: number;
};

export type LLMWikiSearchFilters = {
  q?: string;
  spaceId?: number | "all";
  type?: string | "all";
  page?: number;
  pageSize?: number;
};

export type LLMWikiReindexResult = {
  root: string;
  pageCount: number;
};

export type LLMWikiRun = {
  id: number;
  spaceId: number;
  chatId: number;
  summaryId: number;
  rangeStart?: string;
  rangeEnd?: string;
  status: "pending" | "running" | "succeeded" | "failed";
  updatedPageCount: number;
  errorMessage: string;
  startedAt: string;
  finishedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type LLMWikiRunListResponse = {
  items: LLMWikiRun[];
};

export type KnowledgeMaintenanceEvent = {
  id: number;
  factId: number;
  factTitle: string;
  spaceId: number;
  spaceName: string;
  chatId: number;
  chatTitle: string;
  action: "expire" | "dismiss" | "restore" | string;
  source: "auto_status_update" | "bot_command" | "bot_update" | "web" | string;
  reason: string;
  operatorText: string;
  matchedQuery: string;
  previousStatus: KnowledgeFact["status"] | "";
  nextStatus: KnowledgeFact["status"] | "";
  createdAt: string;
};

export type DeliveryChannel = {
  id: number;
  name: string;
  enabled: boolean;
  sourceChatIds: number[];
  targetChatId: string;
  targetLanguage: SummaryOutputLanguage;
  contentFilter: string;
  contentFilterTypes: string[];
  summaryTimeLocal: string;
  summaryTimezone: string;
  summaryPrompt: string;
  summaryKnowledgeDays: number;
  createdAt: string;
  updatedAt: string;
};
