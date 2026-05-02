"use client";

import { useEffect, useState } from "react";
import { AppSelect } from "@/components/app-select";
import { Input } from "@/components/ui";
import { SummaryOutputLanguage } from "@/lib/types";

const customValue = "__custom";
const inheritValue = "__inherit";

const commonSummaryLanguages = [
  { value: "zh-CN", label: "中文" },
  { value: "en", label: "English" },
  { value: "ru", label: "Русский" },
  { value: "ar", label: "العربية" },
];

export function SummaryLanguageControl({
  includeInherit = false,
  onChange,
  value,
}: {
  includeInherit?: boolean;
  onChange: (value: SummaryOutputLanguage) => void;
  value: SummaryOutputLanguage;
}) {
  const [customMode, setCustomMode] = useState(false);
  const normalized = String(value ?? "").trim();
  const hasCommonValue = commonSummaryLanguages.some(
    (option) => option.value === normalized,
  );
  const isCustom = customMode || (normalized !== "" && !hasCommonValue);
  const selectValue = isCustom
    ? customValue
    : includeInherit && normalized === ""
      ? inheritValue
      : normalized || "zh-CN";
  const options = [
    ...(includeInherit ? [{ value: inheritValue, label: "跟随全局" }] : []),
    ...commonSummaryLanguages,
    { value: customValue, label: "自定义" },
  ];

  useEffect(() => {
    if (hasCommonValue) {
      setCustomMode(false);
    }
  }, [hasCommonValue, normalized]);

  return (
    <div className="form-stack">
      <AppSelect
        onChange={(nextValue) => {
          if (nextValue === inheritValue) {
            setCustomMode(false);
            onChange("");
            return;
          }
          if (nextValue === customValue) {
            setCustomMode(true);
            onChange(isCustom ? normalized : "");
            return;
          }
          setCustomMode(false);
          onChange(nextValue);
        }}
        options={options}
        value={selectValue}
      />
      {selectValue === customValue ? (
        <Input
          placeholder="例如 Japanese、Deutsch、Español"
          value={normalized}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : null}
    </div>
  );
}
