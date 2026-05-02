# Knowledge Spaces

知识空间用于把群消息中的长期信息沉淀成结构化事实。它不是固定的供需功能；每个自部署用户都可以按自己的群聊场景定义 schema、抽取规则和展示规则。

系统会在没有任何知识空间时创建一个默认的“通用群聊知识库”。它适用于所有群组，并覆盖需求、供应、技能、教程、资源、风险、活动和状态变更。用户可以直接修改、停用或新建自己的知识空间。

## 适用场景

- 供需频道：记录谁需要什么、谁出售什么。
- 招聘与求职：记录岗位、候选人、内推线索。
- 技能画像：记录用户擅长领域、可提供帮助的主题。
- 活动报名：记录活动、报名用户和资源支持。
- 自定义情报：记录项目线索、工具推荐、报价、合作需求等。

## 配置字段

每个知识空间包含：

- `name`：知识空间名称。
- `description`：给使用者看的说明。
- `schema`：JSON 对象，定义事实类型和字段。
- `extractPrompt`：给 AI 的额外抽取要求。
- `summaryPrompt`：事实附加到摘要时的展示要求。
- `confidenceThreshold`：低于该置信度的事实会被丢弃。
- `retentionDays`：默认有效期；过期事实保留记录但不进入摘要和查询。
- `includeInSummary`：是否把 active 事实附加到后续摘要。

网页端“导出配置”会生成可分享的 JSON 文件。导出内容不会包含本机数据库 ID、群组绑定、事实数据、消息内容或时间戳。

## 可导入配置示例

仓库内提供了几份可直接导入的示例：

- [供需频道](examples/knowledge-spaces/marketplace.json)
- [通用群聊知识库](examples/knowledge-spaces/general.json)
- [招聘线索](examples/knowledge-spaces/hiring.json)
- [技能画像](examples/knowledge-spaces/skills.json)
- [活动报名](examples/knowledge-spaces/events.json)

下面是通用群聊知识库示例的完整结构：

```json
{
  "kind": "tgtldr.knowledge-space",
  "version": 1,
  "space": {
    "name": "通用群聊知识库",
    "description": "记录群聊中长期可复用的需求、供应、技能、教程、资源、风险和状态变化。",
    "schema": {
      "types": {
        "demand": {
          "label": "需求",
          "fields": {
            "item": "string",
            "quantity": "string",
            "budget": "string",
            "location": "string",
            "deadline": "string",
            "status": "string"
          }
        },
        "supply": {
          "label": "供应",
          "fields": {
            "item": "string",
            "quantity": "string",
            "price": "string",
            "location": "string",
            "status": "string"
          }
        },
        "skill": {
          "label": "技能",
          "fields": {
            "area": "string",
            "evidence": "string",
            "level": "string"
          }
        },
        "solution": {
          "label": "教程方法",
          "fields": {
            "topic": "string",
            "steps": "string",
            "context": "string"
          }
        },
        "resource": {
          "label": "工具资源",
          "fields": {
            "name": "string",
            "url": "string",
            "usage": "string"
          }
        },
        "risk": {
          "label": "风险避坑",
          "fields": {
            "topic": "string",
            "risk": "string",
            "mitigation": "string"
          }
        },
        "event": {
          "label": "活动机会",
          "fields": {
            "name": "string",
            "time": "string",
            "location": "string",
            "topic": "string"
          }
        },
        "status_update": {
          "label": "状态变更",
          "fields": {
            "target_type": "string",
            "target_query": "string",
            "action": "string",
            "reason": "string",
            "target_user": "string"
          }
        }
      }
    },
    "extractPrompt": "只记录未来可能复用的信息。覆盖需求、供应、技能、教程方法、工具资源、风险避坑、活动机会。技能画像必须基于用户自述、作品、持续高质量回答或明确承诺，不能凭一句闲聊推断。状态变更请用 status_update，target_type 填 demand/supply/skill/help_offer 等旧事实类型，target_query 填要失效的物品或主题，action 使用 resolved、expired、sold_out、paused、no_longer_needed 等英文短语。不要记录玩笑、猜测、纯闲聊、临时情绪或无证据结论。",
    "summaryPrompt": "摘要附加时按需求、供应、技能、教程、资源、风险、活动分组；保留可联系用户和置信度，不展示 status_update。",
    "confidenceThreshold": 0.75,
    "retentionDays": 60,
    "includeInSummary": true
  }
}
```

导入配置只会更新当前编辑表单。确认无误后，需要点击“保存知识空间”才会写入后端。

## Bot 查询

启用 Bot 并配置目标会话后，可以在该目标会话中查询知识事实：

```text
/knowledge <关键词>
/type <事实类型> <关键词>
/fact <事实类型> <关键词>
/facts <事实类型> <关键词>
/demand <关键词>
/supply <关键词>
/who <关键词>
/ask <自然语言问题>
/expire <事实ID>
/forget <事实ID>
/restore <事实ID>
/update <自然语言说明>
/confirm <确认码>
/cancel
```

`/type`、`/fact`、`/facts` 适合自定义 schema 类型，例如：

```text
/type hiring remote
/type skill rust
/fact event meetup
```

Bot 只响应配置的目标会话，避免把本地知识库内容发送到未授权聊天。

查询结果会显示事实 ID。`/expire` 会把事实标记为过期，`/forget` 会把事实标记为忽略，`/restore` 可恢复过期或忽略的事实。

`/ask` 会调用已配置的 OpenAI 兼容模型，把自然语言问题解析成关键词和事实类型，例如 `/ask 谁了解炒币` 会优先查询 `skill` 类型中和“炒币”相关的事实。

`/update` 会调用已配置的 OpenAI 兼容模型解析自然语言维护说明，例如：

```text
/update A 不再需要 Gmail 邮箱
/update @alice 的服务器已经卖完了
```

系统只会在解析出受影响用户和物品/主题后预览匹配事实；不明确时会拒绝执行，避免误伤知识库。预览通过后，需要发送 `/confirm <确认码>` 才会真正更新事实状态；发送 `/cancel` 可以取消本次维护。

## 事实合并规则

同一知识空间、群组、事实类型、标题和用户主体相同的事实会自动合并。

合并时：

- 保留最早发现时间。
- 刷新最近发现时间。
- 合并来源消息 ID 作为证据。
- 置信度保留较高值。
- 已手动忽略的事实保持 dismissed，不会因为后续抽取自动恢复。

这能减少同一用户反复表达相同需求或供应时产生的重复事实。

## 状态变更

通用模板包含 `status_update` 类型，用于识别“已买到”“不需要了”“卖完了”“暂停接单”“资源失效”等消息。系统会用这类事实匹配同一用户、同一知识空间、同一群组中仍为 active 的需求、供应、报名或服务类事实，并把匹配项标记为 expired。

`status_update` 本身不会作为普通知识展示，避免查询结果里混入维护事件。

## 维护记录

当事实被网页操作、Bot 命令、`/update` 自然语言维护或自动 `status_update` 标记为过期、忽略、恢复时，系统会写入维护记录。记录包含来源、操作、旧状态、新状态、匹配关键词和原因，方便之后排查“为什么某条知识不再出现在摘要或查询结果里”。
