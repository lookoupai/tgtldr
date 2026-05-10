"use client";

import { startTransition, useDeferredValue, useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { AppSelect } from "@/components/app-select";
import { AppSettings, Bootstrap, BotTargetChatCandidate, Chat } from "@/lib/types";
import { DashboardPage, EmptyState, MetricCard, MetricRail, Surface } from "@/components/dashboard-page";
import { useToast } from "@/components/toast";
import { Button, Field, Input, StatusPill, Textarea } from "@/components/ui";
import { SummaryLanguageControl } from "@/components/summary-language-control";
import { describeBotChatCandidate, hasAvailableBotToken } from "@/lib/bot-target-chat";

type ChatTypeFilter = "all" | Chat["chatType"];
type SwitchFilter = "all" | "yes" | "no";
type HistoryMode = "1d" | "3d" | "7d" | "30d" | "custom";

const historyRangeOptions = [
  { value: "1d", label: "最近 1 天" },
  { value: "3d", label: "最近 3 天" },
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
  { value: "custom", label: "自定义日期范围" }
];

const topicGroupTemplates: {
  key: string;
  label: string;
  groups: Chat["topicGroups"];
}[] = [
  {
    key: "general",
    label: "通用社群",
    groups: [
      { name: "公告", description: "官方通知、规则调整、重要提醒" },
      { name: "新闻", description: "政策、市场、突发事件" },
      { name: "讨论", description: "观点交流、问题解答、经验分享" },
      { name: "活动", description: "会议、线下活动、报名信息" }
    ]
  },
  {
    key: "crypto",
    label: "加密投资",
    groups: [
      { name: "行情", description: "价格走势、资金费率、宏观影响" },
      { name: "项目", description: "新项目、生态进展、代币经济" },
      { name: "链上", description: "链上数据、地址异动、合约交互" },
      { name: "风险", description: "安全事件、骗局提醒、清算风险" }
    ]
  },
  {
    key: "product",
    label: "产品技术",
    groups: [
      { name: "需求", description: "用户反馈、功能建议、使用场景" },
      { name: "研发", description: "技术方案、Bug、发布进度" },
      { name: "运营", description: "增长、内容、社群维护" },
      { name: "决策", description: "结论、负责人、后续行动" }
    ]
  },
  {
    key: "ops",
    label: "运营活动",
    groups: [
      { name: "报名", description: "报名、名单、资格确认" },
      { name: "日程", description: "时间、地点、流程安排" },
      { name: "物料", description: "海报、文案、链接、资料" },
      { name: "复盘", description: "效果、问题、改进项" }
    ]
  }
];

export function ChatsPanel() {
  const [items, setItems] = useState<Chat[]>([]);
  const [savedItems, setSavedItems] = useState<Chat[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [chatType, setChatType] = useState<ChatTypeFilter>("all");
  const [messageSaveFilter, setMessageSaveFilter] = useState<SwitchFilter>("all");
  const [summaryFilter, setSummaryFilter] = useState<SwitchFilter>("all");
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const deferredQuery = useDeferredValue(query);
  const toast = useToast();

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    try {
      const [chatsData, settingsData, bootstrapData] = await Promise.all([
        api.listChats(),
        api.settings(),
        api.bootstrap()
      ]);
      const chats = chatsData.map(normalizeChat);
      setItems(chats);
      setSavedItems(chats);
      setSettings(settingsData);
      setBootstrap(bootstrapData);
      setEditingId((current) =>
        current && chats.some((chat) => chat.id === current) ? current : null
      );
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function saveChat(chat: Chat) {
    try {
      await api.saveChat(chat);
      toast.showSuccess(`已保存「${chat.title}」的配置。`);
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function bindBotChat(chat: Chat, chatID: string) {
    const nextChat = { ...chat, botChatId: chatID };
    patchChat(chat.id, { botChatId: chatID });
    try {
      await api.saveChat(nextChat);
      toast.showSuccess(`已绑定「${chat.title}」的 Bot Chat ID。`);
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function startHistoryBackfill(chat: Chat, fromDate: string, toDate: string) {
    try {
      const task = await api.startHistoryBackfill(chat.id, fromDate, toDate);
      toast.watchHistoryBackfill(task);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function patchChat(chatID: number, patch: Partial<Chat>) {
    setItems((current) =>
      current.map((item) => (item.id === chatID ? { ...item, ...patch } : item))
    );
  }

  const filtered = useMemo(() => {
    return items.filter((chat) => {
      if (!chat.title.toLowerCase().includes(deferredQuery.toLowerCase())) {
        return false;
      }
      if (chatType !== "all" && chat.chatType !== chatType) {
        return false;
      }
      if (messageSaveFilter !== "all") {
        const expected = messageSaveFilter === "yes";
        if (chat.enabled !== expected) {
          return false;
        }
      }
      if (summaryFilter !== "all") {
        const expected = summaryFilter === "yes";
        if (chat.summaryEnabled !== expected) {
          return false;
        }
      }
      return true;
    });
  }, [chatType, deferredQuery, items, messageSaveFilter, summaryFilter]);

  const syncedCount = savedItems.length;
  const messageSaveCount = savedItems.filter((chat) => chat.enabled).length;
  const summaryEnabledCount = savedItems.filter((chat) => chat.summaryEnabled).length;

  return (
    <DashboardPage
      title="群组"
      description="在这里选择需要保存消息和生成摘要的群组，并调整每个群的摘要设置。"
    >
      <MetricRail>
        <MetricCard
          label="已同步群组"
          value={syncedCount}
          badge="最新"
          detail="当前 Telegram 账号下可管理的群组与超级群组。"
        />
        <MetricCard
          label="已启用消息保存"
          value={messageSaveCount}
          tone={messageSaveCount > 0 ? "good" : "neutral"}
          badge={messageSaveCount > 0 ? "运行中" : "未启用"}
          detail="启用后，系统会实时保存该群的消息。"
        />
        <MetricCard
          label="已启用 AI 总结"
          value={summaryEnabledCount}
          tone={summaryEnabledCount > 0 ? "good" : "neutral"}
          badge={summaryEnabledCount > 0 ? "已配置" : "未启用"}
          detail="只有启用 AI 总结的群组才会参与每日摘要。"
        />
      </MetricRail>

      <Surface
        title="群组列表"
        description="先查看每个群当前是否启用，再通过操作按钮调整摘要规则、补充群聊背景或回补历史消息。"
      >
        <div className="toolbar-grid">
          <Field label="搜索群组">
            <Input
              placeholder="按群名搜索"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </Field>
          <Field label="群类型">
            <AppSelect
              onChange={(value) => setChatType(value as ChatTypeFilter)}
              options={[
                { value: "all", label: "全部" },
                { value: "group", label: "群组" },
                { value: "supergroup", label: "超级群组" }
              ]}
              value={chatType}
            />
          </Field>
          <Field label="消息保存">
            <AppSelect
              onChange={(value) => setMessageSaveFilter(value as SwitchFilter)}
              options={[
                { value: "all", label: "全部" },
                { value: "yes", label: "已启用" },
                { value: "no", label: "未启用" }
              ]}
              value={messageSaveFilter}
            />
          </Field>
          <Field label="AI 总结">
            <AppSelect
              onChange={(value) => setSummaryFilter(value as SwitchFilter)}
              options={[
                { value: "all", label: "全部" },
                { value: "yes", label: "已启用" },
                { value: "no", label: "未启用" }
              ]}
              value={summaryFilter}
            />
          </Field>
        </div>

        {filtered.length === 0 ? (
          <EmptyState
            title="没有匹配的群组"
            description="调整筛选条件后再试一次。"
          />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>群组名称</th>
                  <th>群类型</th>
                  <th>消息保存</th>
                  <th>AI 总结</th>
                  <th>Bot 查询</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((chat) => (
                  <ChatTableRow
                    key={chat.id}
                    chat={chat}
                    editing={editingId === chat.id}
                    botTokenAvailable={hasAvailableBotToken(settings?.botToken)}
                    telegramAuthorized={bootstrap?.telegramAuthorized ?? false}
                    onBackfill={(fromDate, toDate) =>
                      startTransition(() => void startHistoryBackfill(chat, fromDate, toDate))
                    }
                    onPatch={(patch) => patchChat(chat.id, patch)}
                    onBindBotChat={(chatID) => startTransition(() => void bindBotChat(chat, chatID))}
                    onEdit={() =>
                      setEditingId((current) => (current === chat.id ? null : chat.id))
                    }
                    onSave={() => startTransition(() => void saveChat(chat))}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Surface>

    </DashboardPage>
  );
}

function asMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function normalizeChat(chat: Chat): Chat {
  return {
    ...chat,
    summaryMode: chat.summaryMode === "chat_topic" ? "chat_topic" : "channel",
    summaryLanguage: chat.summaryLanguage ?? "",
    summaryContext: chat.summaryContext ?? "",
    topicGroups: Array.isArray(chat.topicGroups) ? chat.topicGroups : [],
    filteredKeywords: Array.isArray(chat.filteredKeywords) ? chat.filteredKeywords : [],
    filteredSenders: Array.isArray(chat.filteredSenders) ? chat.filteredSenders : [],
    keepBotMessages: chat.keepBotMessages ?? true,
    botChatId: chat.botChatId ?? "",
    botInteractionEnabled: chat.botInteractionEnabled ?? false,
    botAllowedUsers: Array.isArray(chat.botAllowedUsers) ? chat.botAllowedUsers : []
  };
}

function joinLines(values: string[]) {
  return values.join("\n");
}

function splitLines(value: string) {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function formatTopicGroups(groups: Chat["topicGroups"]) {
  return groups
    .map((group) =>
      group.description?.trim()
        ? `${group.name.trim()} | ${group.description.trim()}`
        : group.name.trim()
    )
    .filter(Boolean)
    .join("\n");
}

function parseTopicGroups(value: string): Chat["topicGroups"] {
  return value
    .split("\n")
    .map((item) => item.trim().replaceAll("｜", "|"))
    .filter(Boolean)
    .map((item) => {
      const [name, ...descriptionParts] = item.split("|");
      return {
        name: name.trim(),
        description: descriptionParts.join("|").trim()
      };
    })
    .filter((item) => item.name);
}

function matchBotCandidate(
  chat: Chat,
  candidates: BotTargetChatCandidate[]
): BotTargetChatCandidate | null {
  const username = normalizeName(chat.username);
  if (username) {
    const byUsername = candidates.find(
      (candidate) => normalizeName(candidate.username ?? "") === username
    );
    if (byUsername) {
      return byUsername;
    }
  }

  const title = normalizeName(chat.title);
  if (!title) {
    return null;
  }
  return (
    candidates.find((candidate) => normalizeName(candidate.title ?? "") === title) ??
    null
  );
}

function normalizeName(value: string) {
  return value.trim().replace(/^@/, "").toLowerCase();
}

function botChatStatus(
  chat: Chat,
  botTokenAvailable: boolean
): { label: string; tone: "neutral" | "good" | "warn" | "bad" } {
  if (!botTokenAvailable) {
    return { label: "缺 Token", tone: "warn" };
  }
  if (!chat.botChatId.trim()) {
    return { label: "未绑定", tone: "neutral" };
  }
  if (!chat.botInteractionEnabled) {
    return { label: "未开放", tone: "neutral" };
  }
  if (chat.botAllowedUsers.length > 0) {
    return { label: "白名单", tone: "good" };
  }
  return { label: "可查询", tone: "good" };
}

function ChatTableRow({
  chat,
  editing,
  botTokenAvailable,
  telegramAuthorized,
  onBackfill,
  onBindBotChat,
  onPatch,
  onEdit,
  onSave
}: {
  chat: Chat;
  editing: boolean;
  botTokenAvailable: boolean;
  telegramAuthorized: boolean;
  onBackfill: (fromDate: string, toDate: string) => void;
  onBindBotChat: (chatID: string) => void;
  onPatch: (patch: Partial<Chat>) => void;
  onEdit: () => void;
  onSave: () => void;
}) {
  const [historyMode, setHistoryMode] = useState<HistoryMode>("30d");
  const [historyFromDate, setHistoryFromDate] = useState(localDateOffset(-29));
  const [historyToDate, setHistoryToDate] = useState(localDateInputValue());
  const [historyExpanded, setHistoryExpanded] = useState(false);
  const [topicGroupsInput, setTopicGroupsInput] = useState(() =>
    formatTopicGroups(chat.topicGroups)
  );
  const [botTargetChatCandidates, setBotTargetChatCandidates] = useState<
    BotTargetChatCandidate[]
  >([]);
  const [resolvingBotTargetChat, setResolvingBotTargetChat] = useState(false);
  const toast = useToast();
  const historyRange = resolveHistoryRange(historyMode, historyFromDate, historyToDate);
  const expanded = editing || historyExpanded;
  const botStatus = botChatStatus(chat, botTokenAvailable);

  useEffect(() => {
    if (editing) {
      setTopicGroupsInput(formatTopicGroups(chat.topicGroups));
    }
  }, [chat.id, editing]);

  function patchTopicGroupsInput(value: string) {
    setTopicGroupsInput(value);
    onPatch({ topicGroups: parseTopicGroups(value) });
  }

  function applyTopicGroupTemplate(templateKey: string) {
    const template = topicGroupTemplates.find((item) => item.key === templateKey);
    if (!template) {
      return;
    }
    const value = formatTopicGroups(template.groups);
    setTopicGroupsInput(value);
    onPatch({ topicGroups: template.groups });
  }

  async function resolveBotTargetChat() {
    if (!telegramAuthorized) {
      toast.showError("自动获取前请先完成 Telegram 登录。");
      return;
    }
    if (!botTokenAvailable) {
      toast.showError("请先在系统配置中保存 Bot Token。");
      return;
    }

    setResolvingBotTargetChat(true);
    try {
      const result = await api.resolveBotTargetChat();
      setBotTargetChatCandidates(result.candidates);
      if (result.candidates.length === 0) {
        toast.showError("未找到最近消息，请先在目标会话里给 Bot 发一条消息后再重试。");
        return;
      }
      const matched = matchBotCandidate(chat, result.candidates);
      if (matched) {
        onBindBotChat(matched.chatId);
        setBotTargetChatCandidates([]);
        return;
      }
      if (result.candidates.length === 1) {
        const [candidate] = result.candidates;
        onBindBotChat(candidate.chatId);
        setBotTargetChatCandidates([]);
        return;
      }
      toast.showSuccess("找到了多个可能的会话，请选择一个。");
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setResolvingBotTargetChat(false);
    }
  }

  return (
    <>
      <tr className={expanded ? "data-row active" : "data-row"}>
        <td>
          <div className="data-row-title">
            <strong>{chat.title}</strong>
            <span>{chat.username ? `@${chat.username}` : "无公开用户名"}</span>
          </div>
        </td>
        <td>{chat.chatType === "supergroup" ? "超级群组" : "群组"}</td>
        <td>
          <StatusPill tone={chat.enabled ? "good" : "neutral"}>
            {chat.enabled ? "已启用" : "未启用"}
          </StatusPill>
        </td>
        <td>
          <StatusPill tone={chat.summaryEnabled ? "good" : "neutral"}>
            {chat.summaryEnabled ? "已启用" : "未启用"}
          </StatusPill>
        </td>
        <td>
          <StatusPill tone={botStatus.tone}>{botStatus.label}</StatusPill>
        </td>
        <td>
          <div className="table-row-actions">
            <Button
              aria-label={editing ? "收起编辑" : "编辑群组配置"}
              className={editing ? "table-icon-button active" : "table-icon-button"}
              onClick={onEdit}
              title={editing ? "收起编辑" : "编辑群组配置"}
              type="button"
              variant="ghost"
            >
              <EditIcon />
            </Button>
            <Button
              aria-label={historyExpanded ? "收起历史消息回补" : "加载历史消息"}
              className={historyExpanded ? "table-icon-button active" : "table-icon-button"}
              onClick={() => setHistoryExpanded((current) => !current)}
              title={historyExpanded ? "收起历史消息回补" : "加载历史消息"}
              type="button"
              variant="ghost"
            >
              <HistoryIcon />
            </Button>
          </div>
        </td>
      </tr>

      {expanded ? (
        <tr className="data-row-detail">
          <td colSpan={6}>
            <div className="table-editor">
              {editing ? (
                <>
                  <div className="form-grid table-editor-primary-grid">
                    <Field label="消息保存">
                      <AppSelect
                        onChange={(value) => onPatch({ enabled: value === "yes" })}
                        options={[
                          { value: "yes", label: "启用" },
                          { value: "no", label: "停用" }
                        ]}
                        value={chat.enabled ? "yes" : "no"}
                      />
                    </Field>

                    <Field label="AI 总结">
                      <AppSelect
                        onChange={(value) =>
                          onPatch({ summaryEnabled: value === "yes" })
                        }
                        options={[
                          { value: "yes", label: "启用" },
                          { value: "no", label: "停用" }
                        ]}
                        value={chat.summaryEnabled ? "yes" : "no"}
                      />
                    </Field>
                  </div>

                  <div className="form-grid">
                    <Field
                      label="Bot Chat ID"
                      hint="把 Bot 加入目标私聊或群聊并发一条消息后，可自动获取并绑定。"
                    >
                      <div className="bot-target-chat-field">
                        <Input
                          placeholder="例如 -1001234567890"
                          value={chat.botChatId}
                          onChange={(event) => {
                            setBotTargetChatCandidates([]);
                            onPatch({ botChatId: event.target.value });
                          }}
                        />
                        <div className="button-row">
                          <Button
                            disabled={
                              resolvingBotTargetChat ||
                              !telegramAuthorized ||
                              !botTokenAvailable
                            }
                            onClick={() => void resolveBotTargetChat()}
                            type="button"
                            variant="secondary"
                          >
                            {resolvingBotTargetChat ? "正在获取..." : "获取 Chat ID"}
                          </Button>
                        </div>
                        {botTargetChatCandidates.length > 1 ? (
                          <div className="bot-chat-candidates">
                            {botTargetChatCandidates.map((candidate) => (
                              <Button
                                className="bot-chat-candidate"
                                key={candidate.chatId}
                                onClick={() => {
                                  onBindBotChat(candidate.chatId);
                                  setBotTargetChatCandidates([]);
                                }}
                                type="button"
                                variant={
                                  chat.botChatId === candidate.chatId
                                    ? "primary"
                                    : "secondary"
                                }
                              >
                                {describeBotChatCandidate(candidate)}
                              </Button>
                            ))}
                          </div>
                        ) : null}
                      </div>
                    </Field>

                    <Field label="允许 Bot 查询">
                      <AppSelect
                        onChange={(value) =>
                          onPatch({ botInteractionEnabled: value === "yes" })
                        }
                        options={[
                          { value: "no", label: "不允许" },
                          { value: "yes", label: "允许" }
                        ]}
                        value={chat.botInteractionEnabled ? "yes" : "no"}
                      />
                    </Field>
                  </div>

                  <Field
                    label="允许查询用户"
                    hint="留空表示该 Bot Chat ID 内所有用户都可查询；每行填写 @username 或 Telegram 数字用户 ID。"
                  >
                    <Textarea
                      rows={4}
                      placeholder={"@alice\n123456789"}
                      value={joinLines(chat.botAllowedUsers)}
                      onChange={(event) =>
                        onPatch({ botAllowedUsers: splitLines(event.target.value) })
                      }
                    />
                  </Field>
                  <p className="table-editor-note">
                    当前状态：{botStatus.label}。启用后，Bot 只会响应这个 Chat ID 里的明确命令、@BotName 提问或对 Bot 消息的回复；普通群消息不会触发查询。
                  </p>

                  {chat.summaryEnabled ? (
                    <>
                      <div className="form-grid">
                        <Field label="AI 总结交付方式">
                          <AppSelect
                            onChange={(value) =>
                              onPatch({
                                deliveryMode: value as Chat["deliveryMode"]
                              })
                            }
                            options={[
                              { value: "dashboard", label: "仅在网页端查看" },
                              { value: "bot", label: "通过 Bot 推送" }
                            ]}
                            value={chat.deliveryMode}
                          />
                        </Field>

                        <Field label="摘要模式">
                          <AppSelect
                            onChange={(value) =>
                              onPatch({
                                summaryMode: value as Chat["summaryMode"]
                              })
                            }
                            options={[
                              { value: "channel", label: "按群组摘要" },
                              { value: "chat_topic", label: "按 AI 话题分组" }
                            ]}
                            value={chat.summaryMode}
                          />
                        </Field>

                        <Field
                          label="摘要输出语言"
                          hint="留空时跟随系统默认摘要输出语言。"
                        >
                          <SummaryLanguageControl
                            includeInherit
                            onChange={(summaryLanguage) =>
                              onPatch({ summaryLanguage })
                            }
                            value={chat.summaryLanguage ?? ""}
                          />
                        </Field>

                        <Field label="摘要时间">
                          <Input
                            value={chat.summaryTimeLocal}
                            onChange={(event) =>
                              onPatch({ summaryTimeLocal: event.target.value })
                            }
                          />
                        </Field>

                        <Field
                          label="当前有效情报范围"
                          hint="0 表示展示所有未过期 active 事实；1 表示只展示摘要日期当天来源消息对应的事实，30 表示截至摘要日期最近 30 天。"
                        >
                          <Input
                            min={0}
                            type="number"
                            value={chat.summaryKnowledgeDays}
                            onChange={(event) =>
                              onPatch({ summaryKnowledgeDays: Number(event.target.value) })
                            }
                          />
                        </Field>

                        <Field label="模型 override">
                          <Input
                            placeholder="例如 gpt-4.1-mini"
                            value={chat.modelOverride}
                            onChange={(event) =>
                              onPatch({ modelOverride: event.target.value })
                            }
                          />
                        </Field>

                        <Field label="保留机器人消息">
                          <AppSelect
                            onChange={(value) =>
                              onPatch({ keepBotMessages: value === "yes" })
                            }
                            options={[
                              { value: "yes", label: "保留" },
                              { value: "no", label: "过滤" }
                            ]}
                            value={chat.keepBotMessages ? "yes" : "no"}
                          />
                        </Field>
                      </div>
                      <p className="table-editor-note">
                        模型 override 留空时会跟随系统默认模型。
                      </p>

                      {chat.summaryMode === "chat_topic" ? (
                        <Field
                          label="话题分组"
                          hint="每行一个，格式为「名称 | 描述」。未命中的内容会归入“其他”。"
                        >
                          <div className="topic-group-editor">
                            <div className="topic-group-template">
                              <AppSelect
                                onChange={applyTopicGroupTemplate}
                                options={[
                                  { value: "", label: "选择常用模板" },
                                  ...topicGroupTemplates.map((template) => ({
                                    value: template.key,
                                    label: template.label
                                  }))
                                ]}
                                value=""
                              />
                            </div>
                            <Textarea
                              rows={5}
                              placeholder={"新闻 | 政策、市场、突发事件\n活动 | 会议、线下活动、报名信息\n体育 | 比赛、转会、赛事讨论"}
                              value={topicGroupsInput}
                              onChange={(event) =>
                                patchTopicGroupsInput(event.target.value)
                              }
                            />
                          </div>
                        </Field>
                      ) : null}

                      <div className="form-grid">
                        <Field
                          label="过滤发言人"
                          hint="每行一个，支持昵称或 @username，精确匹配。"
                        >
                          <Textarea
                            rows={5}
                            placeholder={"验证机器人\n@verify_bot"}
                            value={joinLines(chat.filteredSenders)}
                            onChange={(event) =>
                              onPatch({ filteredSenders: splitLines(event.target.value) })
                            }
                          />
                        </Field>

                        <Field
                          label="过滤关键词"
                          hint="每行一个，按包含关系过滤消息内容。"
                        >
                          <Textarea
                            rows={5}
                            placeholder={"请完成入群验证\n验证已过期"}
                            value={joinLines(chat.filteredKeywords)}
                            onChange={(event) =>
                              onPatch({ filteredKeywords: splitLines(event.target.value) })
                            }
                          />
                        </Field>
                      </div>

                      <Field
                        label="群聊背景"
                        hint="补充群里常见黑话、术语、长期背景或默认语境，帮助模型正确理解讨论内容。"
                      >
                        <Textarea
                          rows={6}
                          placeholder="例如：这个群主要讨论二级市场和链上项目；ATL 指 All Time Low；喊单通常是半开玩笑表达。"
                          value={chat.summaryContext}
                          onChange={(event) =>
                            onPatch({ summaryContext: event.target.value })
                          }
                        />
                      </Field>

                      <Field label="摘要额外要求">
                        <Textarea
                          rows={8}
                          placeholder="告诉模型你希望如何总结这个群的消息，例如保留决策、行动项和重要链接。"
                          value={chat.summaryPrompt}
                          onChange={(event) =>
                            onPatch({ summaryPrompt: event.target.value })
                          }
                        />
                      </Field>
                    </>
                  ) : null}

                  <div className="editor-footer">
                    <p className="muted">
                      当前群类型：{chat.chatType === "supergroup" ? "超级群组" : "群组"}
                    </p>
                    <Button onClick={onSave} type="button">
                      保存该群配置
                    </Button>
                  </div>
                </>
              ) : null}

              {historyExpanded ? (
                <div className="history-backfill-panel">
                  <div className="history-backfill-head">
                    <strong>加载历史消息</strong>
                    <p>回补后，这个群较早时间段的消息也可以用于生成摘要。</p>
                  </div>
                  <div className="form-grid">
                    <Field label="时间范围">
                      <AppSelect
                        onChange={(value) => setHistoryMode(value as HistoryMode)}
                        options={historyRangeOptions}
                        value={historyMode}
                      />
                    </Field>
                    {historyMode === "custom" ? (
                      <>
                        <Field label="开始日期">
                          <Input
                            type="date"
                            value={historyFromDate}
                            onChange={(event) => setHistoryFromDate(event.target.value)}
                          />
                        </Field>
                        <Field label="结束日期">
                          <Input
                            type="date"
                            value={historyToDate}
                            onChange={(event) => setHistoryToDate(event.target.value)}
                          />
                        </Field>
                      </>
                    ) : (
                      <div className="history-backfill-range muted">
                        将回补 {historyRange.fromDate} 到 {historyRange.toDate} 的消息。
                      </div>
                    )}
                  </div>
                  <div className="history-backfill-actions">
                    <Button
                      disabled={!historyRange.fromDate || !historyRange.toDate}
                      onClick={() => onBackfill(historyRange.fromDate, historyRange.toDate)}
                      type="button"
                      variant="secondary"
                    >
                      加载历史消息
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>
          </td>
        </tr>
      ) : null}
    </>
  );
}

function resolveHistoryRange(
  mode: HistoryMode,
  customFromDate: string,
  customToDate: string
) {
  if (mode === "custom") {
    return { fromDate: customFromDate, toDate: customToDate };
  }
  const days = Number.parseInt(mode, 10);
  return { fromDate: localDateOffset(1 - days), toDate: localDateInputValue() };
}

function localDateOffset(offsetDays: number) {
  const now = new Date();
  now.setDate(now.getDate() + offsetDays);
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

function localDateInputValue() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

function EditIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
      <path
        d="M4 20h4l10-10-4-4L4 16v4Z"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
      <path
        d="M13.5 6.5l4 4"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}

function HistoryIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="16" viewBox="0 0 24 24" width="16">
      <path
        d="M3 12a9 9 0 1 0 3-6.708"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
      <path
        d="M3 4v4h4"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
      <path
        d="M12 7.5v5l3 2"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.8"
      />
    </svg>
  );
}
