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
  KnowledgeFactSources,
  KnowledgeMaintenanceEvent,
  KnowledgeMaintenanceResult,
  KnowledgeQueryResult,
  KnowledgeRun,
  KnowledgeSpace,
  KnowledgeSubject,
  Message,
} from "@/lib/types";

type FactStatusFilter = "all" | KnowledgeFact["status"];
type KnowledgeTemplateKey =
  | "general"
  | "marketplace"
  | "hiring"
  | "skills"
  | "events"
  | "risk_accounts"
  | "blank";

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

const defaultTemplateKey: KnowledgeTemplateKey = "general";

const defaultManualFactType = "skill";

type ManualFactMode = "skill" | "demand" | "supply" | "risk_account" | "custom";

type ManualFactDraft = {
  mode: ManualFactMode;
  person: string;
  username: string;
  title: string;
  detail: string;
};

type CorrectionDraft = {
  fact: KnowledgeFact;
  wrong: string;
  replacement: string;
};

const manualFactModeOptions: Array<{
  mode: ManualFactMode;
  label: string;
  hint: string;
}> = [
  { mode: "skill", label: "记录技能", hint: "例如：Alice 会 Rust" },
  { mode: "demand", label: "记录需求", hint: "例如：Bob 需要 Gmail 账号" },
  { mode: "supply", label: "记录供应", hint: "例如：Carol 出售 API" },
  { mode: "risk_account", label: "记录风险账号", hint: "只用于明确曝光或举报" },
  { mode: "custom", label: "高级自定义", hint: "手动填写类型和 JSON" },
];

const knowledgeSpaceTemplates: KnowledgeSpaceTemplate[] = [
  {
    key: "general",
    label: "通用",
    name: "通用群聊知识库",
    description: "记录群聊中长期可复用的需求、供应、技能、教程、资源、风险和状态变化。",
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
            status: "string",
          },
        },
        supply: {
          label: "供应",
          fields: {
            item: "string",
            quantity: "string",
            price: "string",
            location: "string",
            status: "string",
          },
        },
        skill: {
          label: "技能",
          fields: {
            area: "string",
            evidence: "string",
            level: "string",
          },
        },
        solution: {
          label: "教程方法",
          fields: {
            topic: "string",
            steps: "string",
            context: "string",
          },
        },
        resource: {
          label: "工具资源",
          fields: {
            name: "string",
            url: "string",
            usage: "string",
          },
        },
        risk: {
          label: "风险避坑",
          fields: {
            topic: "string",
            risk: "string",
            mitigation: "string",
          },
        },
        risk_account: {
          label: "风险账号",
          fields: {
            reported_account_username: "string",
            reported_account_id: "string",
            reported_account_name: "string",
            reporter: "string",
            risk_type: "string",
            allegation: "string",
            evidence: "string",
            status: "reported|confirmed|disputed|cleared",
            mitigation: "string",
          },
        },
        event: {
          label: "活动机会",
          fields: {
            name: "string",
            time: "string",
            location: "string",
            topic: "string",
          },
        },
        status_update: {
          label: "状态变更",
          fields: {
            target_type: "string",
            target_query: "string",
            action: "string",
            reason: "string",
            target_user: "string",
          },
        },
      },
    }),
    extractPrompt:
      "只记录未来可能复用的信息。覆盖需求、供应、技能、教程方法、工具资源、风险避坑、风险账号、活动机会。技能画像必须基于用户自述、作品、持续高质量回答或明确承诺，不能凭一句闲聊推断。风险账号请用 risk_account，但只记录明确账号身份风险：某账号被点名曝光、举报、拉黑、指控诈骗/冒充/跑路/收款不发货，或围绕这类曝光的澄清和争议。不要因为账号本人发布敏感、不正规、灰产、博彩、成人、交易、广告或争议话题内容，就把发言者记为风险账号；这类内容如果有可复用避坑价值，只能用 risk，不能用 risk_account。risk_account 的 reported_account_username 记录被举报的 @username；reported_account_id 只有在消息中明确出现稳定数字 ID 或可从被举报账号本人消息确认时才填写；reporter 记录举报/曝光来源；evidence 必须写明举报/曝光/澄清依据；status 默认 reported，只有多方证据或明确结论时才用 confirmed，出现澄清或争议时用 disputed/cleared；subjectMessageRef 指向举报/曝光/澄清消息，不要指向被举报账号的普通聊天消息；不要把可变 @username 当成稳定身份。状态变更请用 status_update，target_type 填 demand/supply/skill/help_offer 等旧事实类型，target_query 填要失效的物品或主题，action 使用 resolved、expired、sold_out、paused、no_longer_needed 等英文短语。不要记录玩笑、猜测、纯闲聊、临时情绪或无证据结论。",
    summaryPrompt: "摘要附加时按需求、供应、技能、教程、资源、风险、风险账号、活动分组；风险账号只写“有人举报/已确认/存在争议”等证据状态，保留可联系用户、置信度和事实 ID，不展示 status_update。",
    confidenceThreshold: 0.75,
    retentionDays: 60,
  },
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
    key: "risk_accounts",
    label: "风险账号",
    name: "风险账号库",
    description: "记录群内明确曝光、举报、澄清和争议的账号身份风险，区分可变用户名与稳定身份 ID。",
    schemaJson: schemaString({
      types: {
        risk_account: {
          label: "风险账号",
          fields: {
            reported_account_username: "string",
            reported_account_id: "string",
            reported_account_name: "string",
            reporter: "string",
            risk_type: "string",
            allegation: "string",
            evidence: "string",
            status: "reported|confirmed|disputed|cleared",
            mitigation: "string",
          },
        },
      },
    }),
    extractPrompt:
      "只抽取明确账号身份风险：某账号被点名曝光、举报、拉黑、指控诈骗/冒充/跑路/收款不发货，或围绕这类曝光的澄清和争议。不记录玩笑、辱骂、猜测或没有对象的泛泛提醒。不要因为账号本人发布敏感、不正规、灰产、博彩、成人、交易、广告或争议话题内容，就把发言者记为风险账号；当前空间只有 risk_account，遇到这类普通发言必须跳过。reported_account_username 记录被举报的 @username；reported_account_id 只有在消息中明确出现稳定数字 ID，或可从被举报账号本人消息确认时才填写；reported_account_name 记录被举报账号显示名；reporter 记录举报/曝光来源；evidence 必须写明举报/曝光/澄清依据。subjectMessageRef 指向举报、曝光或澄清消息，让事实主体代表信息来源，不要指向被举报账号的普通聊天消息。status 默认 reported；多方证据或明确结论才用 confirmed；出现反驳或未证实时用 disputed；明确澄清时用 cleared。回答时必须说明证据状态，不要把可变 @username 当成稳定身份。",
    summaryPrompt:
      "摘要附加时只列出风险账号、证据状态、风险类型、举报来源和规避建议；除非 status 为 confirmed，否则使用“有人举报/群内曾曝光/存在争议”的措辞，并保留置信度和事实 ID。",
    confidenceThreshold: 0.8,
    retentionDays: 180,
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
  const [maintenanceEvents, setMaintenanceEvents] = useState<KnowledgeMaintenanceEvent[]>([]);
  const [chats, setChats] = useState<Chat[]>([]);
  const [editing, setEditing] = useState<KnowledgeSpace | null>(null);
  const [knowledgeQueryPreview, setKnowledgeQueryPreview] =
    useState<KnowledgeQueryResult | null>(null);
  const [knowledgeSearchResult, setKnowledgeSearchResult] =
    useState<KnowledgeQueryResult | null>(null);
  const [factSources, setFactSources] = useState<KnowledgeFactSources | null>(null);
  const [selectedSubject, setSelectedSubject] = useState<KnowledgeSubject | null>(null);
  const [editingFact, setEditingFact] = useState<KnowledgeFact | null>(null);
  const [editingFactSourceIDs, setEditingFactSourceIDs] = useState("");
  const [correctionDraft, setCorrectionDraft] = useState<CorrectionDraft | null>(null);
  const [maintenancePreview, setMaintenancePreview] =
    useState<KnowledgeMaintenanceResult | null>(null);
  const [previewingKnowledgeQuery, setPreviewingKnowledgeQuery] = useState(false);
  const [sendingKnowledgeQuery, setSendingKnowledgeQuery] = useState(false);
  const [runningNaturalQuery, setRunningNaturalQuery] = useState(false);
  const [previewingMaintenance, setPreviewingMaintenance] = useState(false);
  const [applyingMaintenance, setApplyingMaintenance] = useState(false);
  const [selectedSpaceId, setSelectedSpaceId] = useState<number | "all">("all");
  const [statusFilter, setStatusFilter] = useState<FactStatusFilter>("all");
  const [factChatId, setFactChatId] = useState<number | "all">("all");
  const [factTypeFilter, setFactTypeFilter] = useState("");
  const [factQuery, setFactQuery] = useState("");
  const [naturalQuery, setNaturalQuery] = useState("");
  const [maintenanceText, setMaintenanceText] = useState("");
  const [manualFact, setManualFact] = useState<KnowledgeFact>(() => newManualFact());
  const [manualFactDraft, setManualFactDraft] = useState<ManualFactDraft>(() =>
    newManualFactDraft(),
  );
  const [creatingManualFact, setCreatingManualFact] = useState(false);
  const [savingFactEdit, setSavingFactEdit] = useState(false);
  const [applyingFactCorrection, setApplyingFactCorrection] = useState(false);
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

  useEffect(() => {
    void loadMaintenanceEvents();
  }, [selectedSpaceId, factChatId]);

  async function load() {
    try {
      const [spaceItems, chatItems] = await Promise.all([
        api.listKnowledgeSpaces(),
        api.listChats(),
      ]);
      setSpaces(spaceItems.map(normalizeSpace));
      setChats(chatItems);
      setEditing((current) => current ?? (spaceItems[0] ? normalizeSpace(spaceItems[0]) : null));
      await Promise.all([loadFacts(), loadSubjects(), loadRuns(), loadMaintenanceEvents()]);
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

  async function loadMaintenanceEvents(spaceId: number | "all" = selectedSpaceId) {
    try {
      const items = await api.listKnowledgeMaintenanceEvents({
        spaceId: spaceId === "all" ? undefined : spaceId,
        chatId: factChatId === "all" ? undefined : factChatId,
        limit: 50,
      });
      setMaintenanceEvents(items);
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
        toast.showSuccess(`知识抽取完成：读取 ${run.inputMessageCount} 条消息，处理 ${run.extractedCount} 条知识事实或状态变更。`);
      }
      setSelectedSpaceId(editing.id);
      await Promise.all([
        loadFacts(editing.id),
        loadSubjects(editing.id),
        loadRuns(editing.id),
        loadMaintenanceEvents(editing.id),
      ]);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function updateFactStatus(fact: KnowledgeFact, status: KnowledgeFact["status"]) {
    try {
      await api.updateKnowledgeFactStatus(fact.id, status);
      toast.showSuccess(status === "active" ? "已恢复这条事实。" : "已忽略这条事实。");
      await Promise.all([loadFacts(), loadSubjects(), loadMaintenanceEvents()]);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function showFactSources(fact: KnowledgeFact) {
    if (fact.sourceMessageIds.length === 0) {
      toast.showError("这条事实没有记录来源消息。");
      return;
    }
    try {
      const result = await api.getKnowledgeFactSources(fact.id);
      setFactSources(result);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function startEditingFact(fact: KnowledgeFact) {
    setEditingFact({
      ...fact,
      dataJson: prettyJsonString(fact.dataJson),
      sourceMessageIds: [...fact.sourceMessageIds],
    });
    setEditingFactSourceIDs(formatSourceMessageIDs(fact.sourceMessageIds));
  }

  async function saveEditingFact() {
    if (!editingFact) {
      return;
    }
    const error = validateManualFact(editingFact);
    if (error) {
      toast.showError(error);
      return;
    }
    const parsedSourceIDs = parseSourceMessageIDs(editingFactSourceIDs);
    if (parsedSourceIDs.error) {
      toast.showError(parsedSourceIDs.error);
      return;
    }

    setSavingFactEdit(true);
    try {
      const saved = await api.saveKnowledgeFact({
        ...editingFact,
        factType: editingFact.factType.trim(),
        title: editingFact.title.trim(),
        dataJson: editingFact.dataJson.trim() || "{}",
        subjectSenderName: editingFact.subjectSenderName.trim(),
        subjectUsername: editingFact.subjectUsername.trim().replace(/^@/, ""),
        sourceMessageIds: parsedSourceIDs.ids,
      });
      toast.showSuccess(`已更新知识事实「${saved.title}」。`);
      setEditingFact(null);
      setEditingFactSourceIDs("");
      await Promise.all([loadFacts(), loadSubjects()]);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setSavingFactEdit(false);
    }
  }

  function startCorrectingFact(fact: KnowledgeFact) {
    const item = primaryFactItem(fact);
    setCorrectionDraft({
      fact,
      wrong: item || fact.title,
      replacement: "",
    });
  }

  async function applyFactCorrection() {
    if (!correctionDraft) {
      return;
    }
    const wrong = compactText(correctionDraft.wrong);
    const replacement = compactText(correctionDraft.replacement);
    if (!wrong) {
      toast.showError("请填写错误内容。");
      return;
    }
    if (!replacement) {
      toast.showError("请填写正确内容。");
      return;
    }
    const corrected = buildCorrectedFact(correctionDraft.fact, wrong, replacement);
    const error = validateManualFact(corrected);
    if (error) {
      toast.showError(error);
      return;
    }

    setApplyingFactCorrection(true);
    try {
      const saved = await api.createKnowledgeFact({
        ...corrected,
        factType: corrected.factType.trim(),
        title: corrected.title.trim(),
        dataJson: corrected.dataJson.trim() || "{}",
        subjectSenderName: corrected.subjectSenderName.trim(),
        subjectUsername: corrected.subjectUsername.trim().replace(/^@/, ""),
      });
      await api.updateKnowledgeFactStatus(correctionDraft.fact.id, "dismissed");
      toast.showSuccess(`已纠正为「${saved.title}」，并忽略旧事实。`);
      setCorrectionDraft(null);
      await Promise.all([loadFacts(), loadSubjects(), loadMaintenanceEvents()]);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setApplyingFactCorrection(false);
    }
  }

  async function createManualFact() {
    const draftError = validateManualFactDraft(manualFactDraft);
    if (draftError) {
      toast.showError(draftError);
      return;
    }
    const prepared = prepareManualFact(manualFact, manualFactDraft);
    const error = validateManualFact(prepared);
    if (error) {
      toast.showError(error);
      return;
    }
    setCreatingManualFact(true);
    try {
      const saved = await api.createKnowledgeFact({
        ...prepared,
        factType: prepared.factType.trim(),
        title: prepared.title.trim(),
        dataJson: prepared.dataJson.trim() || "{}",
        subjectSenderName: prepared.subjectSenderName.trim(),
        subjectUsername: prepared.subjectUsername.trim().replace(/^@/, ""),
      });
      toast.showSuccess(`已新增知识事实「${saved.title}」。`);
      setSelectedSpaceId(saved.spaceId);
      setManualFact(newManualFact({ spaceId: saved.spaceId, chatId: saved.chatId }));
      setManualFactDraft(newManualFactDraft({ mode: manualFactDraft.mode }));
      await Promise.all([loadFacts(saved.spaceId), loadSubjects(saved.spaceId)]);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setCreatingManualFact(false);
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

  async function runNaturalKnowledgeQuery() {
    if (!naturalQuery.trim()) {
      toast.showError("请输入自然语言问题。");
      return;
    }
    setRunningNaturalQuery(true);
    try {
      const result = await api.renderNaturalKnowledgeQuery({
        text: naturalQuery,
        spaceId: selectedSpaceId === "all" ? undefined : selectedSpaceId,
        chatId: factChatId === "all" ? undefined : factChatId,
        limit: 20,
      });
      setKnowledgeSearchResult(result);
      setFactQuery(result.query);
      setFactTypeFilter(result.factType);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setRunningNaturalQuery(false);
    }
  }

  async function previewMaintenance() {
    if (!maintenanceText.trim()) {
      toast.showError("请输入维护说明。");
      return;
    }
    setPreviewingMaintenance(true);
    try {
      const result = await api.previewKnowledgeMaintenance(maintenanceText);
      setMaintenancePreview(result);
      if (!maintenanceResultHasMatches(result)) {
        toast.showError("没有找到可安全维护的匹配事实。");
      }
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setPreviewingMaintenance(false);
    }
  }

  async function applyMaintenance() {
    if (!maintenanceText.trim()) {
      toast.showError("请输入维护说明。");
      return;
    }
    setApplyingMaintenance(true);
    try {
      const result = await api.applyKnowledgeMaintenance(maintenanceText);
      setMaintenancePreview(result);
      toast.showSuccess(`已维护 ${result.updatedFacts.length} 条知识事实。`);
      await Promise.all([loadFacts(), loadSubjects(), loadMaintenanceEvents()]);
    } catch (err) {
      toast.showError(asMessage(err));
    } finally {
      setApplyingMaintenance(false);
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
  const manualFactChatOptions = useMemo(() => {
    const space = spaces.find((item) => item.id === manualFact.spaceId);
    if (!space || space.chatIds.length === 0) {
      return chats;
    }
    return chats.filter((chat) => space.chatIds.includes(chat.id));
  }, [chats, manualFact.spaceId, spaces]);
  const spaceNameByID = useMemo(
    () => new Map(spaces.map((space) => [space.id, space.name])),
    [spaces],
  );

  function renderFactActions(fact: KnowledgeFact) {
    return (
      <div className="table-row-actions">
        <button
          className="text-link-button"
          onClick={() => startEditingFact(fact)}
          type="button"
        >
          编辑
        </button>
        <button
          className="text-link-button"
          onClick={() => startTransition(() => void showFactSources(fact))}
          type="button"
        >
          来源
        </button>
        <button
          className="text-link-button"
          onClick={() => startCorrectingFact(fact)}
          type="button"
        >
          纠正
        </button>
        {fact.status === "active" ? (
          <button
            className="text-link-button"
            onClick={() =>
              startTransition(() => void updateFactStatus(fact, "dismissed"))
            }
            type="button"
          >
            忽略
          </button>
        ) : (
          <button
            className="text-link-button"
            onClick={() => startTransition(() => void updateFactStatus(fact, "active"))}
            type="button"
          >
            恢复
          </button>
        )}
      </div>
    );
  }

  function renderFactCard(fact: KnowledgeFact) {
    return (
      <article className="knowledge-fact-card" key={fact.id}>
        <div className="knowledge-card-head">
          <div className="data-row-title">
            <strong>{fact.title}</strong>
            <span>{formatFactMeta(fact)}</span>
          </div>
          <StatusPill tone={factStatusTone(fact.status)}>{fact.status}</StatusPill>
        </div>
        <pre className="knowledge-fact-json">{prettyJsonString(fact.dataJson)}</pre>
        <div className="knowledge-card-footer">
          <span>{formatSubject(fact)}</span>
          <span>{Math.round(fact.confidence * 100)}%</span>
          <span>{formatDateTime(fact.lastSeenAt)}</span>
        </div>
        {renderFactActions(fact)}
      </article>
    );
  }

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

      <Surface
        title="知识搜索"
        description="用自然语言从结构化事实里找人、主题、供需和来源。"
      >
        <div className="knowledge-search-shell">
          <div className="knowledge-search-bar">
            <Field label="问题">
              <Input
                onChange={(event) => {
                  setNaturalQuery(event.target.value);
                  setKnowledgeSearchResult(null);
                }}
                placeholder="谁了解炒币"
                value={naturalQuery}
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
            <Button
              className="knowledge-search-button"
              disabled={runningNaturalQuery || !naturalQuery.trim()}
              onClick={() => startTransition(() => void runNaturalKnowledgeQuery())}
              type="button"
              variant="secondary"
            >
              {runningNaturalQuery ? "搜索中..." : "搜索知识"}
            </Button>
          </div>

          {knowledgeSearchResult ? (
            <div className="knowledge-search-result">
              <div className="knowledge-result-summary">
                <StatusPill tone={knowledgeSearchResult.facts.length > 0 ? "good" : "neutral"}>
                  {knowledgeSearchResult.facts.length} 条事实
                </StatusPill>
                <StatusPill tone={knowledgeSearchResult.subjects.length > 0 ? "good" : "neutral"}>
                  {knowledgeSearchResult.subjects.length} 个用户
                </StatusPill>
                {knowledgeSearchResult.query ? (
                  <span>关键词：{knowledgeSearchResult.query}</span>
                ) : null}
                {knowledgeSearchResult.factType ? (
                  <span>类型：{knowledgeSearchResult.factType}</span>
                ) : null}
              </div>

              {knowledgeSearchResult.facts.length === 0 &&
              knowledgeSearchResult.subjects.length === 0 ? (
                <EmptyState
                  title="没有匹配知识"
                  description="可以换一个关键词，或先运行知识抽取。"
                />
              ) : (
                <div className="knowledge-result-grid">
                  <section className="knowledge-result-column">
                    <div className="knowledge-column-head">
                      <h3>相关用户</h3>
                      <span>{knowledgeSearchResult.subjects.length}</span>
                    </div>
                    {knowledgeSearchResult.subjects.length === 0 ? (
                      <p className="muted">没有可聚合的用户画像。</p>
                    ) : (
                      <div className="knowledge-card-list">
                        {knowledgeSearchResult.subjects.map((subject) => (
                          <button
                            className="knowledge-subject-card"
                            key={subject.key}
                            onClick={() => setSelectedSubject(subject)}
                            type="button"
                          >
                            <span className="knowledge-subject-name">
                              {formatSubjectName(subject)}
                            </span>
                            <span className="muted">{formatSubjectIdentity(subject)}</span>
                            <span>{subject.factCount} 条事实</span>
                            <span className="muted">{formatList(subject.factTypes)}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </section>

                  <section className="knowledge-result-column">
                    <div className="knowledge-column-head">
                      <h3>匹配事实</h3>
                      <span>{knowledgeSearchResult.facts.length}</span>
                    </div>
                    {knowledgeSearchResult.facts.length === 0 ? (
                      <p className="muted">没有直接匹配的事实。</p>
                    ) : (
                      <div className="knowledge-card-list">
                        {knowledgeSearchResult.facts.map(renderFactCard)}
                      </div>
                    )}
                  </section>
                </div>
              )}
            </div>
          ) : null}
        </div>
      </Surface>

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
        title="智能维护"
        description="在网页端验证自然语言维护结果。"
      >
        <div className="form-stack">
          <div className="knowledge-run-panel">
            <div>
              <strong>自然语言维护</strong>
              <p className="muted">先预览匹配事实，确认后再更新状态并写入维护记录。</p>
            </div>
            <Field label="维护说明">
              <Textarea
                onChange={(event) => {
                  setMaintenanceText(event.target.value);
                  setMaintenancePreview(null);
                }}
                placeholder="Alice 不再需要 Gmail 邮箱"
                rows={3}
                value={maintenanceText}
              />
            </Field>
            <div className="table-row-actions">
              <Button
                disabled={previewingMaintenance || !maintenanceText.trim()}
                onClick={() => startTransition(() => void previewMaintenance())}
                type="button"
                variant="ghost"
              >
                {previewingMaintenance ? "预览中..." : "预览维护"}
              </Button>
              <Button
                disabled={
                  applyingMaintenance ||
                  !maintenanceText.trim() ||
                  !maintenanceResultHasMatches(maintenancePreview)
                }
                onClick={() => startTransition(() => void applyMaintenance())}
                type="button"
                variant="secondary"
              >
                {applyingMaintenance ? "执行中..." : "确认执行"}
              </Button>
            </div>
          </div>

          {maintenancePreview ? (
            <div className="data-table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>动作</th>
                    <th>目标</th>
                    <th>用户</th>
                    <th>匹配事实</th>
                    <th>原因</th>
                  </tr>
                </thead>
                <tbody>
                  {maintenancePreviewFacts(maintenancePreview).map((fact) => (
                    <tr className="data-row" key={fact.id}>
                      <td>{formatMaintenanceAction(maintenancePreview.action)}</td>
                      <td>{maintenancePreview.targetQuery || "未识别"}</td>
                      <td>{maintenancePreview.targetUser || "未识别"}</td>
                      <td>
                        <div className="data-row-title">
                          <strong>#{fact.id} {fact.title}</strong>
                          <span>{fact.factType} / {fact.chatTitle || fact.chatId}</span>
                        </div>
                      </td>
                      <td>
                        <span className="muted">{maintenancePreview.reason || "无"}</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!maintenanceResultHasMatches(maintenancePreview) ? (
                <p className="muted">没有可安全维护的匹配事实。</p>
              ) : null}
            </div>
          ) : null}
        </div>
      </Surface>

      <Surface
        title="人工新增事实"
        description="给小白使用的快捷录入。优先填人话字段；只有高级自定义才需要写 JSON。"
      >
        <div className="form-stack">
          <div className="form-grid">
            <Field label="知识空间" required>
              <AppSelect
                onChange={(value) =>
                  setManualFact((current) => ({
                    ...current,
                    spaceId: Number(value) || 0,
                    chatId: 0,
                  }))
                }
                options={[
                  { value: "", label: "选择知识空间" },
                  ...spaces.map((space) => ({
                    value: String(space.id),
                    label: space.name,
                  })),
                ]}
                value={manualFact.spaceId ? String(manualFact.spaceId) : ""}
              />
            </Field>
            <Field label="群组" required>
              <AppSelect
                onChange={(value) =>
                  setManualFact((current) => ({
                    ...current,
                    chatId: Number(value) || 0,
                  }))
                }
                options={[
                  { value: "", label: "选择群组" },
                  ...manualFactChatOptions.map((chat) => ({
                    value: String(chat.id),
                    label: chat.title,
                  })),
                ]}
                value={manualFact.chatId ? String(manualFact.chatId) : ""}
              />
            </Field>
            <Field label="置信度">
              <Input
                max={1}
                min={0.1}
                onChange={(event) =>
                  setManualFact((current) => ({
                    ...current,
                    confidence: Number(event.target.value),
                  }))
                }
                step={0.05}
                type="number"
                value={manualFact.confidence}
              />
            </Field>
          </div>

          <Field label="要记录什么" required>
            <AppSelect
              onChange={(value) =>
                setManualFactDraft((current) => ({
                  ...current,
                  mode: (value as ManualFactMode) || "skill",
                }))
              }
              options={manualFactModeOptions.map((option) => ({
                value: option.mode,
                label: `${option.label} · ${option.hint}`,
              }))}
              value={manualFactDraft.mode}
            />
          </Field>

          {manualFactDraft.mode === "custom" ? (
            <>
              <div className="form-grid">
                <Field label="类型" required>
                  <Input
                    onChange={(event) =>
                      setManualFact((current) => ({
                        ...current,
                        factType: event.target.value,
                      }))
                    }
                    placeholder="skill / demand / supply / risk_account"
                    value={manualFact.factType}
                  />
                </Field>
                <Field label="标题" required>
                  <Input
                    onChange={(event) =>
                      setManualFact((current) => ({
                        ...current,
                        title: event.target.value,
                      }))
                    }
                    placeholder="Alice 了解炒币"
                    value={manualFact.title}
                  />
                </Field>
              </div>
              <div className="form-grid">
                <Field label="用户名">
                  <Input
                    onChange={(event) =>
                      setManualFact((current) => ({
                        ...current,
                        subjectUsername: event.target.value,
                      }))
                    }
                    placeholder="@alice"
                    value={manualFact.subjectUsername}
                  />
                </Field>
                <Field label="显示名">
                  <Input
                    onChange={(event) =>
                      setManualFact((current) => ({
                        ...current,
                        subjectSenderName: event.target.value,
                      }))
                    }
                    placeholder="Alice"
                    value={manualFact.subjectSenderName}
                  />
                </Field>
              </div>
              <Field label="数据 JSON">
                <Textarea
                  onChange={(event) =>
                    setManualFact((current) => ({
                      ...current,
                      dataJson: event.target.value,
                    }))
                  }
                  rows={5}
                  value={manualFact.dataJson}
                />
              </Field>
            </>
          ) : (
            <>
              <div className="form-grid">
                <Field label="相关人 / 账号" required={manualFactDraft.mode !== "demand"}>
                  <Input
                    onChange={(event) =>
                      setManualFactDraft((current) => ({
                        ...current,
                        person: event.target.value,
                      }))
                    }
                    placeholder={manualFactDraft.mode === "risk_account" ? "@alice 或 Alice" : "Alice"}
                    value={manualFactDraft.person}
                  />
                </Field>
                <Field label="用户名">
                  <Input
                    onChange={(event) =>
                      setManualFactDraft((current) => ({
                        ...current,
                        username: event.target.value,
                      }))
                    }
                    placeholder="@alice，可不填"
                    value={manualFactDraft.username}
                  />
                </Field>
              </div>
              <Field label={manualFactDraft.mode === "risk_account" ? "曝光原因" : "主题 / 物品"} required>
                <Input
                  onChange={(event) =>
                    setManualFactDraft((current) => ({
                      ...current,
                      title: event.target.value,
                    }))
                  }
                  placeholder={manualFactPlaceholder(manualFactDraft.mode)}
                  value={manualFactDraft.title}
                />
              </Field>
              <Field label="补充说明 / 证据">
                <Textarea
                  onChange={(event) =>
                    setManualFactDraft((current) => ({
                      ...current,
                      detail: event.target.value,
                    }))
                  }
                  placeholder="例如：谁说的、为什么可信、规避建议。没有就留空。"
                  rows={4}
                  value={manualFactDraft.detail}
                />
              </Field>
            </>
          )}

          {manualFactDraft.mode !== "custom" ? (
            <div className="knowledge-fact-card">
              <div className="knowledge-card-head">
                <div className="data-row-title">
                  <strong>将保存为</strong>
                  <span>{manualFactPreviewText(manualFact, manualFactDraft)}</span>
                </div>
                <StatusPill tone="neutral">{manualFactDraft.mode}</StatusPill>
              </div>
              <pre className="knowledge-fact-json">
                {prepareManualFact(manualFact, manualFactDraft).dataJson}
              </pre>
            </div>
          ) : null}

          <div className="editor-footer">
            <p className="muted">
              人工新增的事实会立即进入 active 状态，并参与后续查询和摘要附加。
              风险账号只建议录入明确曝光或举报，不要把敏感聊天内容当成账号风险。
            </p>
            <Button
              disabled={creatingManualFact}
              onClick={() => startTransition(() => void createManualFact())}
              type="button"
            >
              {creatingManualFact ? "保存中..." : "新增事实"}
            </Button>
          </div>
        </div>
      </Surface>

      <Surface
        title="维护记录"
        description="记录事实被恢复、过期或忽略的来源，便于排查知识库状态变化。"
      >
        {maintenanceEvents.length === 0 ? (
          <EmptyState title="还没有维护记录" description="通过 Bot 或页面维护知识事实后会显示在这里。" />
        ) : (
          <div className="data-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>时间</th>
                  <th>动作</th>
                  <th>事实</th>
                  <th>来源</th>
                  <th>状态</th>
                  <th>原因</th>
                </tr>
              </thead>
              <tbody>
                {maintenanceEvents.map((event) => (
                  <tr className="data-row" key={event.id}>
                    <td>{formatDateTime(event.createdAt)}</td>
                    <td>{formatMaintenanceAction(event.action)}</td>
                    <td>{event.factTitle || `#${event.factId}`}</td>
                    <td>{formatMaintenanceSource(event.source)}</td>
                    <td>
                      <StatusPill tone={maintenanceEventStatusTone(event.nextStatus)}>
                        {event.previousStatus || "-"} → {event.nextStatus || "-"}
                      </StatusPill>
                    </td>
                    <td>{event.reason || event.operatorText || "-"}</td>
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
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {subjects.map((subject) => (
                  <tr className="data-row" key={subject.key}>
                    <td>
                      <button
                        className="text-link-button"
                        onClick={() => setSelectedSubject(subject)}
                        type="button"
                      >
                        {formatSubjectName(subject)}
                      </button>
                    </td>
                    <td>{subject.factCount}</td>
                    <td>{formatList(subject.factTypes)}</td>
                    <td>{formatList(subject.chatTitles)}</td>
                    <td>{formatDateTime(subject.lastSeenAt)}</td>
                    <td>
                      <div className="data-row-title">
                        <span>{formatSubjectFactTitles(subject)}</span>
                      </div>
                    </td>
                    <td>
                      <button
                        className="text-link-button"
                        onClick={() => setSelectedSubject(subject)}
                        type="button"
                      >
                        详情
                      </button>
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
                      {renderFactActions(fact)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Surface>

      <Modal
        actions={<Button onClick={() => setSelectedSubject(null)} type="button">关闭</Button>}
        description={
          selectedSubject
            ? `${selectedSubject.factCount} 条 active 事实 / 最近发现 ${formatDateTime(selectedSubject.lastSeenAt)}`
            : undefined
        }
        onClose={() => setSelectedSubject(null)}
        open={selectedSubject !== null}
        title={selectedSubject ? `用户画像：${formatSubjectName(selectedSubject)}` : "用户画像"}
      >
        {selectedSubject ? (
          <div className="knowledge-profile-detail">
            <div className="knowledge-profile-summary">
              <div>
                <span>身份</span>
                <strong>{formatSubjectIdentity(selectedSubject)}</strong>
              </div>
              <div>
                <span>事实类型</span>
                <strong>{formatList(selectedSubject.factTypes)}</strong>
              </div>
              <div>
                <span>群组</span>
                <strong>{formatList(selectedSubject.chatTitles)}</strong>
              </div>
            </div>

            {selectedSubject.facts.length === 0 ? (
              <EmptyState title="暂无事实" description="这个用户当前没有可展示的 active 事实。" />
            ) : (
              <div className="knowledge-card-list">
                {selectedSubject.facts.map(renderFactCard)}
              </div>
            )}
          </div>
        ) : null}
      </Modal>

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

      <Modal
        actions={
          <>
            <Button
              onClick={() => {
                setEditingFact(null);
                setEditingFactSourceIDs("");
              }}
              type="button"
              variant="ghost"
            >
              取消
            </Button>
            <Button
              disabled={savingFactEdit}
              onClick={() => startTransition(() => void saveEditingFact())}
              type="button"
            >
              {savingFactEdit ? "保存中..." : "保存事实"}
            </Button>
          </>
        }
        description={
          editingFact
            ? `#${editingFact.id} / ${editingFact.chatTitle || editingFact.chatId}`
            : undefined
        }
        onClose={() => {
          setEditingFact(null);
          setEditingFactSourceIDs("");
        }}
        open={editingFact !== null}
        title="编辑知识事实"
      >
        {editingFact ? (
          <div className="form-stack">
            <div className="form-grid">
              <Field label="类型" required>
                <Input
                  onChange={(event) =>
                    setEditingFact((current) =>
                      current ? { ...current, factType: event.target.value } : current,
                    )
                  }
                  value={editingFact.factType}
                />
              </Field>
              <Field label="置信度">
                <Input
                  max={1}
                  min={0.1}
                  onChange={(event) =>
                    setEditingFact((current) =>
                      current ? { ...current, confidence: Number(event.target.value) } : current,
                    )
                  }
                  step={0.05}
                  type="number"
                  value={editingFact.confidence}
                />
              </Field>
            </div>

            <Field label="标题" required>
              <Input
                onChange={(event) =>
                  setEditingFact((current) =>
                    current ? { ...current, title: event.target.value } : current,
                  )
                }
                value={editingFact.title}
              />
            </Field>

            <div className="form-grid">
              <Field label="用户名">
                <Input
                  onChange={(event) =>
                    setEditingFact((current) =>
                      current
                        ? { ...current, subjectUsername: event.target.value }
                        : current,
                    )
                  }
                  placeholder="@alice"
                  value={editingFact.subjectUsername}
                />
              </Field>
              <Field label="显示名">
                <Input
                  onChange={(event) =>
                    setEditingFact((current) =>
                      current
                        ? { ...current, subjectSenderName: event.target.value }
                        : current,
                    )
                  }
                  value={editingFact.subjectSenderName}
                />
              </Field>
            </div>

            <Field label="数据 JSON">
              <Textarea
                onChange={(event) =>
                  setEditingFact((current) =>
                    current ? { ...current, dataJson: event.target.value } : current,
                  )
                }
                rows={7}
                value={editingFact.dataJson}
              />
            </Field>

            <div className="form-grid">
              <Field label="过期时间">
                <Input
                  onChange={(event) =>
                    setEditingFact((current) =>
                      current
                        ? {
                            ...current,
                            expiresAt: dateTimeLocalToISO(event.target.value),
                          }
                        : current,
                    )
                  }
                  type="datetime-local"
                  value={isoToDateTimeLocal(editingFact.expiresAt)}
                />
              </Field>
              <Field label="来源消息 ID" hint="多个 ID 用逗号或空格分隔。">
                <Input
                  onChange={(event) => setEditingFactSourceIDs(event.target.value)}
                  placeholder="123, 456"
                  value={editingFactSourceIDs}
                />
              </Field>
            </div>
          </div>
        ) : null}
      </Modal>

      <Modal
        actions={
          <>
            <Button
              onClick={() => setCorrectionDraft(null)}
              type="button"
              variant="ghost"
            >
              取消
            </Button>
            <Button
              disabled={applyingFactCorrection}
              onClick={() => startTransition(() => void applyFactCorrection())}
              type="button"
            >
              {applyingFactCorrection ? "纠正中..." : "确认纠正"}
            </Button>
          </>
        }
        description={
          correctionDraft
            ? `#${correctionDraft.fact.id} / ${correctionDraft.fact.chatTitle || correctionDraft.fact.chatId}`
            : undefined
        }
        onClose={() => setCorrectionDraft(null)}
        open={correctionDraft !== null}
        title="纠正知识事实"
      >
        {correctionDraft ? (
          <div className="form-stack">
            <div className="knowledge-fact-card">
              <div className="knowledge-card-head">
                <div className="data-row-title">
                  <strong>{correctionDraft.fact.title}</strong>
                  <span>{correctionDraft.fact.factType} / {formatSubject(correctionDraft.fact)}</span>
                </div>
                <StatusPill tone={factStatusTone(correctionDraft.fact.status)}>
                  {correctionDraft.fact.status}
                </StatusPill>
              </div>
              <pre className="knowledge-fact-json">
                {prettyJsonString(correctionDraft.fact.dataJson)}
              </pre>
            </div>

            <div className="form-grid">
              <Field label="错误内容" required>
                <Input
                  onChange={(event) =>
                    setCorrectionDraft((current) =>
                      current ? { ...current, wrong: event.target.value } : current,
                    )
                  }
                  placeholder="手机号码"
                  value={correctionDraft.wrong}
                />
              </Field>
              <Field label="正确内容" required>
                <Input
                  onChange={(event) =>
                    setCorrectionDraft((current) =>
                      current ? { ...current, replacement: event.target.value } : current,
                    )
                  }
                  placeholder="Telegram账号"
                  value={correctionDraft.replacement}
                />
              </Field>
            </div>

            <div className="knowledge-fact-card">
              <div className="knowledge-card-head">
                <div className="data-row-title">
                  <strong>将保存为</strong>
                  <span>{buildCorrectedFact(correctionDraft.fact, correctionDraft.wrong, correctionDraft.replacement).title}</span>
                </div>
                <StatusPill tone="neutral">active</StatusPill>
              </div>
              <pre className="knowledge-fact-json">
                {prettyJsonString(buildCorrectedFact(correctionDraft.fact, correctionDraft.wrong, correctionDraft.replacement).dataJson)}
              </pre>
            </div>
          </div>
        ) : null}
      </Modal>

      <Modal
        actions={
          <Button onClick={() => setFactSources(null)} type="button">
            关闭
          </Button>
        }
        description={
          factSources
            ? `共 ${factSources.messages.length} 条来源消息。`
            : undefined
        }
        onClose={() => setFactSources(null)}
        open={factSources !== null}
        title={factSources ? `来源消息：${factSources.fact.title}` : "来源消息"}
      >
        {factSources ? (
          factSources.messages.length === 0 ? (
            <EmptyState title="未找到来源消息" description="来源消息可能还没有被同步或已被清理。" />
          ) : (
            <div className="form-stack">
              {factSources.messages.map((message) => (
                <div className="knowledge-run-panel" key={message.id}>
                  <div>
                    <strong>{formatMessageSender(message)}</strong>
                    <p className="muted">
                      #{message.telegramMessageId} / {formatDateTime(message.messageTime)}
                    </p>
                  </div>
                  <p>{messageSummaryText(message)}</p>
                </div>
              ))}
            </div>
          )
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

function newManualFact(overrides: Partial<KnowledgeFact> = {}): KnowledgeFact {
  return {
    id: 0,
    spaceId: 0,
    chatId: 0,
    factType: defaultManualFactType,
    title: "",
    dataJson: schemaString({ area: "", evidence: "", level: "" }),
    subjectSenderId: 0,
    subjectSenderName: "",
    subjectUsername: "",
    confidence: 1,
    status: "active",
    sourceMessageIds: [],
    firstSeenAt: "",
    lastSeenAt: "",
    createdAt: "",
    updatedAt: "",
    ...overrides,
  };
}

function newManualFactDraft(overrides: Partial<ManualFactDraft> = {}): ManualFactDraft {
  return {
    mode: "skill",
    person: "",
    username: "",
    title: "",
    detail: "",
    ...overrides,
  };
}

function prepareManualFact(fact: KnowledgeFact, draft: ManualFactDraft): KnowledgeFact {
  if (draft.mode === "custom") {
    return fact;
  }
  const person = compactText(draft.person);
  const username = compactText(draft.username).replace(/^@/, "");
  const title = compactText(draft.title);
  const detail = compactText(draft.detail);
  const prepared = newManualFact({
    ...fact,
    subjectSenderName: person,
    subjectUsername: username,
  });

  switch (draft.mode) {
    case "skill":
      return {
        ...prepared,
        factType: "skill",
        title: title || `${person || username || "未知用户"} 的技能`,
        dataJson: schemaString({
          area: title || detail || "未填写",
          evidence: detail || title || person || username || "",
          level: "unknown",
        }),
      };
    case "demand":
      return {
        ...prepared,
        factType: "demand",
        title: title || `${person || username || "未知用户"} 的需求`,
        dataJson: schemaString({
          item: title || detail || "未填写",
          quantity: "",
          budget: "",
          location: "",
          deadline: "",
        }),
      };
    case "supply":
      return {
        ...prepared,
        factType: "supply",
        title: title || `${person || username || "未知用户"} 的供应`,
        dataJson: schemaString({
          item: title || detail || "未填写",
          quantity: "",
          price: "",
          location: "",
        }),
      };
    case "risk_account":
      return {
        ...prepared,
        factType: "risk_account",
        title: title || `${person || username || "未知账号"} 风险账号`,
        dataJson: schemaString({
          reported_account_username: username,
          reported_account_name: person,
          reporter: "",
          risk_type: "风险账号",
          allegation: title || detail || "未填写",
          evidence: detail || title || "",
          status: "reported",
          mitigation: "",
        }),
      };
    default:
      return prepared;
  }
}

function buildCorrectedFact(fact: KnowledgeFact, wrong: string, replacement: string): KnowledgeFact {
  const cleanWrong = compactText(wrong);
  const cleanReplacement = compactText(replacement);
  const nextTitle = correctionTitle(fact.factType, cleanReplacement || cleanWrong || fact.title);
  return newManualFact({
    ...fact,
    id: 0,
    title: nextTitle,
    dataJson: rewriteFactDataJson(fact.dataJson, cleanWrong, cleanReplacement),
    status: "active",
    firstSeenAt: "",
    lastSeenAt: "",
    createdAt: "",
    updatedAt: "",
  });
}

function primaryFactItem(fact: KnowledgeFact) {
  try {
    const data = JSON.parse(fact.dataJson || "{}");
    if (!isRecord(data)) {
      return "";
    }
    for (const key of ["item", "topic", "resource", "title", "keyword", "query"]) {
      const value = data[key];
      if (typeof value === "string" && value.trim()) {
        return value.trim();
      }
    }
  } catch {
    return "";
  }
  return "";
}

function correctionTitle(factType: string, replacement: string) {
  switch (factType.trim().toLowerCase()) {
    case "demand":
      return `需要 ${replacement}`;
    case "supply":
      return `供应 ${replacement}`;
    default:
      return replacement;
  }
}

function rewriteFactDataJson(raw: string, wrong: string, replacement: string) {
  let data: Record<string, unknown>;
  try {
    const parsed = JSON.parse(raw || "{}");
    data = isRecord(parsed) ? { ...parsed } : {};
  } catch {
    data = {};
  }
  const keys = ["item", "topic", "resource", "title", "keyword", "query"];
  for (const key of keys) {
    const value = data[key];
    if (typeof value === "string" && (value.trim() === wrong || !wrong)) {
      data[key] = replacement;
    }
  }
  if (!("item" in data) && !("topic" in data)) {
    data.item = replacement;
  }
  return schemaString(data);
}

function validateManualFactDraft(draft: ManualFactDraft) {
  if (draft.mode === "custom") {
    return "";
  }
  if (!compactText(draft.person) && draft.mode !== "demand") {
    return "请填写相关人或账号。";
  }
  if (!compactText(draft.title)) {
    return draft.mode === "risk_account" ? "请填写曝光原因。" : "请填写主题或物品。";
  }
  return "";
}

function manualFactPlaceholder(mode: ManualFactMode) {
  switch (mode) {
    case "skill":
      return "例如：Rust、剪辑、翻译";
    case "demand":
      return "例如：Gmail 账号、显卡、素材";
    case "supply":
      return "例如：API、教程、资源";
    case "risk_account":
      return "例如：被曝光为骗子、收款不发货、已澄清";
    default:
      return "例如：任何长期有价值的信息";
  }
}

function manualFactPreviewText(fact: KnowledgeFact, draft: ManualFactDraft) {
  const prepared = prepareManualFact(fact, draft);
  return `${prepared.factType} / ${prepared.title}`;
}

function compactText(value: string) {
  return value.trim().replace(/\s+/g, " ");
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

function prettyJsonString(value: string) {
  try {
    return schemaString(JSON.parse(value || "{}"));
  } catch {
    return value;
  }
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

function validateManualFact(fact: KnowledgeFact) {
  if (!fact.spaceId) {
    return "请选择知识空间。";
  }
  if (!fact.chatId) {
    return "请选择群组。";
  }
  if (!fact.factType.trim()) {
    return "请填写事实类型。";
  }
  if (!fact.title.trim()) {
    return "请填写事实标题。";
  }
  try {
    JSON.parse(fact.dataJson || "{}");
  } catch {
    return "数据 JSON 必须是合法 JSON。";
  }
  if (fact.confidence <= 0 || fact.confidence > 1) {
    return "置信度必须在 0 到 1 之间。";
  }
  return "";
}

function formatSourceMessageIDs(ids: number[]) {
  return ids.join(", ");
}

function parseSourceMessageIDs(value: string): { ids: number[]; error: string } {
  const trimmed = value.trim();
  if (!trimmed) {
    return { ids: [], error: "" };
  }
  const ids: number[] = [];
  for (const part of trimmed.split(/[,\s]+/)) {
    const id = Number(part);
    if (!/^\d+$/.test(part) || !Number.isSafeInteger(id) || id <= 0) {
      return { ids: [], error: "来源消息 ID 必须是正整数。" };
    }
    ids.push(id);
  }
  return { ids: compactNumbers(ids), error: "" };
}

function compactNumbers(values: number[]) {
  return Array.from(new Set(values)).sort((a, b) => a - b);
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

function formatSubjectName(subject: KnowledgeSubject) {
  if (subject.displayName) {
    return subject.displayName;
  }
  if (subject.subjectUsername) {
    return `@${subject.subjectUsername}`;
  }
  if (subject.subjectSenderName) {
    return subject.subjectSenderName;
  }
  if (subject.subjectSenderId) {
    return String(subject.subjectSenderId);
  }
  return "未记录";
}

function formatSubjectIdentity(subject: KnowledgeSubject) {
  const parts: string[] = [];
  if (subject.subjectUsername) {
    parts.push(`@${subject.subjectUsername}`);
  }
  if (subject.subjectSenderName && subject.subjectSenderName !== subject.displayName) {
    parts.push(subject.subjectSenderName);
  }
  if (subject.subjectSenderId) {
    parts.push(String(subject.subjectSenderId));
  }
  return parts.join(" / ") || "未记录";
}

function formatFactMeta(fact: KnowledgeFact) {
  const parts = [
    fact.spaceName,
    fact.factType,
    fact.chatTitle || String(fact.chatId),
  ].filter(Boolean);
  return parts.join(" / ") || "未记录";
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

function isoToDateTimeLocal(value?: string) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const offset = date.getTimezoneOffset() * 60 * 1000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function dateTimeLocalToISO(value: string) {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toISOString();
}

function formatFactExpiry(fact: KnowledgeFact) {
  if (!fact.expiresAt) {
    return "长期保留";
  }
  return formatDateTime(fact.expiresAt);
}

function formatMessageSender(message: Message) {
  if (message.senderUsername) {
    return `@${message.senderUsername}`;
  }
  if (message.senderName) {
    return message.senderName;
  }
  if (message.telegramSenderId) {
    return String(message.telegramSenderId);
  }
  return "未知用户";
}

function messageSummaryText(message: Message) {
  const text = message.textContent || message.caption;
  if (text.trim()) {
    return text;
  }
  if (message.mediaKind) {
    return `[${message.mediaKind}]`;
  }
  return "[非文本消息]";
}

function formatKnowledgeQueryPreviewDescription(result: KnowledgeQueryResult) {
  return `匹配 ${result.facts.length} 条事实，关联 ${result.subjects.length} 个用户。`;
}

function maintenanceResultHasMatches(result: KnowledgeMaintenanceResult | null) {
  if (!result || result.action === "" || result.action === "none") {
    return false;
  }
  return result.matchedFacts.length > 0 || result.updatedFacts.length > 0;
}

function maintenancePreviewFacts(result: KnowledgeMaintenanceResult) {
  if (result.updatedFacts.length > 0) {
    return result.updatedFacts;
  }
  return result.matchedFacts;
}

function formatMaintenanceSource(source: KnowledgeMaintenanceEvent["source"]) {
  switch (source) {
    case "auto_status_update":
      return "自动状态变更";
    case "bot_command":
      return "Bot 命令";
    case "bot_update":
      return "Bot 自然语言";
    case "web":
      return "网页操作";
    default:
      return source || "未知";
  }
}

function formatMaintenanceAction(action: KnowledgeMaintenanceEvent["action"]) {
  switch (action) {
    case "expire":
      return "过期";
    case "dismiss":
      return "忽略";
    case "restore":
      return "恢复";
    case "correct":
      return "纠正";
    default:
      return action || "未知";
  }
}

function maintenanceEventStatusTone(status: KnowledgeMaintenanceEvent["nextStatus"]) {
  if (status === "active" || status === "expired" || status === "dismissed") {
    return factStatusTone(status);
  }
  return "neutral";
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
