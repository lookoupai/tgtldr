"use client";

import Markdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";

export function SummaryMarkdown({ content }: { content: string }) {
  return (
    <div className="summary-markdown" data-i18n-skip="true">
      <Markdown remarkPlugins={[remarkGfm]} urlTransform={urlTransform}>
        {content}
      </Markdown>
    </div>
  );
}

function urlTransform(url: string) {
  if (/^tg:\/\/user\?id=\d+$/i.test(url)) {
    return url;
  }
  return defaultUrlTransform(url);
}
