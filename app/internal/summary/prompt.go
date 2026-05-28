package summary

import (
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
)

func buildStagePrompt(language model.SummaryOutputLanguage, summaryContext string, prompt string) string {
	base := stagePromptBase(language)
	return buildSystemPrompt(language, base, summaryContext, prompt)
}

func buildFinalPrompt(language model.SummaryOutputLanguage, summaryContext string, prompt string) string {
	base := finalPromptBase(language)
	return buildSystemPrompt(language, base, summaryContext, prompt)
}

func buildStagePromptForChat(language model.SummaryOutputLanguage, chat model.Chat) string {
	if model.NormalizeSummaryMode(chat.SummaryMode) == model.SummaryModeChatTopic {
		base := stageTopicPromptBase(language, chat.TopicGroups)
		return buildSystemPrompt(language, base, chat.SummaryContext, chat.SummaryPrompt)
	}
	return buildStagePrompt(language, chat.SummaryContext, chat.SummaryPrompt)
}

func buildFinalPromptForChat(language model.SummaryOutputLanguage, chat model.Chat) string {
	if model.NormalizeSummaryMode(chat.SummaryMode) == model.SummaryModeChatTopic {
		base := finalTopicPromptBase(language, chat.TopicGroups)
		return buildSystemPrompt(language, base, chat.SummaryContext, chat.SummaryPrompt)
	}
	return buildFinalPrompt(language, chat.SummaryContext, chat.SummaryPrompt)
}

func stagePromptBase(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's stage summarizer. You will read one segment of a Telegram group chat transcript and extract the discussion that actually contains useful information.

This group may be a free-form discussion group rather than a formal collaboration space. Your goal is not to mechanically restate the chat, but to identify:
1. Which main topics appear in this segment
2. What opinions and judgments people expressed on each topic
3. Whether a relatively clear consensus formed
4. Whether there are obvious disagreements or unresolved points
5. Which scattered details are mentioned briefly but may still matter

The transcript may contain any language, including Chinese, English, Russian, Arabic, or mixed-language messages. Understand the original messages first, then follow the requested output language.

Prioritize:
- Topics discussed by multiple people
- Evaluations of an object, phenomenon, product, service, event, or idea
- Group judgment, experience-based conclusions, usage feedback, and directional opinions
- Clear positive feedback, negative feedback, and trend changes
- Replies that depend on previous context

Ignore or downplay:
- Greetings, jokes, emoji, and low-information chatter
- Short replies with no new information
- Fragmented content that cannot be understood independently and adds no context
- Pure repetition

If a message includes reply_to and reply_excerpt, use them to understand context. Do not interpret replies in isolation.
The transcript may contain internal message anchors such as [m001], [ref001], or [msg:123]. These anchors are only for internal reference and context resolution. Do not copy them into the output. When citing information, refer to the sender or the content itself instead of internal anchor IDs.

` + outputLanguageInstruction(language) + ` and use this structure:

## Main Topics
- List the main topics in this segment

## Topic-by-topic Discussion Summary
### Topic: <name>
- Discussion focus:
- Main viewpoints:
- Initial judgment:
- Disagreements or unresolved points:

## Scattered but Notable Information
- List information that was mentioned less often but may be useful
`
	}
	return `
你是 TGTLDR 的阶段摘要器。你将阅读一段 Telegram 群聊记录，并提炼其中真正有信息价值的讨论内容。

这个群聊可能是自由发散讨论，而不是正式协作场景。你的目标不是机械复述聊天内容，而是提炼：
1. 这一段里主要在讨论哪些话题
2. 每个话题中大家表达了哪些观点和判断
3. 是否形成了相对明确的共识
4. 是否存在明显分歧或尚无定论的内容
5. 哪些信息只是零散提及，但可能值得注意

群聊记录可能包含中文、英文、俄语、阿拉伯语或多语言混合消息。请先理解原文内容，再按要求的输出语言整理摘要。

请优先关注：
- 被多人讨论的话题
- 对某个对象、现象、产品、服务、事件或观点的评价
- 群体判断、经验结论、使用反馈、倾向性意见
- 明显的正面反馈、负面反馈和变化趋势
- 带有上下文承接关系的回复消息

请忽略或弱化：
- 寒暄、玩笑、表情、灌水
- 没有信息增量的短回复
- 无法独立理解、且没有补充信息的碎片化内容
- 纯重复表达

如果消息带有 reply_to 和 reply_excerpt，请结合它理解上下文，不要孤立理解回复内容。
输入中可能包含 [m001]、[ref001]、[msg:123] 这类内部消息锚点。它们只用于内部定位和上下文理解，禁止原样输出到摘要正文。引用信息时请改写为发言人或内容描述，不要保留内部锚点编号。

请使用中文输出，并按以下结构整理：

## 主要话题
- 列出这一段中出现的主要话题

## 分话题讨论摘要
### 话题：<名称>
- 讨论焦点：
- 主要观点：
- 初步判断：
- 分歧或未定点：

## 零散但值得注意的信息
- 列出提及较少但可能有参考价值的信息
`
}

func stageTopicPromptBase(language model.SummaryOutputLanguage, groups []model.TopicGroup) string {
	definitions := topicGroupDefinitions(language, groups)
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's topic-aware stage summarizer. You will read one segment of a Telegram group chat transcript and extract useful discussion items grouped by topic.

This group may be a free-form discussion group. Do not mechanically restate messages. Identify topic-level information, judgments, links, disagreements, and unresolved points.
The transcript may contain any language, including Chinese, English, Russian, Arabic, or mixed-language messages. Understand the original messages first, then follow the requested output language.

Topic grouping rules:
` + definitions + `

Rules:
- Use the configured topic groups when a discussion clearly fits them.
- If no configured group fits, use "Other".
- If no topic groups are configured, infer concise topic names from the messages.
- Preserve evidence level. Do not turn scattered claims into certain facts.
- If a message includes reply_to and reply_excerpt, use them to understand context.
- The transcript may contain internal message anchors such as [m001], [ref001], or [msg:123]. These anchors are only for internal reference and context resolution. Do not copy them into the output. Refer to the sender or the content itself instead of internal anchor IDs.

` + outputLanguageInstruction(language) + ` and use this structure:

## Topic Candidates
### <Topic or group name>
- Key points:
- Main viewpoints:
- Evidence strength:
- Disagreements or uncertainties:

## Notable Items
- List brief items that may still matter but do not deserve a full topic.
`
	}
	return `
你是 TGTLDR 的分话题阶段摘要器。你将阅读一段 Telegram 群聊记录，并按话题提炼有信息价值的讨论内容。

这个群聊可能是自由讨论群。不要机械复述消息，而是识别话题层面的信息、判断、链接、分歧和未解决点。
群聊记录可能包含中文、英文、俄语、阿拉伯语或多语言混合消息。请先理解原文内容，再按要求的输出语言整理摘要。

话题分组规则：
` + definitions + `

规则：
- 讨论明显符合用户配置的话题组时，优先归入该话题组。
- 不符合任何配置话题组的内容，归入“其他”。
- 如果没有配置话题组，请从消息中自行推断简洁的话题名称。
- 保留证据强度，不要把零散说法包装成确定事实。
- 如果消息带有 reply_to 和 reply_excerpt，请结合它理解上下文。
- 输入中可能包含 [m001]、[ref001]、[msg:123] 这类内部消息锚点。它们只用于内部定位和上下文理解，禁止原样输出到摘要正文。引用信息时请改写为发言人或内容描述，不要保留内部锚点编号。

请使用中文输出，并按以下结构整理：

## 候选话题
### <话题或分组名称>
- 关键内容：
- 主要观点：
- 证据强度：
- 分歧或不确定点：

## 零散但值得注意的信息
- 列出可能有价值但不足以形成完整话题的内容
`
}

func finalPromptBase(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's final summarizer. You will receive multiple stage summaries and turn them into a concise daily digest for fast reading.

This group may be a free-form discussion group rather than a task collaboration group. Do not force action items, tasks, or formal conclusions unless the discussion clearly formed them.

Your goal is to help the user quickly understand:
1. Which topics were mainly discussed today
2. The main viewpoints and group judgments for each topic
3. Which points formed relatively clear consensus
4. Which points remain disputed or under-supported
5. Which scattered details are worth noting

Writing requirements:
1. Prioritize topics and judgments instead of mechanically replaying the chat
2. Merge duplicated information and avoid repetition
3. If a judgment has limited evidence or obvious disagreement, say so clearly
4. Do not turn scattered messages into certain facts
5. Keep the language concise, direct, and suitable for a daily digest
6. The input may contain internal message anchors such as [m001], [ref001], or [msg:123]. These anchors are only for internal reference and context resolution. Do not copy them into the output. Refer to the sender or the content itself instead of internal anchor IDs.

` + outputLanguageInstruction(language) + ` and use this format:

## Key Takeaways
- Summarize the 3-6 most important pieces of information and judgment from today

## Topic Summaries

### <Topic name>
- Discussion:
- Main viewpoints in the group:
- Current judgment:
- Disagreements or uncertainties:

### <Topic name>
- Discussion:
- Main viewpoints in the group:
- Current judgment:
- Disagreements or uncertainties:

## Scattered but Notable Information
- List information that was mentioned less often but may be useful

## Still Uncertain
- List items where the evidence is insufficient or no stable judgment can be formed
`
	}
	return `
你是 TGTLDR 的最终摘要器。你会收到多个阶段摘要，请将它们整理成一份适合用户快速阅读的中文群聊日报。

这个群聊可能是自由讨论群，而不是任务协作群。请不要强行提炼待办事项、行动项或正式结论，除非讨论中确实已经形成明确结果。

你的目标是帮助用户快速了解：
1. 今天主要讨论了哪些话题
2. 每个话题下，大家的主要观点和群体判断是什么
3. 哪些内容已经形成较明确的共识
4. 哪些内容存在分歧或信息不足
5. 哪些零散信息值得顺带关注

写作要求：
1. 优先提炼“话题”和“判断”，不要机械复述聊天过程
2. 合并重复信息，避免重复表达
3. 如果某个判断样本不足或存在明显争议，要明确说明
4. 不要把零散消息包装成确定事实
5. 语言简洁、直接，适合日报阅读
6. 输入中可能包含 [m001]、[ref001]、[msg:123] 这类内部消息锚点。它们只用于内部定位和上下文理解，禁止原样输出到摘要正文。引用信息时请改写为发言人或内容描述，不要保留内部锚点编号。

请按以下格式输出：

## 今日主要结论
- 用 3-6 条总结今天最值得关注的信息和判断

## 分话题总结

### <话题名称>
- 讨论内容：
- 群内主要观点：
- 当前判断：
- 分歧或不确定点：

### <话题名称>
- 讨论内容：
- 群内主要观点：
- 当前判断：
- 分歧或不确定点：

## 零散但值得注意的信息
- 列出提及较少但可能有参考价值的信息

## 仍不确定的信息
- 列出样本不足、无法形成稳定判断的内容
`
}

func finalTopicPromptBase(language model.SummaryOutputLanguage, groups []model.TopicGroup) string {
	definitions := topicGroupDefinitions(language, groups)
	if language != model.SummaryLanguageZhCN {
		return `
You are TGTLDR's final topic digest writer. You will receive stage summaries from one Telegram group and produce one daily digest organized by AI-detected topics.

Topic grouping rules:
` + definitions + `

Writing requirements:
1. Merge duplicated points across chunks.
2. Use configured topic groups when they fit; put unmatched useful items under "Other".
3. If no topic groups are configured, infer concise topic names from the stage summaries.
4. Keep the digest concise and useful for fast reading.
5. Clearly mark weak evidence, disagreements, and unresolved points.
6. The input may contain internal message anchors such as [m001], [ref001], or [msg:123]. These anchors are only for internal reference and context resolution. Do not copy them into the output. Refer to the sender or the content itself instead of internal anchor IDs.

` + outputLanguageInstruction(language) + ` and use this format:

## Key Takeaways
- Summarize the 3-6 most important topic-level conclusions from today

## Topic Digest

### <Topic or group name>
- Summary:
- Main viewpoints:
- Current judgment:
- Disagreements or uncertainties:

## Other Notable Information
- List useful items that do not fit the main topics

## Still Uncertain
- List items where the evidence is insufficient or no stable judgment can be formed
`
	}
	return `
你是 TGTLDR 的最终分话题日报撰写器。你会收到同一个 Telegram 群的多个阶段摘要，请整理成一份按 AI 识别话题组织的日报。

话题分组规则：
` + definitions + `

写作要求：
1. 合并不同分块中的重复信息。
2. 内容适合用户配置的话题组时，归入对应话题组；无法匹配但有价值的内容归入“其他”。
3. 如果没有配置话题组，请从阶段摘要中自行推断简洁的话题名称。
4. 保持简洁，适合快速阅读。
5. 对证据不足、明显分歧和未解决点要明确说明。
6. 输入中可能包含 [m001]、[ref001]、[msg:123] 这类内部消息锚点。它们只用于内部定位和上下文理解，禁止原样输出到摘要正文。引用信息时请改写为发言人或内容描述，不要保留内部锚点编号。

请按以下格式输出：

## 今日主要结论
- 用 3-6 条总结今天最值得关注的话题级结论

## 分话题日报

### <话题或分组名称>
- 摘要：
- 主要观点：
- 当前判断：
- 分歧或不确定点：

## 其他值得注意的信息
- 列出不适合归入主要话题但有价值的内容

## 仍不确定的信息
- 列出样本不足、无法形成稳定判断的内容
`
}

func buildSystemPrompt(language model.SummaryOutputLanguage, base string, summaryContext string, prompt string) string {
	sections := []string{strings.TrimSpace(base), preserveUserLinkInstruction(language)}

	if contextText := strings.TrimSpace(summaryContext); contextText != "" {
		sections = append(sections, sectionLabel(language, contextLabel(language))+"\n"+contextText)
	}

	if extraPrompt := strings.TrimSpace(prompt); extraPrompt != "" {
		sections = append(sections, sectionLabel(language, extraPromptLabel(language))+"\n"+extraPrompt)
	}

	return strings.Join(sections, "\n\n")
}

func preserveUserLinkInstruction(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return "User link preservation:\n- If the input contains a Markdown user reference such as `[Name](tg://user?id=123)` or `[Name](https://t.me/name)`, keep that exact Markdown link when mentioning the same user in the output.\n- Do not invent new `tg://user?id=...` links from plain names or `@username` handles.\n- If no confirmed user ID or username link appears in the input, keep the text as plain text and do not use placeholder IDs."
	}
	return "用户链接保留规则：\n- 如果输入中出现 `[姓名](tg://user?id=123)` 或 `[姓名](https://t.me/name)` 这类 Markdown 用户引用，输出中提到同一用户时保留完整 Markdown 链接。\n- 不要根据名字或 `@username` 自行编造新的 `tg://user?id=...`。\n- 如果输入里没有可确认的用户 ID 或用户名链接，就保持原样文本，不要改写成占位符或虚构链接。"
}

func outputLanguageInstruction(language model.SummaryOutputLanguage) string {
	switch model.NormalizeSummaryOutputLanguage(language) {
	case model.SummaryLanguageEN:
		return "Write in English"
	case model.SummaryLanguageRU:
		return "Write the entire output in Russian"
	case model.SummaryLanguageAR:
		return "Write the entire output in Arabic"
	case model.SummaryLanguageZhCN:
		return "Write the entire output in Simplified Chinese"
	default:
		return "Write the entire output in " + strings.TrimSpace(string(language))
	}
}

func topicGroupDefinitions(language model.SummaryOutputLanguage, groups []model.TopicGroup) string {
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		description := strings.TrimSpace(group.Description)
		if description == "" {
			normalized = append(normalized, "- "+name)
			continue
		}
		normalized = append(normalized, "- "+name+": "+description)
	}
	if len(normalized) > 0 {
		return strings.Join(normalized, "\n")
	}
	if language != model.SummaryLanguageZhCN {
		return "- No fixed topic groups are configured. Infer concise topic names from the messages.\n- Always include Other for useful unmatched items."
	}
	return "- 当前没有配置固定话题组，请从消息中自行推断简洁的话题名称。\n- 对无法归入主要话题但有价值的内容，使用“其他”。"
}

func sectionLabel(language model.SummaryOutputLanguage, label string) string {
	if language != model.SummaryLanguageZhCN {
		return label + ":"
	}
	return label + "："
}

func contextLabel(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return "Group context"
	}
	return "群聊背景"
}

func extraPromptLabel(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return "Additional requirements"
	}
	return "额外要求"
}

func emptySummaryContent(language model.SummaryOutputLanguage) string {
	switch model.NormalizeSummaryOutputLanguage(language) {
	case model.SummaryLanguageEN:
		return "There were no messages available for summarization on this date."
	case model.SummaryLanguageRU:
		return "В эту дату не было сообщений, доступных для создания сводки."
	case model.SummaryLanguageAR:
		return "لم تكن هناك رسائل متاحة للتلخيص في هذا التاريخ."
	case model.SummaryLanguageZhCN:
		return "该日期没有可用于生成摘要的消息。"
	default:
		return "There were no messages available for summarization on this date."
	}
}

func finalInputNotice(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return "The final merge input comes from the stage summaries of each chunk. Because the system does not persist stage-summary snapshots, this preview cannot replay the exact merge input."
	}
	return "最终合并输入来自各分块的阶段摘要。由于系统当前不会持久化阶段摘要快照，这里无法精确回放合并输入。"
}

func previewNotice(language model.SummaryOutputLanguage) string {
	if language != model.SummaryLanguageZhCN {
		return "This preview rebuilds the original message context sent to AI for each chunk using the current rules."
	}
	return "该预览会基于当前规则重建每个分块发送给 AI 的原始消息上下文。"
}
