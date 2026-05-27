"use client";

import { startTransition, useDeferredValue, useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { AppSelect } from "@/components/app-select";
import { AppSettings, Chat, DeliveryChannel, SummaryOutputLanguage } from "@/lib/types";
import { DashboardPage, EmptyState, MetricCard, MetricRail, Surface } from "@/components/dashboard-page";
import { useToast } from "@/components/toast";
import { Button, Field, Input, StatusPill, Textarea } from "@/components/ui";
import { SummaryLanguageControl } from "@/components/summary-language-control";

const contentFilterOptions = [
  { value: "", label: "全部内容（不限制主题）" },
  { value: "supply_demand", label: "仅保留供需信息" },
  { value: "knowledge", label: "仅保留知识事实" },
  { value: "technical", label: "仅保留技术讨论" },
];

const channelStatusOptions = [
  { value: "yes", label: "启用" },
  { value: "no", label: "停用" },
];

type ChannelStatusFilter = "all" | "yes" | "no";

const languageLabels: Record<string, string> = {
  "zh-CN": "中文",
  en: "English",
  ru: "Русский",
  ar: "العربية",
};

export function ChannelsPanel() {
  const [items, setItems] = useState<DeliveryChannel[]>([]);
  const [savedItems, setSavedItems] = useState<DeliveryChannel[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<ChannelStatusFilter>("all");
  const [chats, setChats] = useState<Chat[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const deferredQuery = useDeferredValue(query);
  const toast = useToast();

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    try {
      const [channelsData, chatsData, settingsData] = await Promise.all([
        api.listDeliveryChannels(),
        api.listChats(),
        api.settings(),
      ]);
      setItems(channelsData);
      setSavedItems(channelsData);
      setChats(chatsData);
      setSettings(settingsData);
      setEditingId((current) =>
        current && channelsData.some((ch) => ch.id === current) ? current : null
      );
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function saveChannel(channel: DeliveryChannel) {
    const validationError = validateChannel(channel);
    if (validationError) {
      toast.showError(validationError);
      return;
    }

    const payload = normalizeChannel(channel);
    const creating = channel.id <= 0;
    try {
      const saved = creating
        ? await api.createDeliveryChannel(payload)
        : await api.saveDeliveryChannel(payload);
      toast.showSuccess(
        creating ? `已创建通道「${saved.name}」。` : `已保存「${saved.name}」的配置。`
      );
      await load();
      setEditingId(saved.id);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function createChannel() {
    const draft: DeliveryChannel = {
      id: nextDraftChannelID(items),
      name: "新通道",
      enabled: true,
      sourceChatIds: [],
      targetChatId: "",
      targetLanguage: "zh-CN",
      contentFilter: "",
      contentFilterTypes: [],
      summaryTimeLocal: "09:00",
      summaryTimezone: settings?.defaultTimezone || "Asia/Shanghai",
      summaryPrompt: "",
      summaryKnowledgeDays: 0,
      createdAt: "",
      updatedAt: "",
    };
    setQuery("");
    setItems((current) => [draft, ...current]);
    setEditingId(draft.id);
  }

  async function deleteChannel(id: number) {
    if (id <= 0) {
      setItems((current) => current.filter((item) => item.id !== id));
      setEditingId((current) => (current === id ? null : current));
      toast.showSuccess("未保存的通道草稿已移除。");
      return;
    }

    try {
      await api.deleteDeliveryChannel(id);
      toast.showSuccess("通道已删除。");
      setEditingId(null);
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function patchChannel(channelID: number, patch: Partial<DeliveryChannel>) {
    setItems((current) =>
      current.map((item) => (item.id === channelID ? { ...item, ...patch } : item))
    );
  }

  const filtered = useMemo(() => {
    return items.filter((channel) => {
      if (!channel.name.toLowerCase().includes(deferredQuery.toLowerCase())) {
        return false;
      }
      if (statusFilter !== "all") {
        const expected = statusFilter === "yes";
        if (channel.enabled !== expected) {
          return false;
        }
      }
      return true;
    });
  }, [deferredQuery, items, statusFilter]);

  const enabledCount = savedItems.filter((ch) => ch.enabled).length;
  const coveredChatCount = new Set(savedItems.flatMap((channel) => channel.sourceChatIds)).size;
  const hasQuery = deferredQuery.trim().length > 0;

  return (
    <DashboardPage
      title="推送通道"
      description="配置跨群组摘要聚合与多目标推送。可以监听多个群组，将消息汇总后推送到指定目标，支持按语言和内容类型过滤。"
    >
      <MetricRail>
        <MetricCard
          label="已配置通道"
          value={savedItems.length}
          badge="总量"
          detail="已创建的推送通道数量。"
        />
        <MetricCard
          label="已启用通道"
          value={enabledCount}
          tone={enabledCount > 0 ? "good" : "neutral"}
          badge={enabledCount > 0 ? "运行中" : "未启用"}
          detail="启用后会按配置时间自动推送。"
        />
        <MetricCard
          label="覆盖源群组"
          value={coveredChatCount}
          badge="监听范围"
          detail="至少被一个推送通道纳入监听的群组数量。"
        />
      </MetricRail>

      <Surface
        title="通道列表"
        description="先查看每个通道当前是否启用，再展开编辑源群组、推送目标和摘要规则。"
        actions={
          <Button onClick={() => startTransition(() => createChannel())} type="button">
            新建通道
          </Button>
        }
      >
        <div className="toolbar-grid">
          <Field label="搜索通道">
            <Input
              onChange={(event) => setQuery(event.target.value)}
              placeholder="按通道名称搜索"
              value={query}
            />
          </Field>
          <Field label="启用状态">
            <AppSelect
              onChange={(value) => setStatusFilter(value as ChannelStatusFilter)}
              options={[
                { value: "all", label: "全部" },
                { value: "yes", label: "已启用" },
                { value: "no", label: "未启用" },
              ]}
              value={statusFilter}
            />
          </Field>
        </div>
        <div className="channel-toolbar-meta">
          <span>共 {savedItems.length} 个通道</span>
          <span>已启用 {enabledCount} 个</span>
          <span>当前显示 {filtered.length} 个</span>
        </div>

        {filtered.length === 0 ? (
          <EmptyState
            title={hasQuery ? "没有匹配的通道" : "暂无推送通道"}
            description={
              hasQuery || statusFilter !== "all"
                ? "调整搜索关键词后再试一次，或直接创建新的推送通道。"
                : "创建通道后，可以监听多个群组并将摘要推送到指定目标。"
            }
          />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>通道名称</th>
                  <th>状态</th>
                  <th>源群组</th>
                  <th>输出语言</th>
                  <th>推送时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((channel) => (
                  <ChannelTableRow
                    key={channel.id}
                    channel={channel}
                    chats={chats}
                    editing={editingId === channel.id}
                    onDelete={() => void deleteChannel(channel.id)}
                    onEdit={() =>
                      setEditingId((current) => (current === channel.id ? null : channel.id))
                    }
                    onPatch={(patch) => patchChannel(channel.id, patch)}
                    onSave={() => void saveChannel(channel)}
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

function ChannelTableRow({
  channel,
  chats,
  editing,
  onPatch,
  onSave,
  onDelete,
  onEdit,
}: {
  channel: DeliveryChannel;
  chats: Chat[];
  editing: boolean;
  onPatch: (patch: Partial<DeliveryChannel>) => void;
  onSave: () => void;
  onDelete: () => void;
  onEdit: () => void;
}) {
  const availableSourceChats = chats.filter(
    (chat) => chat.enabled || channel.sourceChatIds.includes(chat.id)
  );
  const selectedSourceChats = chats.filter((chat) => channel.sourceChatIds.includes(chat.id));

  return (
    <>
      <tr className={editing ? "data-row active" : "data-row"}>
        <td>
          <div className="data-row-title">
            <strong>{channel.name}</strong>
            <span>
              {channel.targetChatId.trim()
                ? `目标 ${channel.targetChatId.trim()}`
                : "未设置目标 Chat ID"}
            </span>
            <div className="channel-row-meta">
              <span>{describeContentFilter(channel.contentFilter)}</span>
              <span>
                {selectedSourceChats.length > 0
                  ? describeSelectedChats(selectedSourceChats)
                  : "未选择源群组"}
              </span>
            </div>
          </div>
        </td>
        <td>
          <StatusPill tone={channel.enabled ? "good" : "neutral"}>
            {channel.enabled ? "已启用" : "未启用"}
          </StatusPill>
        </td>
        <td>{channel.sourceChatIds.length} 个</td>
        <td>{formatOutputLanguage(channel.targetLanguage)}</td>
        <td>{channel.summaryTimeLocal || "--:--"}</td>
        <td>
          <div className="table-row-actions">
            <Button
              aria-label={editing ? "收起编辑" : "编辑通道配置"}
              className={editing ? "table-icon-button active" : "table-icon-button"}
              onClick={onEdit}
              title={editing ? "收起编辑" : "编辑通道配置"}
              type="button"
              variant="ghost"
            >
              <EditIcon />
            </Button>
          </div>
        </td>
      </tr>

      {editing ? (
        <tr className="data-row-detail">
          <td colSpan={6}>
            <ChannelEditor
              channel={channel}
              chats={availableSourceChats}
              onDelete={onDelete}
              onPatch={onPatch}
              onSave={onSave}
            />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function ChannelEditor({
  channel,
  chats,
  onPatch,
  onSave,
  onDelete,
}: {
  channel: DeliveryChannel;
  chats: Chat[];
  onPatch: (patch: Partial<DeliveryChannel>) => void;
  onSave: () => void;
  onDelete: () => void;
}) {
  const [sourceQuery, setSourceQuery] = useState("");
  const deferredSourceQuery = useDeferredValue(sourceQuery);

  useEffect(() => {
    setSourceQuery("");
  }, [channel.id]);

  const selectedChatIDSet = useMemo(() => new Set(channel.sourceChatIds), [channel.sourceChatIds]);
  const selectedChats = useMemo(
    () => chats.filter((chat) => selectedChatIDSet.has(chat.id)),
    [chats, selectedChatIDSet]
  );
  const filteredChats = useMemo(() => {
    const keyword = deferredSourceQuery.trim().toLowerCase();
    const normalizedKeyword = keyword.replace(/^@/, "");
    const matched = chats.filter((chat) => {
      if (!keyword) {
        return true;
      }
      const title = chat.title.toLowerCase();
      const username = chat.username.toLowerCase();
      return title.includes(keyword) || username.includes(normalizedKeyword);
    });
    return matched.sort((left, right) => {
      const leftSelected = selectedChatIDSet.has(left.id) ? 1 : 0;
      const rightSelected = selectedChatIDSet.has(right.id) ? 1 : 0;
      return rightSelected - leftSelected;
    });
  }, [chats, deferredSourceQuery, selectedChatIDSet]);
  const targetChatIdAssessment = assessTargetChatId(channel.targetChatId);

  return (
    <div className="table-editor channel-editor-stack">
      <div className="settings-overview-grid">
        <div className="settings-overview-item">
          <span>启用状态</span>
          <strong>{channel.enabled ? "已启用" : "未启用"}</strong>
        </div>
        <div className="settings-overview-item">
          <span>目标会话</span>
          <strong>{describeTargetChatOverview(channel.targetChatId)}</strong>
        </div>
        <div className="settings-overview-item">
          <span>源群组</span>
          <strong>{selectedChats.length} 个</strong>
        </div>
        <div className="settings-overview-item">
          <span>内容范围</span>
          <strong>{describeContentFilter(channel.contentFilter)}</strong>
        </div>
      </div>

      <section className="channel-editor-section">
        <div className="channel-editor-section-head">
          <div>
            <h4>基础配置</h4>
            <p>确定通道名称、目标会话和每日推送时间。</p>
          </div>
        </div>
        <div className="form-grid">
          <Field label="通道名称" required>
            <Input
              value={channel.name}
              onChange={(event) => onPatch({ name: event.target.value })}
            />
          </Field>

          <Field label="启用状态">
            <AppSelect
              onChange={(value) => onPatch({ enabled: value === "yes" })}
              options={channelStatusOptions}
              value={channel.enabled ? "yes" : "no"}
            />
          </Field>

          <Field
            label="目标 Chat ID"
            hint="摘要将推送到此目标。先在目标群组或私聊中给 Bot 发消息，再填写对应 Chat ID。"
            required
          >
            <Input
              value={channel.targetChatId}
              onChange={(event) => onPatch({ targetChatId: event.target.value })}
              placeholder="例如：-1001234567890"
            />
          </Field>

          <Field label="推送时间" hint="每日推送时间，格式 HH:MM。">
            <Input
              type="time"
              value={channel.summaryTimeLocal}
              onChange={(event) => onPatch({ summaryTimeLocal: event.target.value })}
            />
          </Field>
        </div>
        <div className="channel-chatid-panel">
          <div className="channel-chatid-panel-row">
            <StatusPill tone={targetChatIdAssessment.tone}>
              {targetChatIdAssessment.label}
            </StatusPill>
            <span>{targetChatIdAssessment.detail}</span>
          </div>
          {channel.targetChatId.trim() ? (
            <p className="channel-chatid-value">当前值：{channel.targetChatId.trim()}</p>
          ) : null}
        </div>
      </section>

      <section className="channel-editor-section">
        <div className="channel-editor-section-head">
          <div>
            <h4>摘要策略</h4>
            <p>控制输出语言、内容范围和模型可见的有效情报窗口。</p>
          </div>
        </div>
        <div className="form-grid">
          <Field label="输出语言">
            <SummaryLanguageControl
              value={channel.targetLanguage as SummaryOutputLanguage}
              onChange={(value) => onPatch({ targetLanguage: value })}
            />
          </Field>

          <Field
            label="内容范围"
            hint="控制聚合摘要重点保留哪些内容；选择“仅保留供需信息”表示摘要只关注供需相关消息，不是把供需信息过滤掉。"
          >
            <AppSelect
              value={channel.contentFilter}
              onChange={(value) => onPatch({ contentFilter: value })}
              options={contentFilterOptions}
            />
          </Field>

          <Field
            label="当前有效情报范围"
            hint="0 表示展示所有未过期 active 事实；1 表示只展示摘要日期当天来源消息对应的事实，30 表示截至摘要日期最近 30 天。"
          >
            <Input
              min={0}
              type="number"
              value={channel.summaryKnowledgeDays}
              onChange={(event) => onPatch({ summaryKnowledgeDays: Number(event.target.value) })}
            />
          </Field>
        </div>
        <Field label="摘要额外要求">
          <Textarea
            rows={5}
            placeholder="告诉模型你希望如何聚合这些群组的消息..."
            value={channel.summaryPrompt}
            onChange={(event) => onPatch({ summaryPrompt: event.target.value })}
          />
        </Field>
      </section>

      <section className="channel-editor-section">
        <div className="channel-editor-section-head">
          <div>
            <h4>源群组</h4>
            <p>选择要纳入聚合的群组；这里只展示已启用“消息保存”的候选项。</p>
          </div>
          <StatusPill tone="neutral">已选 {selectedChats.length} 个</StatusPill>
        </div>
        <div className="channel-source-toolbar">
          <Input
            onChange={(event) => setSourceQuery(event.target.value)}
            placeholder="按群组名或 @username 搜索"
            value={sourceQuery}
          />
          <div className="channel-source-toolbar-meta">
            <span>候选 {chats.length} 个</span>
            <span>匹配 {filteredChats.length} 个</span>
          </div>
        </div>
        {chats.length === 0 ? (
          <p className="muted">暂无可选源群组，请先到群组页面启用需要监听的“消息保存”。</p>
        ) : filteredChats.length === 0 ? (
          <div className="channel-source-empty">
            <strong>没有匹配的群组</strong>
            <p>调整搜索关键词后再试一次。</p>
          </div>
        ) : (
          <div className="channel-source-grid">
            {filteredChats.map((chat) => {
              const checked = selectedChatIDSet.has(chat.id);
              return (
                <label
                  key={chat.id}
                  className={`channel-source-option${checked ? " checked" : ""}`}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={(event) => {
                      const currentIds = channel.sourceChatIds;
                      if (event.target.checked) {
                        onPatch({ sourceChatIds: [...currentIds, chat.id] });
                        return;
                      }
                      onPatch({ sourceChatIds: currentIds.filter((id) => id !== chat.id) });
                    }}
                  />
                  <span className="channel-source-copy">
                    <strong>{chat.title}</strong>
                    <span>
                      {chat.username
                        ? `@${chat.username}`
                        : chat.chatType === "supergroup"
                          ? "超级群组"
                          : "群组"}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
        )}
        {selectedChats.length > 0 ? (
          <p className="field-hint">已选择：{describeSelectedChats(selectedChats)}</p>
        ) : null}
      </section>

      <div className="editor-footer">
        <p className="muted">{channel.id > 0 ? `通道 ID: ${channel.id}` : "未保存的通道草稿"}</p>
        <div className="button-row">
          <Button onClick={onDelete} variant="destructive" type="button">
            删除
          </Button>
          <Button onClick={onSave} type="button">
            保存配置
          </Button>
        </div>
      </div>
    </div>
  );
}

function asMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function validateChannel(channel: DeliveryChannel): string {
  if (!channel.name.trim()) {
    return "请填写通道名称。";
  }
  if (channel.sourceChatIds.length === 0) {
    return "请选择至少一个源群组。";
  }
  if (!channel.targetChatId.trim()) {
    return "请填写目标 Chat ID。";
  }
  if (assessTargetChatId(channel.targetChatId).kind === "invalid") {
    return "目标 Chat ID 格式不正确，请输入纯数字，可带前导负号。";
  }
  return "";
}

function normalizeChannel(channel: DeliveryChannel): DeliveryChannel {
  return {
    ...channel,
    name: channel.name.trim(),
    targetChatId: channel.targetChatId.trim(),
    sourceChatIds: Array.from(new Set(channel.sourceChatIds)),
    summaryKnowledgeDays: Math.max(0, Number(channel.summaryKnowledgeDays) || 0),
  };
}

function nextDraftChannelID(channels: DeliveryChannel[]): number {
  const draftIDs = channels.filter((channel) => channel.id <= 0).map((channel) => channel.id);
  const minDraftID = draftIDs.length > 0 ? Math.min(...draftIDs) : 0;
  return minDraftID - 1;
}

function describeContentFilter(value: string): string {
  const normalized = value.trim();
  return (
    contentFilterOptions.find((option) => option.value === normalized)?.label ||
    normalized ||
    "全部内容（不限制主题）"
  );
}

function describeSelectedChats(chats: Chat[]): string {
  const names = chats.map((chat) => chat.title);
  if (names.length <= 3) {
    return names.join("、");
  }
  return `${names.slice(0, 3).join("、")} 等 ${names.length} 个群组`;
}

function formatOutputLanguage(value: SummaryOutputLanguage): string {
  const normalized = String(value ?? "").trim();
  if (!normalized) {
    return "默认";
  }
  return languageLabels[normalized] ?? normalized;
}

function describeTargetChatOverview(value: string): string {
  const normalized = value.trim();
  if (!normalized) {
    return "未设置";
  }
  const assessment = assessTargetChatId(normalized);
  return `${assessment.label} · ${normalized}`;
}

function assessTargetChatId(value: string): {
  kind: "empty" | "invalid" | "channel" | "group" | "private";
  label: string;
  detail: string;
  tone: "neutral" | "good" | "warn" | "bad";
} {
  const normalized = value.trim();
  if (!normalized) {
    return {
      kind: "empty",
      label: "未设置",
      detail: "请输入目标会话的 Chat ID。先在目标群组或私聊里给 Bot 发一条消息，再把对应 Chat ID 填到这里。",
      tone: "neutral",
    };
  }
  if (!/^-?\d+$/.test(normalized)) {
    return {
      kind: "invalid",
      label: "格式有误",
      detail: "Chat ID 只能包含数字，可带前导负号，例如 -1001234567890。",
      tone: "bad",
    };
  }
  if (normalized.startsWith("-100")) {
    return {
      kind: "channel",
      label: "群组 / 频道",
      detail: "看起来像超级群组或频道的 Chat ID，适合作为 Bot 推送目标。",
      tone: "good",
    };
  }
  if (normalized.startsWith("-")) {
    return {
      kind: "group",
      label: "普通群组",
      detail: "看起来像普通群组的 Chat ID，请确认 Bot 已在该群并且具备发消息权限。",
      tone: "neutral",
    };
  }
  return {
    kind: "private",
    label: "私聊 / 用户",
    detail: "看起来像私聊或用户 Chat ID，请确认这是你期望的接收目标。",
    tone: "warn",
  };
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
