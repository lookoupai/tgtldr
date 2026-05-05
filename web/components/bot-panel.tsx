"use client";

import { startTransition, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { AppSelect } from "@/components/app-select";
import {
  AppSettings,
  Bootstrap,
  BotStatus,
  BotTargetChatCandidate,
  Chat,
} from "@/lib/types";
import {
  describeBotChatCandidate,
  hasAvailableBotToken,
} from "@/lib/bot-target-chat";
import { notifyBootstrapRefresh } from "@/lib/bootstrap-sync";
import { DashboardPage, Surface } from "@/components/dashboard-page";
import { useToast } from "@/components/toast";
import { Button, Field, Input, StatusPill, Textarea } from "@/components/ui";

type SecretPlaceholders = {
  botToken: string;
};

export function BotPanel() {
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const [secretPlaceholders, setSecretPlaceholders] =
    useState<SecretPlaceholders>({ botToken: "" });
  const [botStatus, setBotStatus] = useState<BotStatus | null>(null);
  const [loadingBotStatus, setLoadingBotStatus] = useState(false);
  const [syncingBotCommands, setSyncingBotCommands] = useState(false);
  const [resolvingBotTargetChat, setResolvingBotTargetChat] = useState(false);
  const [savingBotTargetChat, setSavingBotTargetChat] = useState(false);
  const [botTargetChatCandidates, setBotTargetChatCandidates] = useState<
    BotTargetChatCandidate[]
  >([]);
  const [chats, setChats] = useState<Chat[]>([]);
  const toast = useToast();

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    try {
      const [settingsData, bootstrapData, botStatusData, chatsData] = await Promise.all([
        api.settings(),
        api.bootstrap(),
        api.botStatus().catch(() => null),
        api.listChats(),
      ]);
      setSecretPlaceholders({ botToken: settingsData.botToken || "" });
      setSettings({
        ...settingsData,
        botToken: "",
        openAIApiKey: "",
        telegramApiHash: "",
      });
      setBootstrap(bootstrapData);
      setBotStatus(botStatusData);
      setChats(chatsData.map(normalizeChat));
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function save(showToast = true) {
    if (!settings) {
      return false;
    }
    try {
      const saved = await api.saveSettings(settings);
      setSettings((current) =>
        current
          ? {
              ...current,
              botEnabled: saved.botEnabled,
              botTargetChatId: saved.botTargetChatId,
              botToken: "",
            }
          : current,
      );
      setSecretPlaceholders({ botToken: saved.botToken || "" });
      notifyBootstrapRefresh();
      await refreshBotStatus(false);
      if (showToast) {
        toast.showSuccess("Bot 配置已保存。");
      }
      return true;
    } catch (err) {
      toast.showError(asMessage(err));
      return false;
    }
  }

  async function refreshBotStatus(showToast = false) {
    setLoadingBotStatus(true);
    try {
      const status = await api.botStatus();
      setBotStatus(status);
      if (showToast) {
        toast.showSuccess("Bot 状态已刷新。");
      }
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setLoadingBotStatus(false);
    }
  }

  async function syncBotCommands() {
    setSyncingBotCommands(true);
    try {
      const status = await api.syncBotCommands();
      setBotStatus(status);
      toast.showSuccess("Bot 命令菜单已同步。");
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setSyncingBotCommands(false);
    }
  }

  async function resolveBotTargetChat() {
    if (!settings) {
      return;
    }
    if (!bootstrap?.telegramAuthorized) {
      toast.showError("自动获取前请先完成 Telegram 登录。");
      return;
    }
    if (!hasAvailableBotToken(settings.botToken, secretPlaceholders.botToken)) {
      toast.showError("请先填写 Bot Token。");
      return;
    }

    setResolvingBotTargetChat(true);
    try {
      const result = await api.resolveBotTargetChat(settings.botToken);
      setBotTargetChatCandidates(result.candidates);
      if (result.candidates.length === 0) {
        toast.showError("未找到最近消息，请先给 Bot 发一条消息后再重试。");
        return;
      }
      if (result.candidates.length === 1) {
        const [candidate] = result.candidates;
        await saveResolvedBotTargetChat(candidate.chatId);
        return;
      }
      toast.showSuccess("找到了多个可能的会话，请选择一个。");
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setResolvingBotTargetChat(false);
    }
  }

  async function saveResolvedBotTargetChat(chatId: string) {
    if (!settings) {
      return false;
    }

    setSavingBotTargetChat(true);
    try {
      const persistedSettings = await api.settings();
      const nextSettings = {
        ...persistedSettings,
        botEnabled: settings.botEnabled,
        botTargetChatId: chatId,
        botToken: settings.botToken?.trim() || persistedSettings.botToken || "",
      };
      const saved = await api.saveSettings(nextSettings);
      setSettings((current) =>
        current
          ? {
              ...current,
              botEnabled: saved.botEnabled,
              botTargetChatId: saved.botTargetChatId,
              botToken: "",
            }
          : current,
      );
      setSecretPlaceholders({ botToken: saved.botToken || secretPlaceholders.botToken });
      setBotTargetChatCandidates([]);
      notifyBootstrapRefresh();
      await refreshBotStatus(false);
      toast.showSuccess("已自动绑定并保存 Chat ID。");
      return true;
    } catch (err) {
      toast.showError(asMessage(err));
      return false;
    } finally {
      setSavingBotTargetChat(false);
    }
  }

  function patchChat(chatID: number, patch: Partial<Chat>) {
    setChats((current) =>
      current.map((chat) => (chat.id === chatID ? { ...chat, ...patch } : chat)),
    );
  }

  async function saveChat(chat: Chat) {
    try {
      await api.saveChat(chat);
      toast.showSuccess(`已保存「${chat.title}」的 Bot 配置。`);
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  if (!settings) {
    return (
      <DashboardPage
        title="Telegram Bot"
        description="管理 Bot 推送、命令菜单、默认目标会话和运行状态。"
      />
    );
  }

  return (
    <DashboardPage
      title="Telegram Bot"
      description="管理 Bot 推送、命令菜单、默认目标会话和运行状态。群组级 Bot 查询设置仍在群组页面维护。"
    >
      <div className="dashboard-workspace settings-workspace">
        <div className="settings-column">
          <Surface
            title="基础配置"
            description="配置 Bot Token、默认推送会话，以及是否启用 Telegram Bot。"
          >
            <div className="form-grid">
              <Field label="Bot 状态">
                <AppSelect
                  onChange={(value) =>
                    setSettings({ ...settings, botEnabled: value === "yes" })
                  }
                  options={[
                    { value: "no", label: "未启用" },
                    { value: "yes", label: "启用 Bot" },
                  ]}
                  value={settings.botEnabled ? "yes" : "no"}
                />
              </Field>
              <Field
                label="Bot Token"
                hint="已保存时会显示掩码。留空表示保持现有值。"
              >
                <Input
                  placeholder={secretPlaceholder(secretPlaceholders.botToken)}
                  type="password"
                  value={settings.botToken || ""}
                  onChange={(event) => {
                    setBotTargetChatCandidates([]);
                    setSettings({ ...settings, botToken: event.target.value });
                  }}
                />
              </Field>
              <Field
                label="默认目标 Chat ID"
                hint="可直接填写 Chat ID；不知道 ID 时，先给 Bot 发消息再点击自动获取。"
              >
                <Input
                  placeholder="例如 123456789 或 -1001234567890"
                  value={settings.botTargetChatId}
                  onChange={(event) =>
                    setSettings({
                      ...settings,
                      botTargetChatId: event.target.value,
                    })
                  }
                />
              </Field>
            </div>
            <div className="button-row">
              <Button onClick={() => startTransition(() => void save(true))}>
                保存 Bot 配置
              </Button>
            </div>
          </Surface>

          <Surface
            title="Chat ID 获取"
            description="用于绑定默认推送会话。多个群的独立 Bot Chat ID 请到“群组”页面配置。"
          >
            <div className="bot-target-chat-field">
              <p className="muted">
                可以直接在 Bot 私聊或群里发送 /id 查看 Chat ID 和 User ID，再复制到这里；也可以先给 Bot 发一条消息，然后读取最近候选。
              </p>
              {!bootstrap?.telegramAuthorized ? (
                <p className="field-hint">
                  自动获取前需要先在系统配置中完成 Telegram 登录。
                </p>
              ) : null}
              <div className="button-row">
                <Button
                  disabled={
                    resolvingBotTargetChat ||
                    savingBotTargetChat ||
                    !bootstrap?.telegramAuthorized ||
                    !hasAvailableBotToken(
                      settings.botToken,
                      secretPlaceholders.botToken,
                    )
                  }
                  onClick={() => void resolveBotTargetChat()}
                  type="button"
                  variant="secondary"
                >
                  {resolvingBotTargetChat
                    ? "正在获取..."
                    : savingBotTargetChat
                      ? "正在保存..."
                      : "获取最近 Chat ID"}
                </Button>
              </div>
              {botTargetChatCandidates.length > 1 ? (
                <div className="bot-chat-candidates">
                  {botTargetChatCandidates.map((candidate) => (
                    <Button
                      className="bot-chat-candidate"
                      key={candidate.chatId}
                      disabled={savingBotTargetChat}
                      onClick={() => void saveResolvedBotTargetChat(candidate.chatId)}
                      type="button"
                      variant={
                        settings.botTargetChatId === candidate.chatId
                          ? "primary"
                          : "secondary"
                      }
                    >
                      {describeBotChatCandidate(candidate)}
                    </Button>
                  ))}
                </div>
              ) : null}
              <div
                aria-live="polite"
                className={`bot-target-chat-value ${settings.botTargetChatId ? "" : "empty"}`}
              >
                {settings.botTargetChatId
                  ? `当前默认目标：${settings.botTargetChatId}`
                  : "尚未绑定默认目标 Chat ID"}
              </div>
            </div>
          </Surface>
        </div>

        <div className="settings-column">
          <Surface
            title="运行状态"
            description="查看 Bot 身份、命令菜单同步情况和最近轮询状态。"
          >
            <div className="bot-status-panel">
              <div className="bot-status-header">
                <div>
                  <span className="field-label">Bot 状态</span>
                  <p className="field-hint">
                    状态基于已保存的 Bot 配置，修改 Token 后请先保存。
                  </p>
                </div>
                <StatusPill tone={botStatusTone(botStatus, settings)}>
                  {botStatusLabel(botStatus, settings)}
                </StatusPill>
              </div>
              <div className="bot-status-grid">
                <BotStatusItem
                  label="Token"
                  value={botStatus?.tokenConfigured ? "已配置" : "尚未配置"}
                />
                <BotStatusItem
                  label="Bot"
                  value={
                    botStatus?.username
                      ? `@${botStatus.username}`
                      : botStatus?.botId
                        ? String(botStatus.botId)
                        : "未验证"
                  }
                />
                <BotStatusItem
                  label="命令菜单"
                  value={
                    botStatus?.commandsSynced
                      ? "已同步"
                      : botStatus?.tokenConfigured
                        ? "未同步"
                        : "等待 Token"
                  }
                />
                <BotStatusItem
                  label="默认目标"
                  value={botStatus?.targetChatId || "未绑定"}
                />
                <BotStatusItem
                  label="最近轮询"
                  value={formatBotRuntimeTime(botStatus?.lastPollAt)}
                />
                <BotStatusItem
                  label="最近响应"
                  value={formatBotRuntimeTime(botStatus?.lastHandledAt)}
                />
              </div>
              {botStatus?.error || botStatus?.lastError ? (
                <p className="field-hint bot-status-error">
                  {botStatus.error || botStatus.lastError}
                </p>
              ) : null}
              <div className="button-row">
                <Button
                  disabled={loadingBotStatus}
                  onClick={() => void refreshBotStatus(true)}
                  type="button"
                  variant="secondary"
                >
                  {loadingBotStatus ? "正在刷新..." : "刷新状态"}
                </Button>
                <Button
                  disabled={
                    syncingBotCommands ||
                    !botStatus?.enabled ||
                    !botStatus.tokenConfigured
                  }
                  onClick={() => void syncBotCommands()}
                  type="button"
                  variant="secondary"
                >
                  {syncingBotCommands ? "正在同步..." : "同步命令菜单"}
                </Button>
              </div>
            </div>
          </Surface>

          <Surface
            title="群组级 Bot 查询"
            description="按群配置独立 Bot Chat ID、是否允许查询和查询用户白名单。"
          >
            {chats.length === 0 ? (
              <p className="muted">还没有同步到可配置的群组。</p>
            ) : (
              <div className="bot-chat-settings-list">
                {chats.map((chat) => {
                  const status = botChatStatus(chat, hasAvailableBotToken(settings.botToken, secretPlaceholders.botToken));
                  return (
                    <div className="bot-chat-settings-item" key={chat.id}>
                      <div className="bot-chat-settings-head">
                        <div>
                          <strong>{chat.title}</strong>
                          <p className="field-hint">
                            {chat.username ? `@${chat.username}` : chat.chatType}
                          </p>
                        </div>
                        <StatusPill tone={status.tone}>{status.label}</StatusPill>
                      </div>
                      <div className="form-grid">
                        <Field label="Bot Chat ID">
                          <Input
                            placeholder="例如 -1001234567890"
                            value={chat.botChatId}
                            onChange={(event) =>
                              patchChat(chat.id, { botChatId: event.target.value })
                            }
                          />
                        </Field>
                        <Field label="允许 Bot 查询">
                          <AppSelect
                            onChange={(value) =>
                              patchChat(chat.id, {
                                botInteractionEnabled: value === "yes",
                              })
                            }
                            options={[
                              { value: "no", label: "不允许" },
                              { value: "yes", label: "允许" },
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
                          rows={3}
                          placeholder={"@alice\n123456789"}
                          value={joinLines(chat.botAllowedUsers)}
                          onChange={(event) =>
                            patchChat(chat.id, {
                              botAllowedUsers: splitLines(event.target.value),
                            })
                          }
                        />
                      </Field>
                      <div className="button-row">
                        <Button
                          onClick={() => startTransition(() => void saveChat(chat))}
                          type="button"
                          variant="secondary"
                        >
                          保存该群 Bot 配置
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </Surface>
        </div>
      </div>
    </DashboardPage>
  );
}

function normalizeChat(chat: Chat): Chat {
  return {
    ...chat,
    topicGroups: Array.isArray(chat.topicGroups) ? chat.topicGroups : [],
    filteredKeywords: Array.isArray(chat.filteredKeywords) ? chat.filteredKeywords : [],
    filteredSenders: Array.isArray(chat.filteredSenders) ? chat.filteredSenders : [],
    keepBotMessages: chat.keepBotMessages ?? true,
    botChatId: chat.botChatId ?? "",
    botInteractionEnabled: chat.botInteractionEnabled ?? false,
    botAllowedUsers: Array.isArray(chat.botAllowedUsers) ? chat.botAllowedUsers : [],
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

function botChatStatus(
  chat: Chat,
  botTokenAvailable: boolean,
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

function BotStatusItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="bot-status-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatBotRuntimeTime(value?: string | null) {
  if (!value) {
    return "暂无";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "暂无";
  }
  return date.toLocaleString();
}

function botStatusTone(
  status: BotStatus | null,
  settings: AppSettings,
): "neutral" | "good" | "warn" | "bad" {
  if (!settings.botEnabled) {
    return "neutral";
  }
  if (!status || !status.tokenConfigured || !status.targetChatId) {
    return "warn";
  }
  if (status.error || status.lastError) {
    return "bad";
  }
  return status.commandsSynced ? "good" : "warn";
}

function botStatusLabel(status: BotStatus | null, settings: AppSettings) {
  if (!settings.botEnabled) {
    return "未启用";
  }
  if (!status) {
    return "未检查";
  }
  if (!status.tokenConfigured) {
    return "缺少 Token";
  }
  if (status.error || status.lastError) {
    return "验证失败";
  }
  if (!status.targetChatId) {
    return "未绑定";
  }
  return status.commandsSynced ? "就绪" : "待同步";
}

function secretPlaceholder(value: string) {
  return value || "留空表示保持现有值";
}

function asMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}
