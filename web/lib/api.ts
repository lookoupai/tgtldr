import {
  AppSettings,
  AuthStatus,
  Bootstrap,
  BotStatus,
  BotTargetChatResolveResult,
  Chat,
  DeliveryChannel,
  HistoryBackfillTask,
  KnowledgeFact,
  KnowledgeFactSources,
  KnowledgeMaintenanceEvent,
  KnowledgeMaintenanceResult,
  KnowledgeQueryResult,
  KnowledgeRun,
  KnowledgeSpace,
  KnowledgeSubject,
  OpenAITestResult,
  SummaryListResponse,
  SummarySearchFilters,
  Summary,
  SummaryStats,
  SummaryContextPreview,
} from "@/lib/types";

type ErrorPayload = {
  error?: string;
  code?: string;
  retryAfterSeconds?: number;
};

export class APIError extends Error {
  status: number;
  code?: string;
  retryAfterSeconds?: number;

  constructor(message: string, status: number, payload?: ErrorPayload) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = payload?.code;
    this.retryAfterSeconds = payload?.retryAfterSeconds;
  }
}

function normalizeList<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

type RequestOptions = RequestInit & {
  skipUnauthorizedRedirect?: boolean;
};

async function request<T>(path: string, init?: RequestOptions): Promise<T> {
  const response = await fetch(`${resolveAPIBaseURL()}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    cache: "no-store",
  });

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    let payload: ErrorPayload | undefined;
    try {
      payload = (await response.json()) as ErrorPayload;
      if (typeof payload.error === "string") {
        message = payload.error;
      }
    } catch {
      // ignore
    }
    if (
      response.status === 401 &&
      typeof window !== "undefined" &&
      !init?.skipUnauthorizedRedirect &&
      !window.location.pathname.startsWith("/login")
    ) {
      window.location.href = "/login";
    }
    throw new APIError(message, response.status, payload);
  }

  return response.json() as Promise<T>;
}

function resolveAPIBaseURL() {
  if (typeof window !== "undefined") {
    return "";
  }
  return "";
}

function buildQuery(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === "") {
      continue;
    }
    search.set(key, String(value));
  }
  const encoded = search.toString();
  if (!encoded) {
    return "";
  }
  return `?${encoded}`;
}

export const api = {
  bootstrap: () => request<Bootstrap>("/api/bootstrap"),
  login: (password: string) =>
    request<AuthStatus>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ password }),
      skipUnauthorizedRedirect: true,
    }),
  logout: () =>
    request<AuthStatus>("/api/auth/logout", {
      method: "POST",
      skipUnauthorizedRedirect: true,
    }),
  setupPassword: (password: string) =>
    request<AuthStatus>("/api/auth/setup-password", {
      method: "POST",
      body: JSON.stringify({ password }),
      skipUnauthorizedRedirect: true,
    }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<AuthStatus>("/api/auth/change-password", {
      method: "POST",
      body: JSON.stringify({ currentPassword, newPassword }),
    }),
  settings: () => request<AppSettings>("/api/settings"),
  saveSettings: (payload: AppSettings) =>
    request<AppSettings>("/api/settings", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  testOpenAI: (payload: AppSettings) =>
    request<OpenAITestResult>("/api/openai/test", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  botStatus: () => request<BotStatus>("/api/bot/status"),
  syncBotCommands: () =>
    request<BotStatus>("/api/bot/status", {
      method: "POST",
      body: JSON.stringify({}),
    }),
  resolveBotTargetChat: (botToken?: string) =>
    request<BotTargetChatResolveResult>("/api/bot/target-chat/resolve", {
      method: "POST",
      body: JSON.stringify({ botToken: botToken?.trim() || "" }),
    }),
  startAuth: (phoneNumber: string) =>
    request("/api/telegram/auth/start", {
      method: "POST",
      body: JSON.stringify({ phoneNumber }),
    }),
  verifyCode: (code: string) =>
    request("/api/telegram/auth/code", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),
  verifyPassword: (password: string) =>
    request("/api/telegram/auth/password", {
      method: "POST",
      body: JSON.stringify({ password }),
    }),
  syncChats: async () =>
    normalizeList(
      await request<Chat[] | null>("/api/telegram/chats/sync", {
        method: "POST",
      }),
    ),
  listChats: async () =>
    normalizeList(await request<Chat[] | null>("/api/chats")),
  saveChat: (chat: Chat) =>
    request<Chat>(`/api/chats/${chat.id}`, {
      method: "PUT",
      body: JSON.stringify({
        enabled: chat.enabled,
        summaryEnabled: chat.summaryEnabled,
        summaryContext: chat.summaryContext,
        summaryPrompt: chat.summaryPrompt,
        summaryMode: chat.summaryMode,
        summaryLanguage: chat.summaryLanguage,
        topicGroups: chat.topicGroups,
        summaryTimeLocal: chat.summaryTimeLocal,
        summaryKnowledgeDays: chat.summaryKnowledgeDays,
        deliveryMode: chat.deliveryMode,
        modelOverride: chat.modelOverride,
        keepBotMessages: chat.keepBotMessages,
        filteredSenders: chat.filteredSenders,
        filteredKeywords: chat.filteredKeywords,
        botChatId: chat.botChatId,
        botInteractionEnabled: chat.botInteractionEnabled,
        botAllowedUsers: chat.botAllowedUsers,
      }),
    }),
  listKnowledgeSpaces: async () =>
    normalizeList(await request<KnowledgeSpace[] | null>("/api/knowledge/spaces")),
  createKnowledgeSpace: (payload: KnowledgeSpace) =>
    request<KnowledgeSpace>("/api/knowledge/spaces", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  saveKnowledgeSpace: (payload: KnowledgeSpace) =>
    request<KnowledgeSpace>(`/api/knowledge/spaces/${payload.id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  listKnowledgeFacts: async (filters?: {
    q?: string;
    spaceId?: number;
    chatId?: number;
    status?: KnowledgeFact["status"] | "all";
    factType?: string;
    limit?: number;
  }) =>
    normalizeList(
      await request<KnowledgeFact[] | null>(
        `/api/knowledge/facts${buildQuery({
          q: filters?.q?.trim() || undefined,
          spaceId: filters?.spaceId,
          chatId: filters?.chatId,
          status: filters?.status && filters.status !== "all" ? filters.status : undefined,
          type: filters?.factType?.trim() || undefined,
          limit: filters?.limit,
        })}`,
      ),
    ),
  createKnowledgeFact: (payload: KnowledgeFact) =>
    request<KnowledgeFact>("/api/knowledge/facts", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  saveKnowledgeFact: (payload: KnowledgeFact) =>
    request<KnowledgeFact>(`/api/knowledge/facts/${payload.id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  listKnowledgeSubjects: async (filters?: {
    q?: string;
    spaceId?: number;
    chatId?: number;
    factType?: string;
    limit?: number;
  }) =>
    normalizeList(
      await request<KnowledgeSubject[] | null>(
        `/api/knowledge/subjects${buildQuery({
          q: filters?.q?.trim() || undefined,
          spaceId: filters?.spaceId,
          chatId: filters?.chatId,
          type: filters?.factType?.trim() || undefined,
          limit: filters?.limit,
        })}`,
      ),
    ),
  renderKnowledgeQuery: (filters?: {
    q?: string;
    spaceId?: number;
    chatId?: number;
    factType?: string;
    limit?: number;
  }) =>
    request<KnowledgeQueryResult>(
      `/api/knowledge/query${buildQuery({
        q: filters?.q?.trim() || undefined,
        spaceId: filters?.spaceId,
        chatId: filters?.chatId,
        type: filters?.factType?.trim() || undefined,
        limit: filters?.limit,
      })}`,
    ),
  renderNaturalKnowledgeQuery: (payload: {
    text: string;
    spaceId?: number;
    chatId?: number;
    limit?: number;
  }) =>
    request<KnowledgeQueryResult>("/api/knowledge/query/natural", {
      method: "POST",
      body: JSON.stringify({
        text: payload.text.trim(),
        spaceId: payload.spaceId,
        chatId: payload.chatId,
        limit: payload.limit,
      }),
    }),
  sendKnowledgeQuery: (filters?: {
    q?: string;
    spaceId?: number;
    chatId?: number;
    factType?: string;
    limit?: number;
  }) =>
    request<KnowledgeQueryResult>("/api/knowledge/query/send", {
      method: "POST",
      body: JSON.stringify({
        q: filters?.q?.trim() || "",
        spaceId: filters?.spaceId,
        chatId: filters?.chatId,
        type: filters?.factType?.trim() || "",
        limit: filters?.limit,
      }),
    }),
  previewKnowledgeMaintenance: (text: string) =>
    request<KnowledgeMaintenanceResult>("/api/knowledge/maintenance/preview", {
      method: "POST",
      body: JSON.stringify({ text: text.trim() }),
    }),
  applyKnowledgeMaintenance: (text: string) =>
    request<KnowledgeMaintenanceResult>("/api/knowledge/maintenance/apply", {
      method: "POST",
      body: JSON.stringify({ text: text.trim() }),
    }),
  listKnowledgeRuns: async (filters?: {
    spaceId?: number;
    chatId?: number;
    limit?: number;
  }) =>
    normalizeList(
      await request<KnowledgeRun[] | null>(
        `/api/knowledge/runs${buildQuery({
          spaceId: filters?.spaceId,
          chatId: filters?.chatId,
          limit: filters?.limit,
        })}`,
      ),
    ),
  listKnowledgeMaintenanceEvents: async (filters?: {
    factId?: number;
    spaceId?: number;
    chatId?: number;
    limit?: number;
  }) =>
    normalizeList(
      await request<KnowledgeMaintenanceEvent[] | null>(
        `/api/knowledge/maintenance-events${buildQuery({
          factId: filters?.factId,
          spaceId: filters?.spaceId,
          chatId: filters?.chatId,
          limit: filters?.limit,
        })}`,
      ),
    ),
  runKnowledgeExtraction: (spaceId: number, chatId: number, date: string) =>
    request<KnowledgeRun>(`/api/knowledge/spaces/${spaceId}/run`, {
      method: "POST",
      body: JSON.stringify({ chatId, date }),
    }),
  updateKnowledgeFactStatus: (factId: number, status: KnowledgeFact["status"]) =>
    request<KnowledgeFact>(`/api/knowledge/facts/${factId}/status`, {
      method: "POST",
      body: JSON.stringify({ status }),
    }),
  getKnowledgeFactSources: (factId: number) =>
    request<KnowledgeFactSources>(`/api/knowledge/facts/${factId}/sources`),
  startHistoryBackfill: (chatId: number, fromDate: string, toDate: string) =>
    request<HistoryBackfillTask>("/api/history-backfills", {
      method: "POST",
      body: JSON.stringify({ chatId, fromDate, toDate }),
    }),
  getHistoryBackfill: (taskId: string) =>
    request<HistoryBackfillTask>(`/api/history-backfills/${taskId}`),
  summaryStats: () => request<SummaryStats>("/api/summaries/stats"),
  listSummaries: (filters?: SummarySearchFilters) =>
    request<SummaryListResponse>(
      `/api/summaries${buildQuery({
        q: filters?.q?.trim() || undefined,
        chatId: filters?.chatId && filters.chatId !== "all" ? filters.chatId : undefined,
        status: filters?.status && filters.status !== "all" ? filters.status : undefined,
        delivery: filters?.delivery && filters.delivery !== "all" ? filters.delivery : undefined,
        dateFrom: filters?.dateFrom || undefined,
        dateTo: filters?.dateTo || undefined,
        page: filters?.page,
        pageSize: filters?.pageSize,
      })}`,
    ),
  summaryContextPreview: (summaryId: number) =>
    request<SummaryContextPreview>(
      `/api/summaries/context-preview?id=${summaryId}`,
    ),
  retrySummaryDelivery: (summaryId: number) =>
    request(`/api/summaries/${summaryId}/retry-delivery`, {
      method: "POST",
    }),
  runSummary: (chatId: number, date: string) =>
    request("/api/summaries/run", {
      method: "POST",
      body: JSON.stringify({ chatId, date }),
    }),
  listDeliveryChannels: async () =>
    normalizeList(await request<DeliveryChannel[] | null>("/api/channels")),
  createDeliveryChannel: (payload: DeliveryChannel) =>
    request<DeliveryChannel>("/api/channels/create", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  saveDeliveryChannel: (payload: DeliveryChannel) =>
    request<DeliveryChannel>(`/api/channels/${payload.id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  deleteDeliveryChannel: (id: number) =>
    request(`/api/channels/${id}`, {
      method: "DELETE",
    }),
  runChannelSummary: (channelId: number, date: string) =>
    request(`/api/channels/${channelId}/run`, {
      method: "POST",
      body: JSON.stringify({ date }),
    }),
};
