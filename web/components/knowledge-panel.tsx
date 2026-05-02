"use client";

import {
  startTransition,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { api } from "@/lib/api";
import { AppSelect } from "@/components/app-select";
import {
  DashboardPage,
  EmptyState,
  MetricCard,
  MetricRail,
  Surface,
} from "@/components/dashboard-page";
import { Modal } from "@/components/modal";
import { SummaryMarkdown } from "@/components/summary-markdown";
import { useToast } from "@/components/toast";
import { Button, Field, Input, StatusPill, Textarea } from "@/components/ui";
import {
  Chat,
  KnowledgeFact,
  KnowledgeQueryResult,
  KnowledgeRun,
  KnowledgeSpace,
  KnowledgeSubject,
} from "@/lib/types";

type FactStatusFilter = "all" | KnowledgeFact["status"];
type KnowledgeTemplateKey = "marketplace" | "hiring" | "skills" | "events" | "blank";

type KnowledgeSpaceTemplate = {
  key: KnowledgeTemplateKey;
  label: string;
  name: string;
  description: string;
  schemaJson: string;
  extractPrompt: string;
  summaryPrompt: string;
  confidenceThreshold: number;
  retentionDays: number;
};

type KnowledgeSpaceExport = {
  kind: "tgtldr.knowledge-space";
  version: 1;
  space: {
    name: string;
    description: string;
    schema: unknown;
    extractPrompt: string;
    summaryPrompt: string;
    confidenceThreshold: number;
    retentionDays: number;
    includeInSummary: boolean;
  };
};

const defaultTemplateKey: KnowledgeTemplateKey = "marketplace";

const knowledgeSpaceTemplates: KnowledgeSpaceTemplate[] = [
  {
    key: "marketplace",
    label: "供需",
    name: "供需频道",
    description: "从群消息中识别需求、供应和可匹配线索。",
    schemaJson: schemaString({
      types: {
        demand: {
          label: "需求",
          fields: {
            item: "string",
            quantity: "string",
            budget: "string",
            location: "string",
            deadline: "string",
          },
        },
        supply: {
          label: "供应",
          fields: {
            item: "string",
            quantity: "string",
            price: "string",
            location: "string",
          },
        },
      },
    }),
    extractPrompt: "优先抽取明确表达买、卖、求购、出售、转让、拼单、采购的信息。",
    summaryPrompt: "摘要附加时按需求和供应分组，并保留可联系用户。",
    confidenceThreshold: 0.75,
    retentionDays: 30,
  },
  {
    key: "hiring",
    label: "招聘",
    name: "招聘线索",
    description: "记录招聘、求职、内推和可联系候选人。",
    schemaJson: schemaString({
      types: {
        hiring: {
          label: "招聘",
          fields: {
            role: "string",
            company: "string",
            location: "string",
            salary: "string",
            requirements: "string",
          },
        },
        candidate: {
          label: "求职",
          fields: {
            role: "string",
            skills: "string",
            location: "string",
            availability: "string",
          },
        },
        referral: {
          label: "内推",
          fields: {
            company: "string",
            role: "string",
            contact: "string",
          },
        },
      },
    }),
    extractPrompt: "只抽取明确招聘、求职、内推、接项目的信息；闲聊和泛泛讨论不要记录。",
    summaryPrompt: "摘要附加时突出岗位、候选人方向、地点和可联系用户。",
    confidenceThreshold: 0.75,
    retentionDays: 45,
  },
  {
    key: "skills",
    label: "技能画像",
    name: "技能画像",
    description: "记录用户擅长领域、可提供帮助和正在学习的方向。",
    schemaJson: schemaString({
      types: {
        skill: {
          label: "擅长",
          fields: {
            area: "string",
            evidence: "string",
            level: "string",
          },
        },
        help_offer: {
          label: "可帮助",
          fields: {
            topic: "string",
            availability: "string",
            condition: "string",
          },
        },
        interest: {
          label: "关注方向",
          fields: {
            topic: "string",
            goal: "string",
          },
        },
      },
    }),
    extractPrompt: "根据用户自己表达的经验、作品、回答质量或明确承诺记录，不要凭单句闲聊推断技能。",
    summaryPrompt: "摘要附加时按用户列出擅长方向和可提供帮助的主题。",
    confidenceThreshold: 0.8,
    retentionDays: 90,
  },
  {
    key: "events",
    label: "活动",
    name: "活动报名",
    description: "记录活动、报名、参与意向和资源支持。",
    schemaJson: schemaString({
      types: {
        event: {
          label: "活动",
          fields: {
            name: "string",
            time: "string",
            location: "string",
            topic: "string",
          },
        },
        registration: {
          label: "报名",
          fields: {
            event: "string",
            role: "string",
            note: "string",
          },
        },
        resource: {
          label: "资源",
          fields: {
            event: "string",
            resource: "string",
            condition: "string",
          },
        },
      },
    }),
    extractPrompt: "只记录明确的活动安排、报名意向、资源支持或组织协作信息。",
    summaryPrompt: "摘要附加时列出活动、已报名用户和可用资源。",
    confidenceThreshold: 0.75,
    retentionDays: 60,
  },
  {
    key: "blank",
    label: "空白",
    name: "自定义知识空间",
    description: "按你的群聊场景自定义事实类型和字段。",
    schemaJson: schemaString({
      types: {
        custom_fact: {
          label: "自定义事实",
          fields: {
            topic: "string",
            detail: "string",
          },
        },
      },
    }),
    extractPrompt: "",
    summaryPrompt: "",
    confidenceThreshold: 0.75,
    retentionDays: 30,
  },
];

export function KnowledgePanel() {
  const [spaces, setSpaces] = useState<KnowledgeSpace[]>([]);
  const [facts, setFacts] = useState<KnowledgeFact[]>([]);
  const [subjects, setSubjects] = useState<KnowledgeSubject[]>([]);
  const [runs, setRuns] = useState<KnowledgeRun[]>([]);
  const [chats, setChats] = useState<Chat[]>([]);
  const [editing, setEditing] = useState<KnowledgeSpace | null>(null);
  const [knowledgeQueryPreview, setKnowledgeQueryPreview] =
    useState<KnowledgeQueryResult | null>(null);
  const [previewingKnowledgeQuery, setPreviewingKnowledgeQuery] = useState(false);
  const [sendingKnowledgeQuery, setSendingKnowledgeQuery] = useState(false);
  const [selectedSpaceId, setSelectedSpaceId] = useState<number | "all">("all");
  const [statusFilter, setStatusFilter] = useState<FactStatusFilter>("all");
  const [factChatId, setFactChatId] = useState<number | "all">("all");
  const [factTypeFilter, setFactTypeFilter] = useState("");
  const [factQuery, setFactQuery] = useState("");
  const [runChatId, setRunChatId] = useState<number | "">("");
  const [runDate, setRunDate] = useState(localDateInputValue());
  const deferredFactQuery = useDeferredValue(factQuery);
  const deferredFactTypeFilter = useDeferredValue(factTypeFilter);
  const importInputRef = useRef<HTMLInputElement | null>(null);
  const toast = useToast();

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    void loadFacts();
  }, [selectedSpaceId, statusFilter, factChatId, deferredFactQuery, deferredFactTypeFilter]);

  useEffect(() => {
    void loadSubjects();
  }, [selectedSpaceId, factChatId, deferredFactQuery, deferredFactTypeFilter]);

  useEffect(() => {
    void loadRuns();
  }, [selectedSpaceId]);

  async function load() {
    try {
      const [spaceItems, chatItems] = await Promise.all([
        api.listKnowledgeSpaces(),
        api.listChats(),
      ]);
      setSpaces(spaceItems.map(normalizeSpace));
      setChats(chatItems);
      setEditing((current) => current ?? (spaceItems[0] ? normalizeSpace(spaceItems[0]) : null));
      await Promise.all([loadFacts(), loadSubjects(), loadRuns()]);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function loadFacts(spaceId: number | "all" = selectedSpaceId) {
    try {
      const items = await api.listKnowledgeFacts({
        q: deferredFactQuery,
        spaceId: spaceId === "all" ? undefined : spaceId,
        chatId: factChatId === "all" ? undefined : factChatId,
        status: statusFilter,
        factType: deferredFactTypeFilter,
        limit: 100,
      });
      setFacts(items);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function loadSubjects(spaceId: number | "all" = selectedSpaceId) {
    try {
      const items = await api.listKnowledgeSubjects({
        q: deferredFactQuery,
        spaceId: spaceId === "all" ? undefined : spaceId,
        chatId: factChatId === "all" ? undefined : factChatId,
        factType: deferredFactTypeFilter,
        limit: 50,
      });
      setSubjects(items);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function loadRuns(spaceId: number | "all" = selectedSpaceId) {
    try {
      const items = await api.listKnowledgeRuns({
        spaceId: spaceId === "all" ? undefined : spaceId,
        limit: 20,
      });
      setRuns(items);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function saveCurrent() {
    if (!editing) {
      return;
    }
    const validationError = validateSpace(editing);
    if (validationError) {
      toast.showError(validationError);
      return;
    }

    try {
      const saved =
        editing.id === 0
          ? await api.createKnowledgeSpace(editing)
          : await api.saveKnowledgeSpace(editing);
      toast.showSuccess(`已保存知识空间「${saved.name}」。`);
      setEditing(normalizeSpace(saved));
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function runExtraction() {
    if (!editing || editing.id === 0 || !runChatId || !runDate) {
      toast.showError("请先保存知识空间，并选择群组和日期。");
      return;
    }
    try {
      const run = await api.runKnowledgeExtraction(editing.id, runChatId, runDate);
      if (run.status === "failed") {
        toast.showError(run.errorMessage || "知识抽取失败。");
      } else {
        toast.showSuccess(`知识抽取完成：读取 ${run.inputMessageCount} 条消息，写入 ${run.extractedCount} 条事实。`);
      }
      setSelectedSpaceId(editing.id);
      await Promise.all([loadFacts(editing.id), loadSubjects(editing.id), loadRuns(editing.id)]);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function updateFactStatus(fact: KnowledgeFact, status: KnowledgeFact["status"]) {
    try {
      await api.updateKnowledgeFactStatus(fact.id, status);
      toast.showSuccess(status === "active" ? "已恢复这条事实。" : "已忽略这条事实。");
      await Promise.all([loadFacts(), loadSubjects()]);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function exportCurrentSpace() {
    if (!editing) {
      return;
    }
    const validationError = validateSpace(editing);
    if (validationError) {
      toast.showError(validationError);
      return;
    }
    downloadKnowledgeSpaceConfig(editing);
    toast.showSuccess("知识空间配置已导出。");
  }

  async function importKnowledgeSpaceConfig(file: File | undefined) {
    if (!file) {
      return;
    }
    try {
      const imported = parseKnowledgeSpaceConfig(await file.text(), editing ?? newKnowledgeSpace());
      setEditing(imported);
      toast.showSuccess(`已导入知识空间配置「${imported.name}」。保存后生效。`);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      if (importInputRef.current) {
        importInputRef.current.value = "";
      }
    }
  }

  function currentKnowledgeQueryFilters() {
    return {
      q: deferredFactQuery,
      spaceId: selectedSpaceId === "all" ? undefined : selectedSpaceId,
      chatId: factChatId === "all" ? undefined : factChatId,
      factType: deferredFactTypeFilter,
      limit: 20,
    };
  }

  async function previewKnowledgeQuery() {
    setPreviewingKnowledgeQuery(true);
    try {
      const result = await api.renderKnowledgeQuery(currentKnowledgeQueryFilters());
      setKnowledgeQueryPreview(result);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setPreviewingKnowledgeQuery(false);
    }
  }

  async function sendKnowledgeQueryToBot() {
    setSendingKnowledgeQuery(true);
    try {
      const result = await api.sendKnowledgeQuery(currentKnowledgeQueryFilters());
      toast.showSuccess(result.message || "知识查询结果已发送。");
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setSendingKnowledgeQuery(false);
    }
  }

  const activeCount = spaces.filter((space) => space.enabled).length;
  const includedCount = spaces.filter((space) => space.includeInSummary).length;
  const activeFacts = facts.filter((fact) => fact.status === "active").length;
  const selectedSpace = useMemo(
    () => spaces.find((space) => space.id === selectedSpaceId) ?? null,
    [selectedSpaceId, spaces],
  );
  const chatTitleByID = useMemo(
    () => new Map(chats.map((chat) => [chat.id, chat.title])),
    [chats],
  );
  const spaceNameByID = useMemo(
    () => new Map(spaces.map((space) => [space.id, space.name])),
    [spaces],
  );

  return (
    <DashboardPage
      title="知识空间"
      description="为不同群组配置长期知识抽取规则，管理自动抽取出的事实。"
      actions={
        <Button onClick={() => setEditing(newKnowledgeSpace())} type="button">
          新建知识空间
        </Button>
      }
    >
      <MetricRail>
        <MetricCard
          label="知识空间"
          value={spaces.length}
          badge={activeCount > 0 ? "已启用" : "未启用"}
          tone={activeCount > 0 ? "good" : "neutral"}
          detail={`${activeCount} 个启用中。`}
        />
        <MetricCard
          label="摘要附加"
          value={includedCount}
          badge="配置项"
          detail="开启后，后续摘要可附加结构化事实。"
        />
        <MetricCard
          label="当前事实"
          value={facts.length}
          badge={`${activeFacts} active`}
          tone={activeFacts > 0 ? "good" : "neutral"}
          detail="展示最近的结构化事实记录。"
        />
        <MetricCard
          label="用户画像"
          value={subjects.length}
          badge="active"
          tone={subjects.length > 0 ? "good" : "neutral"}
          detail="按用户聚合仍有效的知识事实。"
        />
      </MetricRail>

      <div className="settings-workspace">
        <Surface
          title="空间配置"
          description="schema 使用 JSON 保存。供需、招聘、活动等场景都走同一套结构。"
          actions={
            editing ? (
              <>
                <input
                  accept="application/json,.json"
                  className="sr-only"
                  onChange={(event) =>
                    startTransition(() =>
                      void importKnowledgeSpaceConfig(event.currentTarget.files?.[0]),
                    )
                  }
                  ref={importInputRef}
                  type="file"
                />
                <Button
                  onClick={() => importInputRef.current?.click()}
                  type="button"
                  variant="ghost"
                >
                  导入配置
                </Button>
                <Button onClick={exportCurrentSpace} type="button" variant="secondary">
                  导出配置
                </Button>
              </>
            ) : null
          }
        >
          {editing ? (
            <div className="form-stack">
              <div className="form-grid">
                <Field
                  label="模板"
                  hint="套用后仍可继续编辑 schema 和提示词。"
                >
                  <AppSelect
                    onChange={(value) => {
                      if (!value) {
                        return;
                      }
                      setEditing(applyKnowledgeTemplate(editing, value as KnowledgeTemplateKey));
                    }}
                    options={[
                      { value: "", label: "选择模板" },
                      ...knowledgeSpaceTemplates.map((template) => ({
                        value: template.key,
                        label: template.label,
                      })),
                    ]}
                    value=""
                  />
                </Field>
                <Field label="名称" required>
                  <Input
                    value={editing.name}
                    onChange={(event) =>
                      setEditing({ ...editing, name: event.target.value })
                    }
                  />
                </Field>
                <Field label="启用状态">
                  <AppSelect
                    onChange={(value) =>
                      setEditing({ ...editing, enabled: value === "yes" })
                    }
                    options={[
                      { value: "yes", label: "启用" },
                      { value: "no", label: "停用" },
                    ]}
                    value={editing.enabled ? "yes" : "no"}
                  />
                </Field>
                <Field label="置信度阈值">
                  <Input
                    min={0}
                    max={1}
                    step={0.05}
                    type="number"
                    value={editing.confidenceThreshold}
                    onChange={(event) =>
                      setEditing({
                        ...editing,
                        confidenceThreshold: Number(event.target.value),
                      })
                    }
                  />
                </Field>
                <Field label="默认保留天数" hint="过期事实会保留记录，但不会附加到后续摘要。">
                  <Input
                    min={1}
                    type="number"
                    value={editing.retentionDays}
                    onChange={(event) =>
                      setEditing({
                        ...editing,
                        retentionDays: Number(event.target.value),
                      })
                    }
                  />
                </Field>
              </div>

              <Field label="描述">
                <Textarea
                  rows={3}
                  value={editing.description}
                  onChange={(event) =>
                    setEditing({ ...editing, description: event.target.value })
                  }
                />
              </Field>

              <Field as="div" label="适用群组">
                <div className="knowledge-chat-list">
                  {chats.length === 0 ? (
                    <p className="muted">暂无群组，请先同步 Telegram 群组。</p>
                  ) : (
                    chats.map((chat) => (
                      <label className="knowledge-chat-option" key={chat.id}>
                        <input
                          checked={editing.chatIds.includes(chat.id)}
                          onChange={(event) =>
                            setEditing({
                              ...editing,
                              chatIds: event.target.checked
                                ? [...editing.chatIds, chat.id]
                                : editing.chatIds.filter((id) => id !== chat.id),
                            })
                          }
                          type="checkbox"
                        />
                        <span>{chat.title}</span>
                      </label>
                    ))
                  )}
                </div>
              </Field>

              <Field
                label="抽取 schema"
                hint="必须是合法 JSON。抽取时会按该 schema 输出结构化事实。"
              >
                <Textarea
                  rows={14}
                  value={editing.schemaJson}
                  onChange={(event) =>
                    setEditing({ ...editing, schemaJson: event.target.value })
                  }
                />
              </Field>

              <Field label="抽取额外要求">
                <Textarea
                  rows={5}
                  value={editing.extractPrompt}
                  onChange={(event) =>
                    setEditing({ ...editing, extractPrompt: event.target.value })
                  }
                />
              </Field>

              <div className="form-grid">
                <Field label="摘要附加">
                  <AppSelect
                    onChange={(value) =>
                      setEditing({
                        ...editing,
                        includeInSummary: value === "yes",
                      })
                    }
                    options={[
                      { value: "yes", label: "附加到摘要" },
                      { value: "no", label: "仅保存事实" },
                    ]}
                    value={editing.includeInSummary ? "yes" : "no"}
                  />
                </Field>
              </div>

              <Field label="摘要展示要求">
                <Textarea
                  rows={4}
                  value={editing.summaryPrompt}
                  onChange={(event) =>
                    setEditing({ ...editing, summaryPrompt: event.target.value })
                  }
                />
              </Field>

              <div className="editor-footer">
                <p className="muted">
                  {editing.id === 0 ? "新建后会进入列表。" : `正在编辑 ID ${editing.id}`}
                </p>
                <Button onClick={() => startTransition(() => void saveCurrent())} type="button">
                  保存知识空间
                </Button>
              </div>

              {editing.id !== 0 ? (
                <div className="knowledge-run-panel">
                  <div>
                    <strong>手动抽取</strong>
                    <p className="muted">按所选日期读取该群消息，并写入结构化事实。</p>
                  </div>
                  <div className="form-grid">
                    <Field label="群组">
                      <AppSelect
                        onChange={(value) => setRunChatId(value ? Number(value) : "")}
                        options={[
                          { value: "", label: "选择群组" },
                          ...chats
                            .filter(
                              (chat) =>
                                editing.chatIds.length === 0 ||
                                editing.chatIds.includes(chat.id),
                            )
                            .map((chat) => ({
                              value: String(chat.id),
                              label: chat.title,
                            })),
                        ]}
                        value={String(runChatId)}
                      />
                    </Field>
                    <Field label="日期">
                      <Input
                        type="date"
                        value={runDate}
                        onChange={(event) => setRunDate(event.target.value)}
                      />
                    </Field>
                  </div>
                  <Button
                    disabled={!runChatId || !runDate}
                    onClick={() => startTransition(() => void runExtraction())}
                    type="button"
                    variant="secondary"
                  >
                    运行抽取
                  </Button>
                </div>
              ) : null}
            </div>
          ) : (
            <EmptyState title="暂无知识空间" description="创建一个空间后再配置抽取规则。" />
          )}
        </Surface>

        <Surface
          title="空间列表"
          description="选择一个空间后，右侧事实列表会按该空间过滤。"
        >
          {spaces.length === 0 ? (
            <EmptyState title="还没有知识空间" description="先创建一个供后续抽取引擎使用。" />
          ) : (
            <div className="knowledge-space-list">
              {spaces.map((space) => (
                <button
                  className={
                    editing?.id === space.id
                      ? "knowledge-space-item active"
                      : "knowledge-space-item"
                  }
                  key={space.id}
                  onClick={() => {
                    setEditing(normalizeSpace(space));
                    setSelectedSpaceId(space.id);
                  }}
                  type="button"
                >
                  <span>{space.name}</span>
                  <StatusPill tone={space.enabled ? "good" : "neutral"}>
                    {space.enabled ? "启用" : "停用"}
                  </StatusPill>
                </button>
              ))}
            </div>
          )}
        </Surface>
      </div>

      <Surface title="抽取记录" description="展示最近的手动和自动抽取结果。">
        {runs.length === 0 ? (
          <EmptyState title="暂无抽取记录" description="运行抽取或生成摘要后会写入记录。" />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>知识空间</th>
                  <th>群组</th>
                  <th>范围</th>
                  <th>状态</th>
                  <th>消息</th>
                  <th>事实</th>
                  <th>完成时间</th>
                  <th>错误</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((run) => (
                  <tr className="data-row" key={run.id}>
                    <td>{spaceNameByID.get(run.spaceId) ?? run.spaceId}</td>
                    <td>{chatTitleByID.get(run.chatId) ?? run.chatId}</td>
                    <td>{formatRunRange(run)}</td>
                    <td>
                      <StatusPill tone={runStatusTone(run.status)}>{run.status}</StatusPill>
                    </td>
                    <td>{run.inputMessageCount}</td>
                    <td>{run.extractedCount}</td>
                    <td>{formatDateTime(run.finishedAt ?? run.updatedAt)}</td>
                    <td>
                      <span className="muted">{run.errorMessage || "无"}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Surface>

      <Surface
        title="用户画像"
        description="按用户聚合 active 事实，后续查询机器人可复用同一类结果。"
        actions={
          <>
            <Button
              disabled={previewingKnowledgeQuery}
              onClick={() => startTransition(() => void previewKnowledgeQuery())}
              type="button"
              variant="ghost"
            >
              {previewingKnowledgeQuery ? "正在预览..." : "预览"}
            </Button>
            <Button
              disabled={sendingKnowledgeQuery}
              onClick={() => startTransition(() => void sendKnowledgeQueryToBot())}
              type="button"
              variant="secondary"
            >
              {sendingKnowledgeQuery ? "发送中..." : "发送到 Bot"}
            </Button>
          </>
        }
      >
        {subjects.length === 0 ? (
          <EmptyState title="暂无用户画像" description="有带用户信息的 active 事实后会在这里聚合展示。" />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>用户</th>
                  <th>事实数</th>
                  <th>类型</th>
                  <th>群组</th>
                  <th>最近发现</th>
                  <th>代表事实</th>
                </tr>
              </thead>
              <tbody>
                {subjects.map((subject) => (
                  <tr className="data-row" key={subject.key}>
                    <td>{subject.displayName || "未记录"}</td>
                    <td>{subject.factCount}</td>
                    <td>{formatList(subject.factTypes)}</td>
                    <td>{formatList(subject.chatTitles)}</td>
                    <td>{formatDateTime(subject.lastSeenAt)}</td>
                    <td>
                      <div className="data-row-title">
                        <span>{formatSubjectFactTitles(subject)}</span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Surface>

      <Surface
        title="事实列表"
        description={
          selectedSpace
            ? `当前过滤：${selectedSpace.name}，默认保留 ${selectedSpace.retentionDays} 天。`
            : "展示最近的结构化事实。"
        }
      >
        <div className="toolbar-grid">
          <Field label="搜索事实">
            <Input
              onChange={(event) => setFactQuery(event.target.value)}
              placeholder="商品、用户、地点"
              value={factQuery}
            />
          </Field>
          <Field label="知识空间">
            <AppSelect
              onChange={(value) =>
                setSelectedSpaceId(value === "all" ? "all" : Number(value))
              }
              options={[
                { value: "all", label: "全部" },
                ...spaces.map((space) => ({
                  value: String(space.id),
                  label: space.name,
                })),
              ]}
              value={String(selectedSpaceId)}
            />
          </Field>
          <Field label="状态">
            <AppSelect
              onChange={(value) => setStatusFilter(value as FactStatusFilter)}
              options={[
                { value: "all", label: "全部" },
                { value: "active", label: "Active" },
                { value: "expired", label: "Expired" },
                { value: "dismissed", label: "Dismissed" },
              ]}
              value={statusFilter}
            />
          </Field>
          <Field label="类型">
            <Input
              onChange={(event) => setFactTypeFilter(event.target.value)}
              placeholder="demand / supply"
              value={factTypeFilter}
            />
          </Field>
          <Field label="群组">
            <AppSelect
              onChange={(value) =>
                setFactChatId(value === "all" ? "all" : Number(value))
              }
              options={[
                { value: "all", label: "全部群组" },
                ...chats.map((chat) => ({
                  value: String(chat.id),
                  label: chat.title,
                })),
              ]}
              value={String(factChatId)}
            />
          </Field>
        </div>

        {facts.length === 0 ? (
          <EmptyState title="暂无事实" description="运行抽取或生成摘要后，会在这里展示结构化事实。" />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>事实</th>
                  <th>类型</th>
                  <th>群组</th>
                  <th>用户</th>
                  <th>置信度</th>
                  <th>最近发现</th>
                  <th>过期时间</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {facts.map((fact) => (
                  <tr className="data-row" key={fact.id}>
                    <td>
                      <div className="data-row-title">
                        <strong>{fact.title}</strong>
                        <span>{fact.dataJson}</span>
                      </div>
                    </td>
                    <td>{fact.factType}</td>
                    <td>{fact.chatTitle || fact.chatId}</td>
                    <td>{formatSubject(fact)}</td>
                    <td>{Math.round(fact.confidence * 100)}%</td>
                    <td>{formatDateTime(fact.lastSeenAt)}</td>
                    <td>{formatFactExpiry(fact)}</td>
                    <td>
                      <StatusPill tone={factStatusTone(fact.status)}>
                        {fact.status}
                      </StatusPill>
                    </td>
                    <td>
                      <div className="table-row-actions">
                        {fact.status === "active" ? (
                          <button
                            className="text-link-button"
                            onClick={() =>
                              startTransition(() =>
                                void updateFactStatus(fact, "dismissed"),
                              )
                            }
                            type="button"
                          >
                            忽略
                          </button>
                        ) : (
                          <button
                            className="text-link-button"
                            onClick={() =>
                              startTransition(() => void updateFactStatus(fact, "active"))
                            }
                            type="button"
                          >
                            恢复
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Surface>

      <Modal
        actions={
          <>
            <Button
              disabled={sendingKnowledgeQuery}
              onClick={() => startTransition(() => void sendKnowledgeQueryToBot())}
              type="button"
              variant="secondary"
            >
              {sendingKnowledgeQuery ? "发送中..." : "发送到 Bot"}
            </Button>
            <Button onClick={() => setKnowledgeQueryPreview(null)} type="button">
              关闭
            </Button>
          </>
        }
        description={
          knowledgeQueryPreview
            ? formatKnowledgeQueryPreviewDescription(knowledgeQueryPreview)
            : undefined
        }
        onClose={() => setKnowledgeQueryPreview(null)}
        open={knowledgeQueryPreview !== null}
        title="知识查询预览"
      >
        {knowledgeQueryPreview ? (
          <SummaryMarkdown content={knowledgeQueryPreview.content} />
        ) : null}
      </Modal>
    </DashboardPage>
  );
}

function newKnowledgeSpace(): KnowledgeSpace {
  return applyKnowledgeTemplate({
    id: 0,
    name: "",
    description: "",
    enabled: true,
    chatIds: [],
    schemaJson: "{}",
    extractPrompt: "",
    summaryPrompt: "",
    confidenceThreshold: 0.75,
    retentionDays: 30,
    includeInSummary: true,
    createdAt: "",
    updatedAt: "",
  }, defaultTemplateKey);
}

function applyKnowledgeTemplate(
  space: KnowledgeSpace,
  templateKey: KnowledgeTemplateKey,
): KnowledgeSpace {
  const template =
    knowledgeSpaceTemplates.find((item) => item.key === templateKey) ??
    knowledgeSpaceTemplates[0];
  return {
    ...space,
    name: template.name,
    description: template.description,
    schemaJson: template.schemaJson,
    extractPrompt: template.extractPrompt,
    summaryPrompt: template.summaryPrompt,
    confidenceThreshold: template.confidenceThreshold,
    retentionDays: template.retentionDays,
  };
}

function schemaString(value: unknown) {
  return JSON.stringify(value, null, 2);
}

function downloadKnowledgeSpaceConfig(space: KnowledgeSpace) {
  const payload: KnowledgeSpaceExport = {
    kind: "tgtldr.knowledge-space",
    version: 1,
    space: {
      name: space.name.trim(),
      description: space.description,
      schema: JSON.parse(space.schemaJson),
      extractPrompt: space.extractPrompt,
      summaryPrompt: space.summaryPrompt,
      confidenceThreshold: space.confidenceThreshold,
      retentionDays: space.retentionDays,
      includeInSummary: space.includeInSummary,
    },
  };
  const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], {
    type: "application/json",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${safeFileName(space.name || "knowledge-space")}.json`;
  link.click();
  URL.revokeObjectURL(url);
}

function parseKnowledgeSpaceConfig(content: string, base: KnowledgeSpace): KnowledgeSpace {
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    throw new Error("导入文件必须是合法 JSON。");
  }
  if (!isRecord(parsed)) {
    throw new Error("导入文件格式不正确。");
  }

  const source = isRecord(parsed.space) ? parsed.space : parsed;
  const schema = readImportedSchema(source);
  const imported: KnowledgeSpace = {
    ...base,
    name: readImportedString(source.name, "未命名知识空间"),
    description: readImportedString(source.description, ""),
    schemaJson: schemaString(schema),
    extractPrompt: readImportedString(source.extractPrompt, ""),
    summaryPrompt: readImportedString(source.summaryPrompt, ""),
    confidenceThreshold: readImportedNumber(source.confidenceThreshold, 0.75),
    retentionDays: readImportedNumber(source.retentionDays, 30),
    includeInSummary: readImportedBoolean(source.includeInSummary, base.includeInSummary),
  };
  const validationError = validateSpace(imported);
  if (validationError) {
    throw new Error(validationError);
  }
  return imported;
}

function readImportedSchema(source: Record<string, unknown>) {
  if (source.schema !== undefined) {
    return source.schema;
  }
  if (typeof source.schemaJson === "string") {
    try {
      return JSON.parse(source.schemaJson);
    } catch {
      throw new Error("导入文件中的 schemaJson 必须是合法 JSON。");
    }
  }
  throw new Error("导入文件缺少 schema。");
}

function readImportedString(value: unknown, fallback: string) {
  return typeof value === "string" ? value : fallback;
}

function readImportedNumber(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function readImportedBoolean(value: unknown, fallback: boolean) {
  return typeof value === "boolean" ? value : fallback;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function safeFileName(value: string) {
  const normalized = value.trim().toLowerCase().replace(/[^a-z0-9._-]+/g, "-");
  return normalized.replace(/^-+|-+$/g, "") || "knowledge-space";
}

function normalizeSpace(space: KnowledgeSpace): KnowledgeSpace {
  return {
    ...space,
    chatIds: Array.isArray(space.chatIds) ? space.chatIds : [],
    schemaJson: space.schemaJson?.trim() || "{}",
    confidenceThreshold: space.confidenceThreshold || 0.75,
    retentionDays: space.retentionDays || 30,
  };
}

function validateSpace(space: KnowledgeSpace) {
  if (!space.name.trim()) {
    return "请填写知识空间名称。";
  }
  try {
    JSON.parse(space.schemaJson);
  } catch {
    return "抽取 schema 必须是合法 JSON。";
  }
  if (space.confidenceThreshold <= 0 || space.confidenceThreshold > 1) {
    return "置信度阈值必须在 0 到 1 之间。";
  }
  if (space.retentionDays <= 0) {
    return "默认保留天数必须大于 0。";
  }
  return "";
}

function formatSubject(fact: KnowledgeFact) {
  if (fact.subjectUsername) {
    return `@${fact.subjectUsername}`;
  }
  if (fact.subjectSenderName) {
    return fact.subjectSenderName;
  }
  if (fact.subjectSenderId) {
    return String(fact.subjectSenderId);
  }
  return "未记录";
}

function formatRunRange(run: KnowledgeRun) {
  return `${formatDateTime(run.rangeStart)} - ${formatDateTime(run.rangeEnd)}`;
}

function formatList(values: string[]) {
  if (values.length === 0) {
    return "未记录";
  }
  return values.join("、");
}

function formatSubjectFactTitles(subject: KnowledgeSubject) {
  const titles = subject.facts.slice(0, 3).map((fact) => fact.title).filter(Boolean);
  if (titles.length === 0) {
    return "未记录";
  }
  const suffix = subject.factCount > titles.length ? ` 等 ${subject.factCount} 条` : "";
  return `${titles.join("；")}${suffix}`;
}

function formatDateTime(value?: string) {
  if (!value) {
    return "未完成";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatFactExpiry(fact: KnowledgeFact) {
  if (!fact.expiresAt) {
    return "长期保留";
  }
  return formatDateTime(fact.expiresAt);
}

function formatKnowledgeQueryPreviewDescription(result: KnowledgeQueryResult) {
  return `匹配 ${result.facts.length} 条事实，关联 ${result.subjects.length} 个用户。`;
}

function factStatusTone(status: KnowledgeFact["status"]) {
  switch (status) {
    case "active":
      return "good";
    case "expired":
      return "warn";
    default:
      return "neutral";
  }
}

function runStatusTone(status: KnowledgeRun["status"]) {
  switch (status) {
    case "succeeded":
      return "good";
    case "failed":
      return "bad";
    case "running":
    case "pending":
      return "warn";
    default:
      return "neutral";
  }
}

function localDateInputValue() {
  const now = new Date();
  const offset = now.getTimezoneOffset() * 60 * 1000;
  return new Date(now.getTime() - offset).toISOString().slice(0, 10);
}

function asMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}
