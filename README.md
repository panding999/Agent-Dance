# Agent Dance

Agent Dance 是一款面向英语演讲、技术分享、国际会议、网课和会议录制的 AI 同声传译助手。系统将单向外语音频流实时转换为中文字幕，并在后续上下文确认后自动回写修正早期识别或翻译错误，帮助用户跟上内容节奏。

## 核心能力

- 实时同传：浏览器采集麦克风音频，经后端网关接入同声传译模型，实时输出中文字幕。
- 上传回放：上传音频或视频文件后生成带时间轴的中文字幕；视频模式支持在播放画面底部叠加字幕。
- 自动纠错：当后续上下文确认术语、专名、数字或长句翻译有误时，系统生成修正事件并保留前后对比。
- 统一事件协议：前端消费统一的字幕事件，后端负责音频格式、模型协议、鉴权和修正逻辑。

## 技术栈

- 前端：Next.js App Router、React、TypeScript、Tailwind CSS、Motion、Phosphor Icons。
- 后端：Go HTTP/WebSocket 网关，负责会话管理、实时音频链路、上传处理和字幕事件输出。
- 存储：SQLite 起步，后续可按部署需要迁移到 PostgreSQL。
- 外部依赖：Doubao/火山引擎同声传译与语音识别能力，上传处理链路依赖 ffmpeg。

## 项目结构

```text
app/                         Next.js 页面入口与全局样式
components/                  前端工作台组件
backend/                     Go 后端服务
docs/                        技术方案与后端实施拆解
PRODUCT.md                   产品定位与界面设计原则
AGENTS.md                    协作、提交和验证规范
```

## 本地启动

安装前端依赖并启动页面：

```powershell
Copy-Item .env.example .env.local
npm install
npm run dev
```

构建与静态检查：

```powershell
npm run lint
npm run build
```

复制后端示例配置，补齐 Doubao 凭据、数据库路径和上传目录后启动服务：

```powershell
cd backend
Copy-Item .env.example .env
go run ./cmd/server
```

后端测试：

```powershell
cd backend
go test ./...
```

## Docker Compose 启动

Docker Compose 会统一构建并启动前端与后端：

```powershell
docker compose up --build -d
```

启动后访问：

- 前端工作台：`http://localhost:3000`
- 后端健康检查：`http://localhost:8080/healthz`
- 后端就绪检查：`http://localhost:8080/readyz`

查看日志和停止服务：

```powershell
docker compose logs -f
docker compose down
```

未配置 Doubao 凭据时，页面和健康检查可以正常访问；点击开始同传会明确提示需要配置凭据。需要真实同传时，在仓库根目录创建不会提交的 `.env`，至少配置一种 Doubao 鉴权方式：

```dotenv
DOUBAO_API_KEY=replace-with-real-api-key
DOUBAO_AST_RESOURCE_ID=volc.service_type.10053
```

SQLite 数据和上传目录分别保存在 `backend-runtime`、`backend-uploads` Docker 命名卷中。当前真实功能链路仅支持麦克风实时同传；音频和视频上传回放仍是前端演示数据，后端上传处理接口尚未实现。

## 接口概览

- `POST /api/sessions`：创建同传会话。
- `GET /api/sessions/{id}`：查询会话状态。
- `GET /api/live/ws`：实时音频与字幕事件 WebSocket。

前端实时同传模式已接入后端会话和 WebSocket 字幕事件；上传回放模式当前仍使用产品化展示数据，等待上传处理接口落地后再切换到真实数据源。

## 文档

- [后端接口文档](docs/backend-api-reference.md)
- [AI 同声传译助手技术方案](docs/ai-simultaneous-interpretation-technical-solution.md)
- [Go 后端实施拆解](docs/backend-go-implementation-plan.md)
