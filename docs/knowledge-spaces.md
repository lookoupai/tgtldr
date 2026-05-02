# Knowledge Spaces

知识空间用于把群消息中的长期信息沉淀成结构化事实。它不是固定的供需功能；每个自部署用户都可以按自己的群聊场景定义 schema、抽取规则和展示规则。

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

```json
{
  "kind": "tgtldr.knowledge-space",
  "version": 1,
  "space": {
    "name": "供需频道",
    "description": "从群消息中识别需求、供应和可匹配线索。",
    "schema": {
      "types": {
        "demand": {
          "label": "需求",
          "fields": {
            "item": "string",
            "quantity": "string",
            "budget": "string",
            "location": "string",
            "deadline": "string"
          }
        },
        "supply": {
          "label": "供应",
          "fields": {
            "item": "string",
            "quantity": "string",
            "price": "string",
            "location": "string"
          }
        }
      }
    },
    "extractPrompt": "优先抽取明确表达买、卖、求购、出售、转让、拼单、采购的信息。",
    "summaryPrompt": "摘要附加时按需求和供应分组，并保留可联系用户。",
    "confidenceThreshold": 0.75,
    "retentionDays": 30,
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
```

`/type`、`/fact`、`/facts` 适合自定义 schema 类型，例如：

```text
/type hiring remote
/type skill rust
/fact event meetup
```

Bot 只响应配置的目标会话，避免把本地知识库内容发送到未授权聊天。

## 事实合并规则

同一知识空间、群组、事实类型、标题和用户主体相同的事实会自动合并。

合并时：

- 保留最早发现时间。
- 刷新最近发现时间。
- 合并来源消息 ID 作为证据。
- 置信度保留较高值。
- 已手动忽略的事实保持 dismissed，不会因为后续抽取自动恢复。

这能减少同一用户反复表达相同需求或供应时产生的重复事实。

