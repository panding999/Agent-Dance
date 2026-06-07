export type CreateLiveSessionRequest = {
  mode: "live";
  source_language: string;
  target_language: string;
  voice_enabled: boolean;
};

export type LiveSession = {
  id: string;
  mode: string;
  source_language: string;
  target_language: string;
  voice_enabled: boolean;
  status: string;
  created_at: string;
  updated_at: string;
  closed_at?: string;
};

export type LiveServerEvent = {
  type: string;
  session_id?: string;
  sequence?: number;
  segmentId?: string;
  text?: string;
  sourceText?: string;
  startMs?: number;
  endMs?: number;
  before?: string;
  after?: string;
  reason?: string;
  code?: string;
  message?: string;
};

type AudioContextWithWebkit = Window &
  typeof globalThis & {
    webkitAudioContext?: typeof AudioContext;
  };

const TARGET_SAMPLE_RATE = 16000;
const FRAME_SAMPLE_COUNT = 1280;
const SCRIPT_PROCESSOR_BUFFER_SIZE = 4096;

export async function createLiveSession(request: CreateLiveSessionRequest): Promise<LiveSession> {
  const response = await fetch(buildHTTPURL("/api/sessions"), {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(request)
  });

  if (!response.ok) {
    throw new Error(await readHTTPError(response));
  }

  return (await response.json()) as LiveSession;
}

export function connectLiveWebSocket(sessionId: string): WebSocket {
  return new WebSocket(buildLiveWebSocketURL(sessionId));
}

export function parseLiveServerEvent(data: MessageEvent["data"]): LiveServerEvent | null {
  if (typeof data !== "string") {
    return null;
  }

  return JSON.parse(data) as LiveServerEvent;
}

export class LiveAudioStreamer {
  private context: AudioContext | null = null;
  private mediaStream: MediaStream | null = null;
  private source: MediaStreamAudioSourceNode | null = null;
  private processor: ScriptProcessorNode | null = null;
  private mutedGain: GainNode | null = null;
  private pendingSamples = new Int16Array(0);
  private sequence = 0;

  constructor(private readonly onFrame: (frame: ArrayBuffer) => void) {}

  async start() {
    if (!navigator.mediaDevices?.getUserMedia) {
      throw new Error("当前浏览器不支持麦克风采集");
    }

    const AudioContextConstructor =
      window.AudioContext ?? (window as AudioContextWithWebkit).webkitAudioContext;
    if (!AudioContextConstructor) {
      throw new Error("当前浏览器不支持 Web Audio");
    }

    this.mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true
      }
    });

    this.context = new AudioContextConstructor({ sampleRate: TARGET_SAMPLE_RATE });
    if (this.context.state === "suspended") {
      await this.context.resume();
    }

    this.source = this.context.createMediaStreamSource(this.mediaStream);
    this.processor = this.context.createScriptProcessor(SCRIPT_PROCESSOR_BUFFER_SIZE, 1, 1);
    this.mutedGain = this.context.createGain();
    this.mutedGain.gain.value = 0;

    this.processor.onaudioprocess = (event) => {
      if (!this.context) {
        return;
      }
      const input = event.inputBuffer.getChannelData(0);
      const resampled = downsample(input, this.context.sampleRate, TARGET_SAMPLE_RATE);
      this.appendSamples(floatToInt16PCM(resampled));
    };

    this.source.connect(this.processor);
    this.processor.connect(this.mutedGain);
    this.mutedGain.connect(this.context.destination);
  }

  async stop() {
    this.processor?.disconnect();
    this.source?.disconnect();
    this.mutedGain?.disconnect();
    this.mediaStream?.getTracks().forEach((track) => track.stop());

    if (this.context && this.context.state !== "closed") {
      await this.context.close();
    }

    this.context = null;
    this.mediaStream = null;
    this.source = null;
    this.processor = null;
    this.mutedGain = null;
    this.pendingSamples = new Int16Array(0);
  }

  private appendSamples(samples: Int16Array) {
    if (samples.length === 0) {
      return;
    }

    const merged = new Int16Array(this.pendingSamples.length + samples.length);
    merged.set(this.pendingSamples);
    merged.set(samples, this.pendingSamples.length);
    this.pendingSamples = merged;

    while (this.pendingSamples.length >= FRAME_SAMPLE_COUNT) {
      const frameSamples = this.pendingSamples.slice(0, FRAME_SAMPLE_COUNT);
      this.pendingSamples = this.pendingSamples.slice(FRAME_SAMPLE_COUNT);
      this.sequence += 1;
      this.onFrame(buildBrowserAudioFrame(this.sequence, Date.now(), frameSamples));
    }
  }
}

export function buildBrowserAudioFrame(
  sequence: number,
  timestampMs: number,
  pcm: Int16Array
): ArrayBuffer {
  const headerSize = 12;
  const buffer = new ArrayBuffer(headerSize + pcm.byteLength);
  const view = new DataView(buffer);

  view.setUint32(0, sequence, true);
  view.setBigUint64(4, BigInt(timestampMs), true);
  new Uint8Array(buffer, headerSize).set(new Uint8Array(pcm.buffer, pcm.byteOffset, pcm.byteLength));

  return buffer;
}

function buildHTTPURL(path: string) {
  const base = trimTrailingSlash(process.env.NEXT_PUBLIC_BACKEND_HTTP_URL ?? "");
  if (base) {
    return `${base}${path}`;
  }
  return path;
}

function buildLiveWebSocketURL(sessionId: string) {
  const configuredWSBase = trimTrailingSlash(process.env.NEXT_PUBLIC_BACKEND_WS_URL ?? "");
  if (configuredWSBase) {
    return `${configuredWSBase}/api/live/ws?sessionId=${encodeURIComponent(sessionId)}`;
  }

  const configuredHTTPBase = trimTrailingSlash(process.env.NEXT_PUBLIC_BACKEND_HTTP_URL ?? "");
  if (configuredHTTPBase) {
    const url = new URL(configuredHTTPBase);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    url.pathname = "/api/live/ws";
    url.search = "";
    url.searchParams.set("sessionId", sessionId);
    return url.toString();
  }

  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const url = new URL("/api/live/ws", `${protocol}//${window.location.host}`);
  url.searchParams.set("sessionId", sessionId);
  return url.toString();
}

function trimTrailingSlash(value: string) {
  return value.trim().replace(/\/+$/, "");
}

async function readHTTPError(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string };
    if (payload.error) {
      return payload.error;
    }
  } catch {
    // Fall through to status text.
  }
  return `${response.status} ${response.statusText}`;
}

function downsample(input: Float32Array, inputRate: number, targetRate: number) {
  if (inputRate === targetRate) {
    return input.slice();
  }

  const ratio = inputRate / targetRate;
  const length = Math.floor(input.length / ratio);
  const output = new Float32Array(length);

  for (let i = 0; i < length; i += 1) {
    const start = Math.floor(i * ratio);
    const end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    let count = 0;

    for (let j = start; j < end; j += 1) {
      sum += input[j];
      count += 1;
    }

    output[i] = count > 0 ? sum / count : input[start] ?? 0;
  }

  return output;
}

function floatToInt16PCM(input: Float32Array) {
  const output = new Int16Array(input.length);

  for (let i = 0; i < input.length; i += 1) {
    const sample = Math.max(-1, Math.min(1, input[i]));
    output[i] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
  }

  return output;
}
