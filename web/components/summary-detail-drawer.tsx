"use client";

import { startTransition } from "react";
import { Drawer } from "@/components/drawer";
import { EmptyState } from "@/components/dashboard-page";
import { SummaryMarkdown } from "@/components/summary-markdown";
import { StatusPill } from "@/components/ui";
import { Chat, Summary } from "@/lib/types";
import { deliveryState, statusText, statusTone } from "@/components/summaries-panel-sections";

export function SummaryDetailDrawer({
  botReady,
  chatTitle,
  onClose,
  onOpenContext,
  onRerunSummary,
  onRetryDelivery,
  open,
  selectedChat,
  selectedSummary
}: {
  botReady: boolean;
  chatTitle: string;
  onClose: () => void;
  onOpenContext: () => void;
  onRerunSummary: (summary: Summary) => Promise<void>;
  onRetryDelivery: (summary: Summary) => Promise<void>;
  open: boolean;
  selectedChat: Chat | null;
  selectedSummary: Summary | null;
}) {
  const selectedDelivery = selectedSummary
    ? deliveryState(selectedSummary, selectedChat, botReady)
    : null;

  return (
    <Drawer onClose={onClose} open={open}>
      {!selectedSummary ? (
        <EmptyState
          description="从列表中选择一条摘要后，这里会展示完整正文。"
          title="没有可查看的摘要"
        />
      ) : (
        <div className="summary-detail-stack">
          <div className="summary-detail-header">
            <h2>
              {chatTitle} · {selectedSummary.summaryDate}
            </h2>
          </div>
          <div className="summary-status-actions">
            <StatusPill tone={statusTone(selectedSummary.status)}>
              {statusText(selectedSummary.status)}
            </StatusPill>
            <StatusPill
              className={selectedDelivery?.detail ? "status-pill-hoverable" : undefined}
              title={selectedDelivery?.detail}
              tone={selectedDelivery?.tone ?? "neutral"}
            >
              {selectedDelivery?.label ?? "不发送"}
            </StatusPill>
            {selectedDelivery?.retryable ? (
              <button
                className="text-link-button summary-delivery-link"
                onClick={() => startTransition(() => void onRetryDelivery(selectedSummary))}
                type="button"
              >
                通过 Bot 发送
              </button>
            ) : null}
          </div>
          <div className="summary-detail-meta">
            <p>
              {selectedSummary.model || "未记录模型"} · 消息 {selectedSummary.sourceMessageCount} 条 · 分块{" "}
              {selectedSummary.chunkCount}
            </p>
            <div className="summary-detail-meta-actions">
              <button className="text-link-button" onClick={onOpenContext} type="button">
                查看原始 prompt
              </button>
              <button
                className="text-link-button"
                onClick={() => startTransition(() => void onRerunSummary(selectedSummary))}
                type="button"
              >
                重新生成
              </button>
            </div>
          </div>
          <SummaryContent summary={selectedSummary} />
        </div>
      )}
    </Drawer>
  );
}

function SummaryContent({ summary }: { summary: Summary }) {
  if (summary.status === "failed") {
    const retryLines = [
      `已重试 ${summary.retryCount || 0} 次`,
      summary.nextRetryAt
        ? `下次自动重试：${new Date(summary.nextRetryAt).toLocaleString()}`
        : "",
    ].filter(Boolean);
    const detailBlocks = [
      { title: "失败上下文", content: summary.errorContext || "" },
      { title: "失败时系统提示词", content: summary.errorSystemPrompt || "" },
      { title: "失败时用户输入", content: summary.errorUserPrompt || "" },
    ].filter((block) => block.content.trim() !== "");

    return (
      <div className="summary-detail-content">
        <pre className="summary-context-block">{summary.errorMessage || ""}</pre>
        {retryLines.length > 0 ? (
          <p className="summary-detail-meta-text">{retryLines.join(" · ")}</p>
        ) : null}
        {detailBlocks.map((block) => (
          <details className="summary-context-details" key={block.title}>
            <summary>{block.title}</summary>
            <pre className="summary-context-block">{block.content}</pre>
          </details>
        ))}
      </div>
    );
  }

  if (!summary.content) {
    return (
      <EmptyState
        description="这条摘要还没有正文，请稍后重试或重新生成。"
        title="还没有摘要内容"
      />
    );
  }

  return (
    <div className="summary-detail-content">
      <SummaryMarkdown content={summary.content} />
    </div>
  );
}
