# AGENTS.md

本文件适用于整个仓库。它定义 Agent-Dance 项目的协作、提交、PR、忽略文件和验证规范。两人协作时优先遵守这里的规则；如果某个子目录后续新增更近的 `AGENTS.md`，以更近文件为准。

## 项目定位

Agent-Dance 是一个豆包优先的 AI 同声传译助手。首版目标是比赛/展示版：

- 实时模式：麦克风音频进入前端，前端转 16kHz/16bit/单声道 PCM，后端 Go 网关接入豆包 AST，输出中文字幕和可选中文语音。
- 上传模式：上传网课/演讲音视频，后端抽取音频，调用豆包录音文件识别和翻译链路，生成双语字幕回放。
- 修正能力：后台复核数字、术语、专名和长句，生成 `segment.revision`，保留修正历史。

当前设计文档入口：

- `docs/ai-simultaneous-interpretation-technical-solution.md`
- `docs/backend-go-implementation-plan.md`

## 技术边界

推荐技术栈：

- 前端：Next.js + React + TypeScript。
- 后端：Go，负责实时音频网关、豆包 AST WebSocket/protobuf、上传处理、字幕稳定和后台修正。
- 存储：SQLite 起步，后续可迁移 PostgreSQL。
- 运行依赖：ffmpeg、豆包 AST、豆包录音文件识别、豆包语音合成。

不要把豆包 App Key、Access Key、Resource ID、临时 token、音频样本、上传视频、识别日志或用户字幕结果提交到仓库。

## 两人协作规则

- 每个人基于独立分支开发，不直接在 `main` 上写功能。
- 开始开发前先同步远程：`git fetch origin --prune`，确认当前分支和工作区状态。
- 认领任务时明确范围，避免两个人同时长期修改同一批文件。
- 如果需要改同一个模块，先约定边界。例如一个人改 `internal/doubao/ast`，另一个人改 `internal/subtitle`。
- 不要覆盖、revert、stash 另一个人的改动；发现冲突先沟通。
- 文档、前端、后端可以并行，但涉及接口协议时先更新协议文档或类型定义。
- 每天结束前尽量推送当前分支；未完成工作用 draft PR 或清晰 commit message 说明状态。

## 分支规范

分支名使用小写英文和短横线：

```text
docs/doubao-architecture
frontend/live-console
backend/live-gateway
backend/doubao-ast-bridge
backend/upload-processing
fix/audio-packetizer
chore/project-setup
```

避免使用：

- `test`
- `new`
- `final`
- `my-branch`
- `codex/<short-description>`
- 含空格或中文的分支名。

## Commit 规范

使用 Conventional Commits：

```text
<type>(<scope>): <summary>
```

常用类型：

- `feat`：新增用户可见功能。
- `fix`：修复缺陷。
- `docs`：文档变更。
- `test`：测试新增或调整。
- `refactor`：不改变行为的结构调整。
- `chore`：工程配置、脚手架、依赖、工具调整。
- `perf`：性能优化。

推荐 scope：

- `frontend`
- `backend`
- `live`
- `doubao`
- `audio`
- `subtitle`
- `repair`
- `upload`
- `docs`

示例：

```text
docs(pr): add collaboration and review conventions
feat(live): add browser audio websocket gateway
feat(doubao): add AST session start request
fix(audio): reject misaligned pcm frames
test(subtitle): cover final segment stabilization
chore(project): add gitignore for generated media
```

Commit 要求：

- 一个 commit 表达一个清晰意图。
- 不要把文档规范、后端功能、前端样式和依赖升级混在同一个 commit。
- 不提交未验证的半成品到 `main`；半成品可以提交到个人分支，但 message 必须明确。
- commit 前运行与改动相关的最小验证命令。
- 不提交生成的大文件、音视频、数据库、日志、缓存和密钥。

## PR 规范

PR 必须小粒度。一个 PR 只处理一个明确主题：

- 一份技术方案文档。
- 一个后端模块。
- 一个前端页面。
- 一个修复。
- 一个测试补充。

大功能拆成多个 PR，例如：

1. `backend/live-gateway`：浏览器 WebSocket 和 PCM 校验。
2. `backend/doubao-ast-bridge`：豆包 AST 连接和 protobuf。
3. `backend/subtitle-normalizer`：事件归一化和字幕稳定。
4. `backend/upload-processing`：上传和录音识别。
5. `frontend/live-console`：同传工作台 UI。

PR 标题格式：

```text
[type] short summary
```

示例：

```text
[docs] Add Doubao-first technical solution
[backend] Add live audio gateway
[frontend] Add realtime subtitle console
[fix] Correct PCM packet duration calculation
```

PR 描述必须包含：

```markdown
## Summary

- 改了什么。
- 为什么要改。

## Implementation

- 核心实现思路。
- 涉及的主要模块。

## Validation

- 运行过的命令。
- 手动验证步骤。
- 未验证的原因，如果有。

## Risk

- 可能影响什么。
- 回滚方式或降级方式。

## Demo

- 如果是可见功能，提供截图、GIF、视频链接或明确的复现步骤。
```

合并要求：

- 至少另一位协作者 review。
- `main` 合并后必须保持可运行或可演示。
- PR 中不能包含密钥、私有音视频、数据库、日志或大体积生成物。
- 如果 PR 改了接口协议，需要同步更新文档或类型定义。
- 如果 PR 改了用户可见功能，需要提供截图、录屏或清晰复现步骤。

## 验证规范

按改动范围选择最小但足够的验证方式。

通用：

```powershell
git status -sb
git diff --check
```

Go 后端：

```powershell
go test ./...
go run ./cmd/server
```

前端：

```powershell
npm install
npm run lint
npm test
npm run build
```

文档：

```powershell
git diff --check
```

真实豆包调用不要默认跑进 CI。需要真实凭据或消耗额度的验证必须显式标记，例如：

```powershell
$env:RUN_DOUBAO_SMOKE="1"
go test ./internal/doubao/... -run Smoke
```

## .gitignore 规则

仓库应忽略：

- 前端依赖和构建产物：`node_modules/`、`.next/`、`dist/`、`build/`。
- Go 构建产物：`bin/`、`*.exe`、`*.test`、`coverage.out`。
- 环境和密钥：`.env`、`.env.*`、`secrets/`、证书和私钥。
- 本地数据库：`*.db`、`*.sqlite`、`data/`、`runtime/`。
- 用户上传、音视频、日志、缓存：`uploads/`、`cache/`、`logs/`、`*.mp4`、`*.wav` 等。
- 本地 Agent 状态：`.agent/`、`.superpowers/`、`.codex/`、`.worktrees/`。
- 编辑器和系统文件：`.vscode/`、`.idea/`、`.DS_Store`、`Thumbs.db`。

允许提交：

- `README.md`
- `docs/**/*.md`
- `.env.example`
- 小型、无隐私、明确用于测试的文本 fixture。

如果必须加入音频或视频 demo，不要直接提交大文件。把视频上传到外部平台或云盘，在 README 或 PR 描述里提供可访问链接。

## Agent 工作规则

`AGENTS.md` 是仓库级协作规范，推荐提交到仓库。它让两位协作者和后续 Agent 使用同一套分支、PR、验证和安全规则，避免每次任务重新解释项目约束。

Agent 在本仓库工作时必须：

- 开始前读取本文件、README 和相关 docs。
- 先确认任务范围、目标子目录和已有未提交改动。
- 不覆盖用户或队友已有改动。
- 修改前说明将改哪些文件。
- 面向用户的沟通、总结、PR 描述和提交说明默认使用中文；代码标识符、命令、协议字段、第三方 API 名称和必须保留的英文原文除外。
- 使用小步、局部、可验证的修改。
- 优先维护文档和接口协议，避免代码与方案漂移。
- 不运行真实豆包请求、长任务、删除操作或大文件生成，除非用户明确要求。
- 完成前运行相关验证命令，并在回复中说明结果。

## 禁止事项

- 禁止提交密钥、token、真实用户音视频、识别结果、数据库和日志。
- 禁止在未经确认的情况下删除、移动或覆盖数据产物。
- 禁止把多个大功能塞进一个 PR。
- 禁止只写“已测试”但不说明验证命令。
- 禁止为了通过检查而移除必要测试或降低质量门槛。
- 禁止在 `main` 上直接开发功能。
