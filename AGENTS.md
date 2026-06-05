# AGENTS.md

本文件适用于整个仓库，用于统一 Agent Dance 项目的协作、提交、PR、验证和安全规范。如果后续某个子目录新增更近的 `AGENTS.md`，以更近文件为准。

## 项目定位

Agent Dance 是一个 Doubao-first 的 AI 同声传译助手，面向外语演讲、技术分享、国际会议、网课和会议录制等单向音视频场景。

核心链路：

- 实时同传：前端采集麦克风音频并输出 16kHz/16bit/单声道 PCM，后端 Go 网关接入同声传译模型，向前端推送中文字幕事件。
- 上传回放：后端处理上传的音频或视频文件，生成带时间轴的中文字幕，视频模式支持字幕叠加播放。
- 自动纠错：后台根据后续上下文、术语、专名和数字复核生成 `segment.revision` 事件，前端展示修正前后内容和触发原因。

当前文档入口：

- `README.md`
- `PRODUCT.md`
- `docs/ai-simultaneous-interpretation-technical-solution.md`
- `docs/backend-go-implementation-plan.md`

## 技术边界

- 前端：Next.js、React、TypeScript、Tailwind CSS。
- 后端：Go，负责实时音频网关、Doubao AST WebSocket/protobuf、上传处理、字幕稳定和修正事件。
- 存储：SQLite 起步，后续可迁移 PostgreSQL。
- 运行依赖：ffmpeg、Doubao AST、Doubao 录音文件识别、Doubao 语音合成。

不要把 Doubao App Key、Access Key、Resource ID、临时 token、真实用户音视频、识别日志、用户字幕结果或本地数据库提交到仓库。

## 协作规则

- 基于独立分支开发，不直接在 `main` 上写功能。
- 开始开发前先同步远端：`git fetch origin --prune`，确认当前分支和工作区状态。
- 修改接口协议时，同步更新文档、类型定义或示例。
- 不覆盖、revert、stash 队友或用户已有改动；发现冲突先确认边界。
- 面向用户、PR 和提交说明默认使用中文；代码标识符、命令、协议字段和第三方 API 名称保留英文。
- 每次提交保持小粒度，一个 commit 表达一个清晰意图。

## 分支规范

分支名使用小写英文和短横线，例如：

```text
frontend/live-console
backend/live-gateway
backend/upload-processing
fix/audio-packetizer
docs/architecture
chore/project-setup
```

避免使用 `test`、`new`、`final`、`my-branch`、含空格或中文的分支名。

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
- `chore`：工程配置、脚手架、依赖或工具调整。
- `perf`：性能优化。

推荐 scope：`frontend`、`backend`、`live`、`doubao`、`audio`、`subtitle`、`repair`、`upload`、`docs`。

## PR 规范

PR 应保持明确主题，例如：

- 一个前端页面或工作台能力。
- 一个后端模块。
- 一组接口协议更新。
- 一个缺陷修复。
- 一组相关文档更新。

PR 描述建议包含：

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
```

合并要求：

- 至少一位协作者 review。
- 不包含密钥、真实用户数据、音视频大文件、数据库、日志或构建产物。
- 修改接口协议时必须同步更新文档或类型定义。
- 修改用户可见功能时提供截图、录屏或清晰复现步骤。

## 验证规范

通用检查：

```powershell
git status -sb
git diff --check
```

前端：

```powershell
npm run lint
npm run build
```

后端：

```powershell
cd backend
go test ./...
```

真实 Doubao 调用不要默认进入 CI。需要消耗额度或真实凭据的验证必须显式标记，并使用环境变量隔离。

## 忽略文件规范

仓库应忽略：

- 前端依赖和构建产物：`node_modules/`、`.next/`、`dist/`、`build/`。
- Go 构建产物：`bin/`、`*.exe`、`*.test`、`coverage.out`。
- 环境和密钥：`.env`、`.env.*`、`secrets/`、证书和私钥。
- 本地数据库：`*.db`、`*.sqlite`、`data/`、`runtime/`。
- 用户上传、音视频、日志和缓存：`uploads/`、`cache/`、`logs/`、`*.mp4`、`*.wav`。
- 本地 Agent 状态：`.agent/`、`.superpowers/`、`.codex/`、`.worktrees/`。
- 编辑器和系统文件：`.vscode/`、`.idea/`、`.DS_Store`、`Thumbs.db`。

允许提交：

- `README.md`
- `PRODUCT.md`
- `docs/**/*.md`
- `.env.example`
- 小型、无隐私、明确用于测试的文本 fixture。

如需演示音频或视频，不要直接提交大文件；应在 PR 描述中提供外部链接或复现步骤。

## Agent 工作规则

Agent 在本仓库工作时必须：

- 开始前读取本文件、README 和相关 docs。
- 确认任务范围、目标目录和已有未提交改动。
- 修改前说明将改哪些文件。
- 使用小步、局部、可验证的修改。
- 优先维护文档和接口协议，避免代码与方案漂移。
- 完成前运行相关验证命令，并在回复中说明结果。

禁止事项：

- 提交密钥、token、真实用户音视频、识别结果、数据库和日志。
- 未确认时删除、移动或覆盖数据产物。
- 把多个大功能塞进同一个 PR。
- 只写“已测试”但不说明验证命令。
- 为了通过检查而移除必要测试或降低质量门槛。
