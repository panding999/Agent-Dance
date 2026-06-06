"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import {
  ArrowRight,
  Broadcast,
  CheckCircle,
  CloudArrowUp,
  FileVideo,
  FileAudio,
  Microphone,
  Play,
  Stop
} from "@phosphor-icons/react";
import {
  LiveAudioStreamer,
  connectLiveWebSocket,
  createLiveSession,
  parseLiveServerEvent,
  type LiveServerEvent
} from "@/lib/live-client";

type WorkbenchMode = "live" | "upload";
type UploadKind = "audio" | "video";
type SessionState = "idle" | "listening" | "draft" | "correcting" | "ready" | "error";
type SegmentState = "draft" | "confirmed" | "corrected";

type TranscriptSegment = {
  id: string;
  time: string;
  source: string;
  zh: string;
  state: SegmentState;
  correction?: {
    first: string;
    final: string;
    trigger: string;
  };
};

const segments: TranscriptSegment[] = [
  {
    id: "intro",
    time: "00:04",
    source: "Today we will look at how retrieval augmented generation changes support workflows.",
    zh: "今天我们来看检索增强生成如何改变支持流程。",
    state: "confirmed"
  },
  {
    id: "term",
    time: "00:10",
    source: "The team moved the Kubernetes cluster closer to the data plane.",
    zh: "团队把 Kubernetes 集群移到更靠近数据平面的位置。",
    state: "corrected",
    correction: {
      first: "团队把古伯内特斯集群移到更靠近数据平面的位置。",
      final: "团队把 Kubernetes 集群移到更靠近数据平面的位置。",
      trigger: "后续上下文确认这是技术专名，并命中术语表。"
    }
  },
  {
    id: "latency",
    time: "00:16",
    source: "Latency matters because the audience has to read while listening.",
    zh: "延迟很关键，因为观众需要一边听一边读。",
    state: "draft"
  }
];

function stateLabel(state: SegmentState) {
  if (state === "draft") return "实时生成";
  if (state === "corrected") return "已自动纠错";
  return "已确认";
}

function stateClass(state: SegmentState) {
  if (state === "draft") return "border-amber-200 bg-amber-50 text-amber-800";
  if (state === "corrected") return "border-blue-200 bg-blue-50 text-blue-800";
  return "border-emerald-200 bg-emerald-50 text-emerald-800";
}

function upsertSegment(current: TranscriptSegment[], event: LiveServerEvent) {
  const id = event.segmentId ?? `segment-${current.length + 1}`;
  const existingIndex = current.findIndex((segment) => segment.id === id);
  const existing = existingIndex >= 0 ? current[existingIndex] : undefined;
  const state: SegmentState =
    event.type === "segment.revision" ? "corrected" : event.type === "segment.final" ? "confirmed" : "draft";
  const nextText = event.after ?? event.text ?? existing?.zh ?? "正在翻译...";
  const hasTime = event.startMs !== undefined || event.endMs !== undefined;
  const next: TranscriptSegment = {
    id,
    time: hasTime ? formatTimestamp(event.startMs ?? event.endMs) : existing?.time ?? "00:00",
    source: event.sourceText ?? existing?.source ?? "等待源文字幕...",
    zh: nextText,
    state,
    correction:
      event.type === "segment.revision"
        ? {
            first: event.before ?? existing?.zh ?? "",
            final: nextText,
            trigger: event.reason ?? "后台复核更新"
          }
        : existing?.correction
  };

  if (existingIndex < 0) {
    return [...current, next];
  }

  const updated = current.slice();
  updated[existingIndex] = next;
  return updated;
}

function formatTimestamp(milliseconds?: number) {
  if (milliseconds === undefined || milliseconds < 0) {
    return "00:00";
  }

  const totalSeconds = Math.floor(milliseconds / 1000);
  const minutes = Math.floor(totalSeconds / 60).toString().padStart(2, "0");
  const seconds = (totalSeconds % 60).toString().padStart(2, "0");
  return `${minutes}:${seconds}`;
}

function errorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return "实时同传启动失败";
}

export function LandingPage() {
  const [mode, setMode] = useState<WorkbenchMode>("live");
  const [uploadKind, setUploadKind] = useState<UploadKind>("video");
  const [sessionState, setSessionState] = useState<SessionState>("idle");
  const [visibleCount, setVisibleCount] = useState(2);
  const [liveSegments, setLiveSegments] = useState<TranscriptSegment[]>([]);
  const [liveSessionId, setLiveSessionId] = useState<string | null>(null);
  const [acceptedFrames, setAcceptedFrames] = useState(0);
  const [liveError, setLiveError] = useState<string | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const streamerRef = useRef<LiveAudioStreamer | null>(null);

  const activeSegments = useMemo(
    () => (mode === "live" ? liveSegments : segments.slice(0, visibleCount)),
    [liveSegments, mode, visibleCount]
  );
  const uploadActive = mode === "upload" && sessionState !== "idle" && sessionState !== "ready";

  const cleanupLiveConnection = useCallback(() => {
    streamerRef.current?.stop().catch(() => undefined);
    streamerRef.current = null;

    if (socketRef.current && socketRef.current.readyState <= WebSocket.OPEN) {
      socketRef.current.close(1000, "client stopped");
    }
    socketRef.current = null;
  }, []);

  useEffect(() => {
    if (!uploadActive) return;

    const timeline: SessionState[] = ["listening", "draft", "correcting", "ready"];
    let step = 0;
    const timer = window.setInterval(() => {
      step += 1;
      setVisibleCount((count) => Math.min(segments.length, count + 1));
      setSessionState(timeline[Math.min(step, timeline.length - 1)]);
      if (step >= timeline.length - 1) {
        window.clearInterval(timer);
      }
    }, 1200);

    return () => window.clearInterval(timer);
  }, [uploadActive]);

  useEffect(() => {
    return () => cleanupLiveConnection();
  }, [cleanupLiveConnection]);

  const handleLiveEvent = useCallback(
    async (event: LiveServerEvent, socket: WebSocket) => {
      if (event.type === "session.ready") {
        setSessionState("listening");
        const streamer = new LiveAudioStreamer((frame) => {
          if (socket.readyState === WebSocket.OPEN) {
            socket.send(frame);
          }
        });
        streamerRef.current = streamer;
        await streamer.start();
        return;
      }

      if (event.type === "audio.frame.accepted") {
        setAcceptedFrames(event.sequence ?? 0);
        return;
      }

      if (event.type === "segment.partial" || event.type === "segment.final") {
        setSessionState("draft");
        setLiveSegments((current) => upsertSegment(current, event));
        return;
      }

      if (event.type === "segment.revision") {
        setSessionState("correcting");
        setLiveSegments((current) => upsertSegment(current, event));
        return;
      }

      if (event.type === "session.error") {
        setLiveError(event.message ?? event.code ?? "实时同传连接出错");
        setSessionState("error");
        cleanupLiveConnection();
      }
    },
    [cleanupLiveConnection]
  );

  const startLiveSession = useCallback(async () => {
    cleanupLiveConnection();
    setMode("live");
    setSessionState("listening");
    setLiveSegments([]);
    setLiveSessionId(null);
    setAcceptedFrames(0);
    setLiveError(null);

    try {
      const session = await createLiveSession({
        mode: "live",
        source_language: "en",
        target_language: "zh",
        voice_enabled: false
      });
      setLiveSessionId(session.id);

      const socket = connectLiveWebSocket(session.id);
      socketRef.current = socket;

      socket.addEventListener("message", (message) => {
        try {
          const event = parseLiveServerEvent(message.data);
          if (event) {
            void handleLiveEvent(event, socket).catch((error: unknown) => {
              setLiveError(errorMessage(error));
              setSessionState("error");
              cleanupLiveConnection();
            });
          }
        } catch (error) {
          setLiveError(errorMessage(error));
          setSessionState("error");
          cleanupLiveConnection();
        }
      });

      socket.addEventListener("error", () => {
        setLiveError("实时同传 WebSocket 连接失败");
        setSessionState("error");
        cleanupLiveConnection();
      });

      socket.addEventListener("close", () => {
        streamerRef.current?.stop().catch(() => undefined);
        streamerRef.current = null;
        setSessionState((state) => (state === "error" || state === "idle" ? state : "ready"));
      });
    } catch (error) {
      setLiveError(errorMessage(error));
      setSessionState("error");
      cleanupLiveConnection();
    }
  }, [cleanupLiveConnection, handleLiveEvent]);

  const stopLiveSession = useCallback(() => {
    cleanupLiveConnection();
    setSessionState("ready");
  }, [cleanupLiveConnection]);

  const startSession = useCallback(
    (nextMode = mode) => {
      if (nextMode === "live") {
        void startLiveSession();
        return;
      }

      cleanupLiveConnection();
      setMode(nextMode);
      setVisibleCount(1);
      setSessionState("listening");
      setLiveError(null);
    },
    [cleanupLiveConnection, mode, startLiveSession]
  );

  return (
    <main className="h-full overflow-hidden">
      <section className="flex h-full w-full items-stretch px-4 py-3 sm:px-6 lg:px-8">
        <InterpreterWorkspace
          mode={mode}
          uploadKind={uploadKind}
          sessionState={sessionState}
          segments={activeSegments}
          liveSessionId={liveSessionId}
          acceptedFrames={acceptedFrames}
          liveError={liveError}
          onModeChange={(nextMode) => setMode(nextMode)}
          onUploadKindChange={setUploadKind}
          onStart={startSession}
          onStop={stopLiveSession}
        />
      </section>
    </main>
  );
}

function InterpreterWorkspace({
  mode,
  uploadKind,
  sessionState,
  segments,
  liveSessionId,
  acceptedFrames,
  liveError,
  onModeChange,
  onUploadKindChange,
  onStart,
  onStop
}: {
  mode: WorkbenchMode;
  uploadKind: UploadKind;
  sessionState: SessionState;
  segments: TranscriptSegment[];
  liveSessionId: string | null;
  acceptedFrames: number;
  liveError: string | null;
  onModeChange: (mode: WorkbenchMode) => void;
  onUploadKindChange: (kind: UploadKind) => void;
  onStart: (mode: WorkbenchMode) => void;
  onStop: () => void;
}) {
  const correctedSegment = segments.find((segment) => segment.correction);
  const showingVideo = mode === "upload" && uploadKind === "video";
  const liveRunning =
    mode === "live" && (sessionState === "listening" || sessionState === "draft" || sessionState === "correcting");

  return (
    <section
      id="workspace"
      className="soft-shadow shell-border glass-panel h-full min-h-0 w-full min-w-0 overflow-hidden rounded-[30px] p-3"
    >
      <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-[24px] border border-white/70 bg-white/62 p-4 text-slate-950 backdrop-blur-xl sm:p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-white/80 bg-white/58 px-4 py-2.5 shadow-sm">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-blue-600 text-white shadow-sm">
              <Broadcast size={19} weight="bold" />
            </span>
            <div className="min-w-0">
              <div className="font-semibold text-slate-950">Agent Dance</div>
              <div className="text-xs text-slate-500">英文音视频 → 中文字幕 · 自动纠错</div>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <SessionStatus state={sessionState} />
            <button
              onClick={() => (liveRunning ? onStop() : onStart(mode))}
              className="inline-flex items-center justify-center gap-2 rounded-xl bg-blue-600 px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-blue-700 active:translate-y-px"
            >
              {liveRunning ? <Stop size={16} weight="fill" /> : <Play size={16} weight="fill" />}
              {liveRunning ? "停止同传" : mode === "live" ? "开始同传" : "查看回放"}
            </button>
          </div>
        </div>

        <div className="grid gap-3 lg:grid-cols-[1fr_1fr]">
          <ModeButton
            active={mode === "live"}
            icon={<Microphone size={18} />}
            title="实时同传"
            detail="麦克风输入英文音频，实时输出中文字幕"
            onClick={() => {
              onModeChange("live");
              onStart("live");
            }}
          />
          <ModeButton
            active={mode === "upload"}
            icon={<CloudArrowUp size={18} />}
            title="上传回放"
            detail="会议录制、网课文件生成回放字幕"
            onClick={() => {
              onModeChange("upload");
              onUploadKindChange("video");
              onStart("upload");
            }}
          />
        </div>

        <div
          className={`mt-3 grid min-h-0 flex-1 min-w-0 gap-4 ${
            showingVideo
              ? "xl:grid-cols-[18rem_minmax(42rem,1fr)_19rem]"
              : "xl:grid-cols-[18rem_minmax(34rem,1fr)_22rem]"
          }`}
        >
          <ControlColumn
            mode={mode}
            uploadKind={uploadKind}
            sessionState={sessionState}
            liveSessionId={liveSessionId}
            acceptedFrames={acceptedFrames}
            liveError={liveError}
            onUploadKindChange={onUploadKindChange}
            onStart={onStart}
          />
          {showingVideo ? (
            <VideoPreview segments={segments} />
          ) : (
            <TranscriptStream segments={segments} />
          )}
          <CorrectionExplainer segment={correctedSegment} compact={showingVideo} />
        </div>
      </div>
    </section>
  );
}

function ModeButton({
  active,
  icon,
  title,
  detail,
  onClick
}: {
  active: boolean;
  icon: React.ReactNode;
  title: string;
  detail: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-2xl border p-3 text-left transition active:translate-y-px ${
        active
          ? "border-blue-300 bg-blue-50/90 text-blue-950 shadow-sm"
          : "border-slate-200 bg-white/58 text-slate-700 hover:bg-white/80"
      }`}
    >
      <div className="flex items-center gap-3">
        <span className="grid size-8 place-items-center rounded-xl bg-slate-900 text-white">{icon}</span>
        <div>
          <div className="font-semibold">{title}</div>
          <div className="mt-1 text-xs text-slate-500">{detail}</div>
        </div>
      </div>
    </button>
  );
}

function SessionStatus({ state }: { state: SessionState }) {
  const labels: Record<SessionState, string> = {
    idle: "待开始",
    listening: "正在听",
    draft: "生成字幕",
    correcting: "正在纠错",
    ready: "已同步",
    error: "连接异常"
  };

  return (
    <span
      className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium ${
        state === "error" ? "border-rose-200 bg-rose-50 text-rose-800" : "border-blue-200 bg-blue-50 text-blue-800"
      }`}
    >
      <span className={`size-2 rounded-full ${state === "error" ? "bg-rose-500" : "bg-blue-500"}`} />
      {labels[state]}
    </span>
  );
}

function ControlColumn({
  mode,
  sessionState,
  uploadKind,
  liveSessionId,
  acceptedFrames,
  liveError,
  onUploadKindChange,
  onStart
}: {
  mode: WorkbenchMode;
  sessionState: SessionState;
  uploadKind: UploadKind;
  liveSessionId: string | null;
  acceptedFrames: number;
  liveError: string | null;
  onUploadKindChange: (kind: UploadKind) => void;
  onStart: (mode: WorkbenchMode) => void;
}) {
  const isLive = mode === "live";
  const isVideo = uploadKind === "video";

  return (
    <div className="flex min-h-0 min-w-0 flex-col rounded-2xl border border-white/70 bg-white/66 p-4 shadow-sm backdrop-blur">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-semibold text-slate-950">{isLive ? "音频输入" : "上传文件"}</span>
        {isLive ? (
          <Microphone size={19} className="text-blue-600" />
        ) : isVideo ? (
          <FileVideo size={19} className="text-blue-600" />
        ) : (
          <FileAudio size={19} className="text-blue-600" />
        )}
      </div>
      {isLive ? (
        <div className="mt-4">
          <WaveBars active={sessionState === "listening" || sessionState === "draft" || sessionState === "correcting"} />
          <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-slate-500">
            <span>16kHz PCM</span>
            <span className="text-right">80ms/frame</span>
          </div>
          <div className="mt-3 rounded-xl border border-slate-200 bg-white/70 p-3 text-xs text-slate-600">
            <div className="flex items-center justify-between gap-3">
              <span>会话</span>
              <span className="mono max-w-[10rem] truncate text-slate-900">{liveSessionId ?? "未创建"}</span>
            </div>
            <div className="mt-2 flex items-center justify-between gap-3">
              <span>后端接收</span>
              <span className="mono text-slate-900">{acceptedFrames} 帧</span>
            </div>
            {liveError ? <div className="mt-2 rounded-lg bg-rose-50 p-2 text-rose-700">{liveError}</div> : null}
          </div>
        </div>
      ) : (
        <UploadFilePanel
          uploadKind={uploadKind}
          onUploadKindChange={(kind) => {
            onUploadKindChange(kind);
            onStart("upload");
          }}
        />
      )}
      <div className="mt-auto rounded-xl border border-slate-200 bg-white/60 p-3">
        <div className="text-xs font-semibold text-slate-700">同传设置</div>
        <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
          <SettingPill label="输入" value={isLive ? "麦克风" : isVideo ? "视频文件" : "音频文件"} />
          <SettingPill label="输出" value="中文字幕" />
          <SettingPill label="模式" value={isLive ? "实时" : "回放"} />
          <SettingPill label="纠错" value="开启" />
        </div>
      </div>
    </div>
  );
}

function UploadFilePanel({
  uploadKind,
  onUploadKindChange
}: {
  uploadKind: UploadKind;
  onUploadKindChange: (kind: UploadKind) => void;
}) {
  const isVideo = uploadKind === "video";

  return (
    <div className="mt-4 space-y-3">
      <div className="grid grid-cols-2 rounded-xl border border-slate-200 bg-white/70 p-1 text-sm">
        {(["audio", "video"] as UploadKind[]).map((kind) => (
          <button
            key={kind}
            onClick={() => onUploadKindChange(kind)}
            className={`rounded-lg px-3 py-2 font-medium transition active:translate-y-px ${
              uploadKind === kind
                ? "bg-slate-950 text-white shadow-sm"
                : "text-slate-600 hover:bg-slate-100"
            }`}
          >
            {kind === "audio" ? "音频" : "视频"}
          </button>
        ))}
      </div>
      <div className="rounded-2xl border border-dashed border-blue-200 bg-blue-50/70 p-4">
        {isVideo ? (
          <FileVideo size={30} className="text-blue-600" />
        ) : (
          <FileAudio size={30} className="text-blue-600" />
        )}
        <div className="mt-4 font-semibold text-slate-950">
          {isVideo ? "conference-keynote.mp4" : "conference-audio.mp3"}
        </div>
        <div className="mt-1 text-sm text-slate-500">
          {isVideo ? "生成视频字幕和时间轴" : "生成滚动字幕和时间轴"}
        </div>
        <div className="mt-4 h-2 overflow-hidden rounded-full bg-blue-100">
          <div className="h-full w-[72%] rounded-full bg-blue-500" />
        </div>
      </div>
    </div>
  );
}

function TranscriptStream({ segments }: { segments: TranscriptSegment[] }) {
  return (
    <div className="flex min-h-0 min-w-0 flex-col space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-slate-950">中文字幕流</div>
          <div className="mt-1 text-xs text-slate-500">英文原声输入后，中文译文逐句滚动追加</div>
        </div>
        <span className="hidden rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-xs font-medium text-blue-800 sm:inline-flex">
          目标语言：中文
        </span>
      </div>
      <div className="stable-scrollbar min-h-0 flex-1 overflow-y-auto pr-1">
        <div className="space-y-3">
          {segments.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-200 bg-white/60 p-6 text-sm text-slate-500">
              等待后端字幕事件...
            </div>
          ) : (
            <AnimatePresence initial={false}>
              {segments.map((segment) => (
                <TranscriptCard key={segment.id} segment={segment} />
              ))}
            </AnimatePresence>
          )}
        </div>
      </div>
    </div>
  );
}

function VideoPreview({ segments }: { segments: TranscriptSegment[] }) {
  const currentSubtitle = segments.find((segment) => segment.state === "corrected")?.zh ?? segments[0]?.zh;

  return (
    <div className="flex min-h-0 min-w-0 flex-col space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-slate-950">视频回放</div>
          <div className="mt-1 text-xs text-slate-500">字幕直接叠加在视频画面底部</div>
        </div>
        <span className="hidden rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-xs font-medium text-blue-800 sm:inline-flex">
          目标语言：中文
        </span>
      </div>
      <div className="relative min-h-[24rem] flex-1 overflow-hidden rounded-2xl border border-slate-200 bg-slate-950 shadow-sm">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_25%_20%,rgba(96,165,250,0.35),transparent_24rem),linear-gradient(135deg,#0f172a,#111827_55%,#1e293b)]" />
        <div className="absolute left-5 top-5 rounded-full border border-white/15 bg-white/10 px-3 py-1 text-xs font-medium text-white/80 backdrop-blur">
          00:10 / 24:18
        </div>
        <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/78 via-black/34 to-transparent px-6 pb-6 pt-16">
          <p className="mx-auto max-w-3xl rounded-xl bg-black/42 px-4 py-3 text-center text-xl font-semibold leading-9 text-white shadow-lg backdrop-blur">
            {currentSubtitle}
          </p>
        </div>
        <div className="absolute bottom-4 left-6 right-6 h-1 rounded-full bg-white/20">
          <div className="h-full w-[42%] rounded-full bg-blue-400" />
        </div>
      </div>
    </div>
  );
}

function SettingPill({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-slate-100 px-2.5 py-2 text-slate-700">
      <span className="text-slate-500">{label}</span>
      <span className="ml-1 font-medium text-slate-900">{value}</span>
    </div>
  );
}

function WaveBars({ active }: { active: boolean }) {
  return (
    <div className="flex h-24 items-center justify-center gap-1.5 rounded-xl bg-gradient-to-br from-slate-100 to-blue-50">
      {Array.from({ length: 20 }).map((_, index) => (
        <motion.span
          key={index}
          className="w-1.5 rounded-full bg-blue-500"
          animate={{
            height: active ? [14, 24 + (index % 5) * 7, 18] : 12,
            opacity: active ? [0.55, 1, 0.6] : 0.58
          }}
          transition={{
            duration: 0.85 + (index % 4) * 0.1,
            repeat: active ? Infinity : 0,
            ease: "easeInOut"
          }}
        />
      ))}
    </div>
  );
}

function TranscriptCard({ segment }: { segment: TranscriptSegment }) {
  const text = segment.correction ? segment.correction.final : segment.zh;

  return (
    <motion.article
      layout
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8 }}
      transition={{ duration: 0.25 }}
      className="rounded-2xl border border-white/80 bg-white/72 p-4 shadow-sm backdrop-blur"
    >
      <div className="mb-3 flex items-center justify-between gap-3">
        <span className="mono text-xs text-slate-500">{segment.time}</span>
        <span className={`rounded-full border px-2.5 py-1 text-xs font-medium ${stateClass(segment.state)}`}>
          {stateLabel(segment.state)}
        </span>
      </div>
      <p className="text-sm leading-6 text-slate-600">{segment.source}</p>
      <p className="mt-2 text-base font-semibold leading-7 text-slate-950">{text}</p>
    </motion.article>
  );
}

function CorrectionExplainer({ segment, compact = false }: { segment?: TranscriptSegment; compact?: boolean }) {
  if (!segment?.correction) {
    return (
      <div className="min-h-0 rounded-2xl border border-white/80 bg-white/64 p-4 shadow-sm backdrop-blur">
        <div className="text-sm font-semibold text-slate-950">纠错什么时候发生？</div>
        <p className="mt-2 text-sm leading-6 text-slate-600">
          当前字幕先快速生成。后续上下文更完整、术语表命中或后台复核发现问题时，系统会回写刚才那一段。
        </p>
      </div>
    );
  }

  return (
    <div className="min-h-0 min-w-0 overflow-hidden rounded-2xl border border-blue-200 bg-blue-50/78 p-4 shadow-sm backdrop-blur">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-blue-950">00:10 的字幕在 00:12 被自动纠错</div>
          <p className="mt-1 text-sm text-slate-600">{segment.correction.trigger}</p>
        </div>
        <CheckCircle size={22} weight="fill" className="text-blue-600" />
      </div>
      <div className={`mt-4 grid gap-3 ${compact ? "" : ""}`}>
        <div className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
          {segment.correction.first}
        </div>
        <ArrowRight size={18} className="text-blue-600" />
        <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-sm font-semibold text-emerald-900">
          {segment.correction.final}
        </div>
      </div>
    </div>
  );
}
