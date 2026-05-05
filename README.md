# TGTLDR

[English version](README.en.md)

TGTLDR （Telegram Too Long, Don't Read）是一个单用户自部署的 Telegram 群消息监听与每日 AI 摘要系统。

这个项目被构建出来的原因是：许多 Telegram 群聊都是超级大群，每天会产生数千条消息。有时我们只想了解一些最新的情报，而并不希望花大量的时间在水群上。使用这个工具，就能为你在每天的固定时间推送前一天的最新群聊结论。

![TGTLDR 首页截图](docs/images/home-zh.png)

## 功能特性

- 监听已加入的 Telegram 群组消息，并保存到本地数据库
- 按群组配置每日摘要时间、Prompt、过滤规则和摘要模型
- 支持按频道整体摘要，或按 AI 识别的话题/自定义话题分组摘要
- 支持阅读多语言频道内容，并可配置默认摘要输出语言或按群组覆盖输出语言
- 使用 OpenAI 兼容接口生成群聊摘要
- 支持在网页端查看摘要，也可以选择通过 Telegram Bot 推送；超长 Bot 消息会自动拆分发送
- 支持手动触发摘要、查看历史摘要和重新投递失败的 Bot 推送
- 支持自定义知识空间，从群消息中抽取长期有效的结构化事实和用户画像
- 支持通过网页或 Bot 命令查询知识事实，例如需求、供应、技能、活动等自定义类型
- 提供首次配置向导，启动后可在网页端完成 Telegram、OpenAI 和群组设置

## 使用前准备

- Docker 和 Docker Compose（推荐启动方式）
- Telegram `api_id` 和 `api_hash`，可在 [my.telegram.org/apps](https://my.telegram.org/apps) 申请
- OpenAI 兼容接口的 Base URL、API Key 和模型名
- 可选：Telegram Bot Token，用于把摘要推送回 Telegram

## 本地启动

### 推荐：使用预构建镜像启动（同时启动前端、后端和数据库）

```bash
cp .env.example .env
docker compose up -d
```

如果你没有显式设置 `TGTLDR_MASTER_KEY`，系统会在首次启动时自动生成一把随机主密钥，并把它持久化到 app 容器的数据卷中。

如果你想拉取指定版本的镜像，可以在启动前设置：

```bash
export TGTLDR_IMAGE_NAMESPACE=lookoupai
export TGTLDR_IMAGE_TAG=latest
docker compose up -d
```

如果宿主机的 `3000` 端口已被占用，或者你希望监听所有网卡而不是仅监听本机，可以在 `.env` 中覆盖：

```bash
cp .env.example .env
# 编辑 .env，将下面这些项改成你想使用的值：
# TGTLDR_HOST_BIND=0.0.0.0
# TGTLDR_HOST_WEB_PORT=13000
docker compose up -d
```

其中：

- `TGTLDR_HOST_BIND=127.0.0.1` 表示只监听本机，适合默认本地使用
- `TGTLDR_HOST_BIND=0.0.0.0` 表示监听所有网卡，适合部署到服务器或 NAS

`TGTLDR_MASTER_KEY` 是本地数据加密主密钥，用来加密保存 Telegram 登录 session、OpenAI API Key 和 Bot Token。它不会发送给外部服务。默认情况下，这把 key 会保存在 app 数据卷中的 `/var/lib/tgtldr/master.key`；如果你删除了这个数据卷，已经保存的这些敏感数据将无法解密。

启动后访问：

- 前端：`http://localhost:${TGTLDR_HOST_WEB_PORT}`（默认 `http://localhost:3000`）

首次访问前端后，按照页面向导完成访问密码、Telegram、OpenAI 和群组摘要配置即可。

## 运行维护

查看容器状态：

```bash
docker compose ps
```

查看日志：

```bash
docker compose logs -f app
docker compose logs -f web
docker compose logs -f postgres
```

检查后端健康状态：

```bash
curl http://127.0.0.1:3000/api/health
```

升级到最新预构建镜像：

```bash
docker compose pull
docker compose up -d
```

如果升级后页面行为异常，优先查看 `app` 日志确认数据库迁移和后端启动是否成功。

## 摘要与知识空间

每个群组都可以单独配置摘要行为：

- `按频道`：默认模式，把前一天消息汇总成一份摘要
- `按话题`：AI 会按讨论主题分组；也可以在群组配置中预设话题名称和描述
- `摘要输出语言`：系统默认支持中文、英文、俄语、阿拉伯语，也可以填写自定义语言名；群组留空时跟随全局默认值
- Bot 推送会遵守 Telegram 单条消息长度限制，超过 4096 可见字符时自动按顺序拆分

知识空间用于维护长期信息，而不是只生成一次性摘要。每个知识空间都有独立的 JSON schema、抽取要求、适用群组、置信度阈值和保留天数，因此可以用于供需、招聘、技能画像、活动报名、项目线索等不同场景。系统会在没有任何知识空间时自动创建一个通用模板，后续可直接改成自己的规则。

同一知识空间、群组、事实类型、标题和用户主体相同的事实会自动合并，系统会保留最早发现时间、刷新最近发现时间，并合并来源消息作为证据。

如果启用 Bot 并配置目标会话，可以在该会话中直接查询知识库：

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

在私聊目标会话里，也可以直接发送自然语言问题，系统会按 `/ask` 处理。目标会话是群聊时，Bot 会响应明确的命令、`@BotName 问题`，以及对 Bot 消息的回复；普通群消息会被忽略。

其中 `/type` 可用于查询自定义 schema 类型，例如 `/type hiring remote` 或 `/type skill rust`。`/fact` 和 `/facts` 是同等用途的别名。
查询结果会显示事实 ID，可用 `/expire` 标记过期、`/forget` 忽略、`/restore` 恢复。
`/ask 谁了解炒币` 会先把自然语言问题解析成关键词和事实类型，检索知识库后再基于匹配事实生成回答，并在回答中引用事实 ID；如果没有足够证据，会明确说明。也可以用 `/update A 不再需要 Gmail 邮箱` 这类自然语言维护命令，系统会先解析受影响用户和物品，预览匹配事实，并要求 `/confirm <确认码>` 后才更新。事实被网页、Bot 或自动状态变更维护时会留下审计记录，便于追踪旧状态、新状态和触发来源。

Bot 只会响应配置的目标会话，避免把本地知识库内容发送到未授权聊天。

### 开发者：本地 Docker 构建启动

如果你需要在本地修改代码并重新构建镜像，请使用开发 override：

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

### 手动开发启动

如果你已经使用 Docker 启动，不需要执行本节。手动方式适合开发调试，需要你自行准备 PostgreSQL、Go 和 Node.js 环境。

启动后端：

```bash
cd app
export TGTLDR_DATABASE_URL='postgres://postgres:postgres@localhost:5432/tgtldr?sslmode=disable'
export TGTLDR_MASTER_KEY_FILE="$HOME/.tgtldr/master.key"
export TGTLDR_MASTER_KEY='替换为 openssl rand -base64 32 生成的值'
go run ./cmd/server
```

启动前端：

```bash
cd web
npm install
TGTLDR_INTERNAL_API_BASE_URL=http://127.0.0.1:8080 npm run dev
```

## 安全提示

- `TGTLDR_MASTER_KEY` 用于加密保存 Telegram session、OpenAI API Key 和 Bot Token。
- 如果你不显式设置 `TGTLDR_MASTER_KEY`，系统会自动生成一把随机 key，并持久化到 `/var/lib/tgtldr/master.key`。
- 请妥善保存这把 key 或对应的数据卷；如果丢失，已经保存到数据库里的密钥和 Telegram session 将无法解密。
- 建议只部署在本机或可信内网；如果要暴露到公网，请先确认已经完成访问密码设置，并放在可信反向代理之后。

## 反向代理部署

如果你准备通过反向代理对外提供服务，请先在 `.env` 中配置这些值：

```env
TGTLDR_HOST_BIND=0.0.0.0
TGTLDR_WEB_ORIGIN=https://tgtldr.example.com
TGTLDR_HOST_WEB_PORT=13000
```

其中：

- `TGTLDR_HOST_BIND`：让容器监听服务器上的所有网卡
- `TGTLDR_WEB_ORIGIN`：填写用户实际访问的公网地址
- `TGTLDR_HOST_WEB_PORT`：反向代理转发到的本机端口

然后启动服务：

```bash
cp .env.example .env
# 编辑 .env
docker compose up -d
```

反向代理只需要转发到 `TGTLDR_HOST_WEB_PORT` 对应的本机端口即可。

Nginx 示例（假设 `TGTLDR_HOST_WEB_PORT=13000`）：

```nginx
server {
    listen 80;
    server_name tgtldr.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tgtldr.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:13000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## 镜像发布

- 默认 `docker-compose.yml` 面向普通用户，直接使用预构建镜像。
- `docker-compose.dev.yml` 面向开发者，保留本地 build 工作流。
- GitHub Actions 会在推送 `main` 或 `v*` tag 时，自动构建并推送：
  - `lookoupai/tgtldr-app`
  - `lookoupai/tgtldr-web`

## License

本项目使用 [PolyForm Noncommercial License 1.0.0](LICENSE)。

你可以基于非商业目的使用、fork、修改和分发本项目。商业使用需要获得作者单独授权。

## 文档

- [架构方案](docs/ARCHITECTURE.md)
- [产品流程与实施计划](docs/PRODUCT_FLOW.md)
- [知识空间配置与示例](docs/knowledge-spaces.md)
