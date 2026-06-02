"use client";

import { useEffect, useState, useTransition } from "react";
import { DashboardPage, EmptyState, MetricCard, MetricRail, Surface } from "@/components/dashboard-page";
import { useToast } from "@/components/toast";
import { Button, Field, Input, Select, StatusPill } from "@/components/ui";
import { api } from "@/lib/api";
import { LLMWikiPage, LLMWikiRun } from "@/lib/types";

const pageSize = 20;
const pageTypes = ["all", "topic", "person", "project", "entity", "daily", "index", "log", "page"];

export function LLMWikiPanel() {
  const toast = useToast();
  const [pages, setPages] = useState<LLMWikiPage[]>([]);
  const [runs, setRuns] = useState<LLMWikiRun[]>([]);
  const [selectedPage, setSelectedPage] = useState<LLMWikiPage | null>(null);
  const [query, setQuery] = useState("");
  const [pageType, setPageType] = useState<string>("all");
  const [total, setTotal] = useState(0);
  const [loading, startTransition] = useTransition();
  const [reindexing, startReindexTransition] = useTransition();

  useEffect(() => {
    startTransition(() => {
      void load();
    });
  }, []);

  async function load(nextQuery = query, nextType = pageType) {
    try {
      const [pageResult, runResult] = await Promise.all([
        api.listLLMWikiPages({
          q: nextQuery,
          type: nextType,
          page: 1,
          pageSize,
        }),
        api.listLLMWikiRuns({ limit: 20 }),
      ]);
      setPages(pageResult.items);
      setTotal(pageResult.total);
      setRuns(runResult.items);
      if (selectedPage && !pageResult.items.some((page) => page.id === selectedPage.id)) {
        setSelectedPage(null);
      }
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function openPage(page: LLMWikiPage) {
    try {
      const detail = await api.getLLMWikiPage(page.id);
      setSelectedPage(detail);
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  async function reindex() {
    try {
      const result = await api.reindexLLMWiki();
      toast.showSuccess(`已重新索引 ${result.pageCount} 个 Wiki 页面。`);
      await load();
    } catch (err) {
      toast.showError(asMessage(err));
    }
  }

  function applySearch() {
    startTransition(() => {
      void load(query, pageType);
    });
  }

  return (
    <DashboardPage
      title="LLM Wiki"
      description="查看 AI 维护的 Markdown 语义记忆、索引状态和最近更新。"
      actions={
        <Button
          disabled={reindexing}
          onClick={() => startReindexTransition(() => void reindex())}
          type="button"
        >
          {reindexing ? "索引中..." : "重新索引"}
        </Button>
      }
    >
      <MetricRail>
        <MetricCard
          label="索引页面"
          value={total}
          badge="Markdown"
          detail="来自当前 Wiki 工作区。"
        />
        <MetricCard
          label="本页结果"
          value={pages.length}
          badge={loading ? "刷新中" : "已加载"}
          tone={loading ? "warn" : "neutral"}
          detail={`最多展示 ${pageSize} 条。`}
        />
        <MetricCard
          label="更新任务"
          value={runs.length}
          badge="最近"
          detail="最近 20 条 Wiki 更新记录。"
        />
      </MetricRail>

      <div className="dashboard-workspace llm-wiki-workspace">
        <div className="llm-wiki-column">
          <Surface title="页面索引">
            <div className="llm-wiki-filter-bar">
              <Field label="搜索">
                <Input
                  onChange={(event) => setQuery(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      applySearch();
                    }
                  }}
                  placeholder="路径、标题或正文"
                  value={query}
                />
              </Field>
              <Field label="类型">
                <Select
                  onChange={(event) => {
                    setPageType(event.target.value);
                    startTransition(() => void load(query, event.target.value));
                  }}
                  value={pageType}
                >
                  {pageTypes.map((type) => (
                    <option key={type} value={type}>
                      {type === "all" ? "全部" : type}
                    </option>
                  ))}
                </Select>
              </Field>
              <Button disabled={loading} onClick={applySearch} type="button" variant="secondary">
                {loading ? "搜索中..." : "搜索"}
              </Button>
            </div>

            {pages.length === 0 ? (
              <EmptyState title="没有 Wiki 页面" description="当前筛选条件下没有可展示的索引页面。" />
            ) : (
              <div className="llm-wiki-page-list">
                {pages.map((page) => (
                  <button
                    className={`llm-wiki-page-item ${selectedPage?.id === page.id ? "active" : ""}`}
                    key={page.id}
                    onClick={() => startTransition(() => void openPage(page))}
                    type="button"
                  >
                    <span>
                      <strong>{page.title}</strong>
                      <small>{page.path}</small>
                    </span>
                    <StatusPill tone="neutral">{page.pageType}</StatusPill>
                  </button>
                ))}
              </div>
            )}
          </Surface>

          <Surface title="最近更新">
            {runs.length === 0 ? (
              <EmptyState title="没有更新记录" description="暂未记录 LLM Wiki 更新任务。" />
            ) : (
              <div className="llm-wiki-run-list">
                {runs.map((run) => (
                  <div className="llm-wiki-run-item" key={run.id}>
                    <div>
                      <strong>#{run.id}</strong>
                      <span>{formatDateTime(run.finishedAt ?? run.updatedAt)}</span>
                    </div>
                    <StatusPill tone={runStatusTone(run.status)}>{run.status}</StatusPill>
                    <small>
                      chat {run.chatId || "-"} / summary {run.summaryId || "-"} / {run.updatedPageCount} 页
                    </small>
                    {run.errorMessage ? <p>{run.errorMessage}</p> : null}
                  </div>
                ))}
              </div>
            )}
          </Surface>
        </div>

        <Surface title={selectedPage ? selectedPage.title : "页面内容"}>
          {selectedPage ? (
            <div className="llm-wiki-detail">
              <div className="llm-wiki-detail-meta">
                <StatusPill tone="neutral">{selectedPage.pageType}</StatusPill>
                <span>{selectedPage.path}</span>
                <span>{formatDateTime(selectedPage.updatedAt)}</span>
              </div>
              <pre className="llm-wiki-content">{selectedPage.contentText || ""}</pre>
            </div>
          ) : (
            <EmptyState title="选择一个页面" description="从左侧索引列表打开 Markdown 文件内容。" />
          )}
        </Surface>
      </div>
    </DashboardPage>
  );
}

function runStatusTone(status: LLMWikiRun["status"]) {
  if (status === "succeeded") {
    return "good";
  }
  if (status === "failed") {
    return "bad";
  }
  if (status === "running") {
    return "warn";
  }
  return "neutral";
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

function asMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}
