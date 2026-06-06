# Agent Dance 后端接口文档

本文档描述 Agent Dance 后端当前已经实现的 HTTP 与 WebSocket 接口，供前端联调、后端开发和测试使用。

> 文档依据：`backend/internal/httpapi`、`backend/internal/live`、`backend/internal/audio`、`backend/internal/store`、`backend/internal/subtitle`、`backend/internal/doubao/ast` 当前实现
>
> 默认服务地址：`http://localhost:8080`
>
> 默认 WebSocket 地址：`ws://localhost:8080`

## 1. 当前接口范围

当前后端已实现：

| 类型 | 方法 | 路径 | 用途 |
| --- | --- | --- | --- |
| HTTP | `GET` | `/healthz` | 进程存活检查 |
| HTTP | `GET` | `/readyz` | 数据库存活与服务就绪检查 |
| HTTP | `POST` | `/api/sessions` | 创建同传会话 |
| HTTP | `GET` | `/api/sessions/{id}` | 查询同传会话 |
| WebSocket | `GET` Upgrade | `/api/live/ws?sessionId={id}` | 建立实时音频连接并上传 PCM 音频帧 |

当前尚未实现：

- 音频与视频文件上传接口。
- 上传处理进度与回放字幕查询接口。
- 主动结束会话的 HTTP 接口。
- 鉴权、用户体系和限流。
- `segment.revision` 的后台复核生成链路。

## 2. 服务配置

后端启动时需要以下环境变量：

| 环境变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `DOUBAO_API_KEY` | 条件必填 | 无 | Doubao API Key；优先使用 |
| `DOUBAO_APP_ID` | 条件必填 | 无 | 旧版 App 凭据之一 |
| `DOUBAO_APP_KEY` | 条件必填 | 无 | 旧版 App 凭据之一，也可单独作为鉴权值 |
| `DOUBAO_ACCESS_KEY` | 条件必填 | 无 | 旧版 App 凭据之一 |
| `DOUBAO_AST_RESOURCE_ID` | 是 | 无 | 同声传译资源 ID |
| `DOUBAO_AST_MODEL_ID` | 否 | 无 | 同声传译模型 ID，例如 Seed LiveInterpret 2.0 对应模型 |
| `DOUBAO_AUC_RESOURCE_ID` | 是 | 无 | 录音文件识别资源 ID |
| `DATABASE_URL` | 是 | 无 | SQLite 数据库路径，例如 `runtime/agent-dance.db` |
| `UPLOAD_DIR` | 是 | 无 | 上传文件目录；当前接口尚未使用 |
| `HTTP_ADDR` | 否 | `:8080` | HTTP 服务监听地址 |
| `HTTP_ALLOWED_ORIGINS` | 否 | 空 | 允许跨源调用 HTTP 和 WebSocket 的前端 Origin，多个值用英文逗号分隔 |

配置示例见 `backend/.env.example`。

## 3. 通用约定

### 3.1 HTTP 数据格式

- JSON 接口请求体使用 `application/json`。
- JSON 响应使用 `Content-Type: application/json`。
- 时间字段使用 UTC 时区的 RFC 3339 Nano 格式。
- 当前接口没有鉴权请求头。

### 3.2 JSON 错误格式

会话相关 HTTP 接口发生错误时返回：

```json
{
  "error": "session not found"
}
```

WebSocket 建连前的错误使用 `text/plain` 返回，例如：

```text
missing sessionId
```

### 3.3 会话状态

| 状态 | 含义 | 进入条件 |
| --- | --- | --- |
| `created` | 会话已创建，可以建立实时 WebSocket | `POST /api/sessions` 成功 |
| `running` | 实时 WebSocket 已连接 | `/api/live/ws` 成功升级连接 |
| `closed` | 实时连接已结束，不可再次连接 | 客户端断开、服务端关闭或 WebSocket 错误 |

状态转换：

```text
created -> running -> closed
```

每个会话只能成功建立一次实时 WebSocket。处于 `running` 或 `closed` 状态的会话再次连接时返回 `409 Conflict`。

## 4. HTTP 接口

### 4.1 进程存活检查

检查后端 HTTP 进程是否存活。该接口不检查数据库或外部服务。

```http
GET /healthz
```

成功响应：

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "status": "ok"
}
```

错误响应：

| 状态码 | 条件 |
| --- | --- |
| `405 Method Not Allowed` | 使用了非 `GET` 方法 |

调用示例：

```bash
curl http://localhost:8080/healthz
```

### 4.2 服务就绪检查

检查后端服务及 SQLite 数据库连接是否可用。

```http
GET /readyz
```

成功响应：

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "status": "ready"
}
```

数据库不可用：

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
```

```json
{
  "status": "not_ready",
  "error": "数据库错误信息"
}
```

错误响应：

| 状态码 | 条件 |
| --- | --- |
| `405 Method Not Allowed` | 使用了非 `GET` 方法 |
| `503 Service Unavailable` | 数据库连接不可用 |

调用示例：

```bash
curl http://localhost:8080/readyz
```

### 4.3 创建同传会话

创建一个会话记录。成功创建后，会话状态为 `created`，可以使用返回的 `id` 建立实时 WebSocket。

```http
POST /api/sessions
Content-Type: application/json
```

请求体：

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `mode` | `string` | 否 | `live` | 会话模式；当前未进行枚举校验 |
| `source_language` | `string` | 否 | `auto` | 源语言代码；当前未进行枚举校验 |
| `target_language` | `string` | 否 | `zh` | 目标语言代码；当前未进行枚举校验 |
| `voice_enabled` | `boolean` | 否 | `false` | 是否启用目标语音；当前实时网关尚未输出语音 |

请求示例：

```json
{
  "mode": "live",
  "source_language": "en",
  "target_language": "zh",
  "voice_enabled": false
}
```

成功响应：

```http
HTTP/1.1 201 Created
Content-Type: application/json
Location: /api/sessions/8f7aa62af869e2f1af3759f12a8c3b90
```

```json
{
  "id": "8f7aa62af869e2f1af3759f12a8c3b90",
  "mode": "live",
  "source_language": "en",
  "target_language": "zh",
  "voice_enabled": false,
  "status": "created",
  "created_at": "2026-06-06T03:00:00.1234567Z",
  "updated_at": "2026-06-06T03:00:00.1234567Z"
}
```

注意事项：

- 请求 JSON 不允许出现未定义字段，否则返回 `400`。
- 空字符串和仅包含空格的字段会使用默认值。
- 当前实现只解码第一个 JSON 值，没有额外校验请求体尾部数据。
- 会话 ID 是 16 字节随机数的十六进制表示，共 32 个字符。

错误响应：

| 状态码 | 响应 | 条件 |
| --- | --- | --- |
| `400 Bad Request` | `{"error":"invalid JSON request body"}` | JSON 无效、类型错误或包含未知字段 |
| `405 Method Not Allowed` | `{"error":"method not allowed"}` | 使用了非 `POST` 方法 |
| `500 Internal Server Error` | `{"error":"create session failed"}` | 数据库写入失败 |

调用示例：

```bash
curl -i -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "live",
    "source_language": "en",
    "target_language": "zh",
    "voice_enabled": false
  }'
```

### 4.4 查询同传会话

根据会话 ID 查询当前配置和状态。

```http
GET /api/sessions/{id}
```

路径参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `id` | `string` | 创建会话时返回的 32 字符 ID |

成功响应：

```http
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "id": "8f7aa62af869e2f1af3759f12a8c3b90",
  "mode": "live",
  "source_language": "en",
  "target_language": "zh",
  "voice_enabled": false,
  "status": "running",
  "created_at": "2026-06-06T03:00:00.1234567Z",
  "updated_at": "2026-06-06T03:00:02.456789Z"
}
```

当状态为 `closed` 时，响应包含 `closed_at`：

```json
{
  "id": "8f7aa62af869e2f1af3759f12a8c3b90",
  "mode": "live",
  "source_language": "en",
  "target_language": "zh",
  "voice_enabled": false,
  "status": "closed",
  "created_at": "2026-06-06T03:00:00.1234567Z",
  "updated_at": "2026-06-06T03:05:00.1234567Z",
  "closed_at": "2026-06-06T03:05:00.1234567Z"
}
```

错误响应：

| 状态码 | 响应 | 条件 |
| --- | --- | --- |
| `404 Not Found` | `{"error":"session not found"}` | 会话不存在 |
| `404 Not Found` | 标准文本 404 响应 | ID 为空或路径中包含额外 `/` |
| `405 Method Not Allowed` | `{"error":"method not allowed"}` | 使用了非 `GET` 方法 |
| `500 Internal Server Error` | `{"error":"get session failed"}` | 数据库读取失败 |

调用示例：

```bash
curl http://localhost:8080/api/sessions/8f7aa62af869e2f1af3759f12a8c3b90
```

## 5. 实时音频 WebSocket

### 5.1 建立连接

建立实时音频上传连接。连接成功前，必须先通过 `POST /api/sessions` 创建会话。

```http
GET /api/live/ws?sessionId={id}
Connection: Upgrade
Upgrade: websocket
```

示例地址：

```text
ws://localhost:8080/api/live/ws?sessionId=8f7aa62af869e2f1af3759f12a8c3b90
```

建连规则：

- `sessionId` 必须存在。
- 会话必须处于 `created` 状态。
- 建连时后端会原子地把会话状态从 `created` 更新为 `running`。
- 同一个会话不允许重复连接。
- WebSocket 断开后，会话状态更新为 `closed`。
- 当前服务没有恢复或重连同一会话的能力；重连需要创建新会话。

建连失败：

| HTTP 状态码 | 文本响应 | 条件 |
| --- | --- | --- |
| `400 Bad Request` | `missing sessionId` | 缺少 `sessionId` |
| `404 Not Found` | `session not found` | 会话不存在 |
| `405 Method Not Allowed` | `method not allowed` | 使用了非 `GET` 方法 |
| `409 Conflict` | `session is not connectable` | 会话正在运行、已经关闭或被其他连接抢先启动 |
| `500 Internal Server Error` | `get session failed` | 查询会话失败 |
| `500 Internal Server Error` | `start session failed` | 更新会话状态失败 |

### 5.2 服务端事件格式

WebSocket 服务端事件均为 UTF-8 JSON 文本消息。

实时网关控制事件结构：

```ts
type LiveEvent = {
  type: string;
  session_id?: string;
  sequence?: number;
  code?: string;
  message?: string;
};
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `type` | `string` | 事件类型 |
| `session_id` | `string` | 会话 ID，仅部分事件返回 |
| `sequence` | `number` | 已接收音频帧序号，仅部分事件返回 |
| `code` | `string` | 错误代码，仅错误事件返回 |
| `message` | `string` | 错误详情，仅错误事件返回 |

Doubao AST 字幕事件会被归一化为前端字段命名：

```ts
type InterpretationEvent =
  | { type: "segment.partial"; segmentId: string; text?: string; sourceText?: string; startMs?: number; endMs?: number }
  | { type: "segment.final"; segmentId: string; text?: string; sourceText?: string; startMs?: number; endMs?: number }
  | { type: "audio.delta"; segmentId?: string; audio: string; codec?: "pcm" | "ogg_opus" }
  | { type: "session.error"; code?: string; message?: string; providerLogId?: string };
```

### 5.3 `session.ready`

WebSocket 建连成功后的第一个服务端事件。

```json
{
  "type": "session.ready",
  "session_id": "8f7aa62af869e2f1af3759f12a8c3b90"
}
```

客户端应在收到该事件后开始发送音频帧。

### 5.4 浏览器音频帧格式

客户端必须发送 WebSocket Binary Message。每个消息由 12 字节头部和 PCM 数据组成。

```text
偏移       长度       类型             字节序        内容
0          4          uint32           little-endian 音频帧 sequence
4          8          uint64           little-endian 音频帧 timestamp_ms
12         N          PCM int16 bytes  little-endian 单声道 PCM 数据
```

音频要求：

| 项目 | 要求 |
| --- | --- |
| 采样率 | `16000 Hz` |
| 位深 | `16 bit` |
| 声道 | 单声道 |
| PCM 字节序 | little-endian |
| 单帧 PCM 最小长度 | `2` 字节 |
| 单帧 PCM 最大长度 | `32000` 字节，即最多 1 秒音频 |
| 完整 WebSocket 消息最大长度 | `32012` 字节 |
| sequence 规则 | 在同一连接中必须严格递增 |
| timestamp_ms 规则 | `uint64` 毫秒时间戳；当前后端只解析和缓存，不校验单调性 |

推荐客户端每 80ms 发送一帧：

```text
16000 samples/s * 0.08s * 2 bytes/sample = 2560 bytes PCM
完整消息大小 = 12 + 2560 = 2572 bytes
```

浏览器组帧示例：

```ts
function buildAudioFrame(
  sequence: number,
  timestampMs: bigint,
  pcm: Int16Array
): ArrayBuffer {
  const headerSize = 12;
  const buffer = new ArrayBuffer(headerSize + pcm.byteLength);
  const view = new DataView(buffer);

  view.setUint32(0, sequence, true);
  view.setBigUint64(4, timestampMs, true);
  new Uint8Array(buffer, headerSize).set(
    new Uint8Array(pcm.buffer, pcm.byteOffset, pcm.byteLength)
  );

  return buffer;
}
```

发送示例：

```ts
const socket = new WebSocket(
  `ws://localhost:8080/api/live/ws?sessionId=${sessionId}`
);

socket.addEventListener("message", (event) => {
  const message = JSON.parse(event.data);

  if (message.type === "session.ready") {
    const pcm = new Int16Array(1280);
    socket.send(buildAudioFrame(1, 0n, pcm));
  }
});
```

### 5.5 `audio.frame.accepted`

服务端成功解析并缓存一帧音频后返回：

```json
{
  "type": "audio.frame.accepted",
  "sequence": 1
}
```

该事件只表示：

- 二进制帧格式有效。
- sequence 顺序有效。
- 音频帧已经加入内存缓存。

该事件不表示：

- 音频已经发送给 Doubao。
- 识别或翻译已经完成。
- 音频已经持久化。

### 5.6 `session.error`

连接建立后发生协议错误时，服务端先返回 `session.error` 文本事件，然后使用 WebSocket 状态码 `1008 Policy Violation` 关闭连接。

示例：

```json
{
  "type": "session.error",
  "code": "invalid_audio_frame",
  "message": "browser audio frame is shorter than header"
}
```

错误代码：

| code | message 示例 | 触发条件 |
| --- | --- | --- |
| `invalid_audio_frame` | `expected binary audio frame` | 客户端发送了文本消息 |
| `invalid_audio_frame` | `browser audio frame is shorter than header` | 消息长度小于 12 字节 |
| `invalid_audio_frame` | `browser audio frame has no pcm payload` | 只有头部，没有 PCM 数据 |
| `invalid_audio_frame` | `browser audio frame pcm payload is too large` | PCM 数据超过 32000 字节 |
| `invalid_audio_frame` | `browser audio frame pcm payload is not int16 aligned` | PCM 数据字节数不是 2 的倍数 |
| `out_of_order_frame` | `audio frame sequence must increase` | sequence 小于或等于上一帧 |

### 5.7 Ping、超时和关闭

- 服务端每 30 秒发送一次 WebSocket Ping。
- Pong 等待超时为 5 秒。
- Ping 失败时，服务端以 `1008 Policy Violation` 关闭连接，关闭原因是 `ping_timeout`。
- 普通连接结束时，服务端以 `1000 Normal Closure` 和原因 `session closed` 关闭连接。
- 连接结束后，后端将会话状态更新为 `closed` 并写入 `closed_at`。

## 6. 推荐联调流程

### 6.1 实时同传连接流程

```text
前端                                      后端
  |                                        |
  | POST /api/sessions                     |
  |--------------------------------------->|
  | 201 Created + session.id               |
  |<---------------------------------------|
  |                                        |
  | WebSocket /api/live/ws?sessionId=...   |
  |--------------------------------------->|
  | session.ready                          |
  |<---------------------------------------|
  |                                        |
  | binary audio frame, sequence=1         |
  |--------------------------------------->|
  | audio.frame.accepted, sequence=1       |
  |<---------------------------------------|
  |                                        |
  | binary audio frame, sequence=2         |
  |--------------------------------------->|
  | audio.frame.accepted, sequence=2       |
  |<---------------------------------------|
  |                                        |
  | Close WebSocket                        |
  |--------------------------------------->|
  | session status becomes closed          |
```

### 6.2 前端处理建议

1. 创建会话并保存 `id`。
2. 使用 `id` 建立 WebSocket。
3. 等待 `session.ready` 后再开始发送麦克风数据。
4. 将麦克风音频重采样为 16kHz、16bit、单声道 PCM。
5. 为每个音频帧生成严格递增的 sequence。
6. 接收 `audio.frame.accepted`，用于监控上传链路是否正常。
7. 收到 `session.error` 后停止采集并展示错误。
8. WebSocket 断开后不要使用原会话重连，应创建新会话。

## 7. 已知限制与联调注意事项

- 实时网关已接入 Doubao AST，并会把识别/翻译结果转为 `segment.partial`、`segment.final`、`audio.delta` 或 `session.error` 事件。
- 真实 Doubao 链路依赖有效的 `DOUBAO_API_KEY` 或旧版 App 凭据、`DOUBAO_AST_RESOURCE_ID` 和可用模型配置。
- 当前内存缓存每个会话保留最近 256 帧；服务重启后缓存丢失。
- 音频帧会被缓存，但当前连接关闭时没有主动删除对应缓存。
- 当前创建会话接口没有校验 `mode` 和语言代码是否合法。
- 前端和后端不同源运行时，需要在 `HTTP_ALLOWED_ORIGINS` 中显式配置前端 Origin，例如 `http://localhost:3000`。
- 当前没有认证和授权机制，接口不应直接暴露到公网。
- 当前没有主动关闭会话接口；关闭 WebSocket 即结束会话。
- 当前 `voice_enabled` 仅保存到会话记录，不会产生语音输出。
- 当前上传目录配置已存在，但上传接口尚未实现。

## 8. 字幕事件约定

当前 WebSocket 已返回以下字幕事件。新增事件或字段时，应同步更新本文档和前端类型定义：

```ts
type InterpretationEvent =
  | {
      type: "segment.partial";
      segmentId: string;
      text?: string;
      sourceText?: string;
      startMs?: number;
      endMs?: number;
    }
  | {
      type: "segment.final";
      segmentId: string;
      text?: string;
      sourceText?: string;
      startMs?: number;
      endMs?: number;
    }
  | {
      type: "segment.revision";
      segmentId: string;
      before?: string;
      after?: string;
      reason?: string;
    }
  | {
      type: "session.error";
      code?: string;
      message?: string;
      providerLogId?: string;
    };
```

规划事件在正式实现前不能作为前后端联调契约使用。

## 9. 快速检查清单

后端队友修改接口时，应检查：

- 路由、方法、状态码和响应结构是否同步更新本文档。
- WebSocket 二进制帧头部是否保持向后兼容。
- 新增事件是否包含稳定的 `type` 和必要关联 ID。
- 错误是否使用稳定错误码，不依赖易变化的 message 文本。
- 会话状态是否仍满足明确且可测试的转换规则。
- 是否补充对应 HTTP、WebSocket 或存储测试。
