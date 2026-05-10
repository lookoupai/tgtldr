"use client";

import { startTransition, useDeferredValue, useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { AppSelect } from "@/components/app-select";
import { AppSettings, Bootstrap, Chat, DeliveryChannel, SummaryOutputLanguage } from "@/lib/types";
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

export function ChannelsPanel() {
  const [items, setItems] = useState<DeliveryChannel[]>([]);
  const [savedItems, setSavedItems] = useState<DeliveryChannel[]>([]);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [query, setQuery] = useState("");
  const [chats, setChats] = useState<Chat[]>([]);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const deferredQuery = useDeferredValue(query);
  const toast = useToast();

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    try {
      const [channelsData, chatsData, settingsData, bootstrapData] = await Promise.all([
        api.listDeliveryChannels(),
        api.listChats(),
        api.settings(),
        api.bootstrap(),
      ]);
      setItems(channelsData);
      setSavedItems(channelsData);
      setChats(chatsData);
      setSettings(settingsData);
      setBootstrap(bootstrapData);
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
      return channel.name.toLowerCase().includes(deferredQuery.toLowerCase());
    });
  }, [deferredQuery, items]);

  const enabledCount = savedItems.filter((ch) => ch.enabled).length;

  const editingChannel = editingId ? items.find((ch) => ch.id === editingId) : null;
  const sourceChats = chats.filter(
    (ch) => ch.enabled || Boolean(editingChannel?.sourceChatIds.includes(ch.id))
  );

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
      </MetricRail>

      <Surface>
        <div className="surface-header">
          <div className="search-row">
            <Input
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索通道..."
              value={query}
            />
            <Button onClick={() => startTransition(() => createChannel())} type="button">
              新建通道
            </Button>
          </div>
        </div>

        {filtered.length === 0 ? (
          <EmptyState
            title="暂无推送通道"
            description="创建通道后，可以监听多个群组并将摘要推送到指定目标。"
          />
        ) : (
          <div className="list-layout">
            <div className="list-sidebar">
              <table className="list-table">
                <thead>
                  <tr>
                    <th>通道名称</th>
                    <th>状态</th>
                    <th>源群组</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((channel) => (
                    <tr
                      key={channel.id}
                      className={editingId === channel.id ? "selected" : ""}
                      onClick={() => setEditingId(channel.id)}
                    >
                      <td>{channel.name}</td>
                      <td>
                        <StatusPill tone={channel.enabled ? "good" : "neutral"}>
                          {channel.enabled ? "已启用" : "未启用"}
                        </StatusPill>
                      </td>
                      <td>{channel.sourceChatIds.length} 个</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="list-main">
              {editingChannel ? (
                <ChannelEditor
                  channel={editingChannel}
                  chats={sourceChats}
                  onPatch={(patch) => patchChannel(editingChannel.id, patch)}
                  onSave={() => void saveChannel(editingChannel)}
                  onDelete={() => void deleteChannel(editingChannel.id)}
                />
              ) : null}
            </div>
          </div>
        )}
      </Surface>
    </DashboardPage>
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
  const selectedChats = chats.filter((ch) => channel.sourceChatIds.includes(ch.id));

  return (
    <div className="editor-layout">
      <div className="editor-header">
        <h3>{channel.name}</h3>
        <div className="editor-actions">
          <Button onClick={onSave} type="button">
            保存
          </Button>
          <Button onClick={onDelete} variant="destructive" type="button">
            删除
          </Button>
        </div>
      </div>

      <div className="editor-body">
        <Field label="通道名称" required>
          <Input
            value={channel.name}
            onChange={(event) => onPatch({ name: event.target.value })}
          />
        </Field>

        <Field label="启用状态">
          <label className="checkbox-label">
            <input
              type="checkbox"
              checked={channel.enabled}
              onChange={(event) => onPatch({ enabled: event.target.checked })}
            />
            启用此通道
          </label>
        </Field>

        <Field
          label="源群组"
          hint="选择要监听的群组，这些群组的消息将被聚合生成摘要。候选项来自群组页面中已启用“消息保存”的群组。"
          required
        >
          {chats.length === 0 ? (
            <p className="muted">暂无可选源群组，请先到群组页面启用需要监听的“消息保存”。</p>
          ) : (
            <div className="multi-select">
              {chats.map((chat) => (
                <label key={chat.id} className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={channel.sourceChatIds.includes(chat.id)}
                    onChange={(event) => {
                      const currentIds = channel.sourceChatIds;
                      if (event.target.checked) {
                        onPatch({ sourceChatIds: [...currentIds, chat.id] });
                      } else {
                        onPatch({ sourceChatIds: currentIds.filter((id) => id !== chat.id) });
                      }
                    }}
                  />
                  {chat.title}
                </label>
              ))}
            </div>
          )}
          {selectedChats.length > 0 && (
            <p className="field-hint">已选择 {selectedChats.length} 个群组</p>
          )}
        </Field>

        <Field
          label="目标 Chat ID"
          hint="摘要将推送到此目标。先在目标群组或私聊中给 Bot 发消息，然后点击获取。"
          required
        >
          <Input
            value={channel.targetChatId}
            onChange={(event) => onPatch({ targetChatId: event.target.value })}
            placeholder="例如：-1001234567890"
          />
        </Field>

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

        <Field label="推送时间" hint="每日推送时间，格式 HH:MM。">
          <Input
            type="time"
            value={channel.summaryTimeLocal}
            onChange={(event) => onPatch({ summaryTimeLocal: event.target.value })}
          />
        </Field>

        <Field label="摘要额外要求">
          <Textarea
            rows={4}
            placeholder="告诉模型你希望如何聚合这些群组的消息..."
            value={channel.summaryPrompt}
            onChange={(event) => onPatch({ summaryPrompt: event.target.value })}
          />
        </Field>

        <Field label="当前有效情报范围" hint="0 表示展示所有未过期 active 事实；1 表示只展示摘要日期当天来源消息对应的事实，30 表示截至摘要日期最近 30 天。">
          <Input
            min={0}
            type="number"
            value={channel.summaryKnowledgeDays}
            onChange={(event) => onPatch({ summaryKnowledgeDays: Number(event.target.value) })}
          />
        </Field>
      </div>

      <div className="editor-footer">
        <p className="muted">通道 ID: {channel.id}</p>
        <Button onClick={onSave} type="button">
          保存配置
        </Button>
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
