import React, { useEffect, useMemo, useRef, useState } from "react";
import { IntentMeta, renderIntentContent } from "./components/IntentViews";

type Message = {
  role: "user" | "assistant" | "system";
  content: string;
  intent?: IntentMeta;
};

// Types mirrored from backend payloads
type PR = {
  number: number;
  title: string;
  author: string;
  status: string;
  url: string;
  repository: string;
};

type Comment = {
  author: string;
  body: string;
  timestamp: string;
  type: string;
  path?: string;
  line?: number;
};

export default function App() {
  const [sessionId, setSessionId] = useState<string>("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [streaming, setStreaming] = useState<boolean>(false);
  const [recording, setRecording] = useState<boolean>(false);
  const [isSpeaking, setIsSpeaking] = useState<boolean>(false);
  const [greetingSpoken, setGreetingSpoken] = useState<boolean>(false);
  const [preferredVoiceName, setPreferredVoiceName] = useState<string | null>(
    null
  );
  const [voiceOnly, setVoiceOnly] = useState<boolean>(true);
  const [elevenVoiceId, setElevenVoiceId] = useState<string | null>(
    "cgSgspJ2msm6clMCkdW9"
  );
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const [recorderMime, setRecorderMime] = useState<string>("");
  const audioStreamRef = useRef<MediaStream | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const vadTimerRef = useRef<number | null>(null);
  const silenceMsRef = useRef<number>(0);
  const heardSpeechRef = useRef<boolean>(false);
  const audioContextUnlockedRef = useRef<boolean>(false);
  const firstSpeechAtRef = useRef<number | null>(null);
  const historyRef = useRef<HTMLDivElement | null>(null);

  // GitHub auth status
  const [ghAuthed, setGhAuthed] = useState<boolean>(false);
  const [ghUsername, setGhUsername] = useState<string>("");
  const [ghBusy, setGhBusy] = useState<boolean>(false);

  const apiBase = useMemo(() => {
    const explicit = (import.meta as any).env?.VITE_API_BASE as
      | string
      | undefined;
    if (explicit && explicit.trim()) return explicit.replace(/\/$/, "");
    const url = new URL(window.location.href);
    const apiPort = url.port === "5173" ? "8080" : url.port;
    return `${url.protocol}//${url.hostname}:${apiPort}`;
  }, []);

  useEffect(() => {
    // Cookie-based sessions: no localStorage needed
    // Session will be managed via HTTP-only cookie
    setSessionId("");
    const savedVoice = localStorage.getItem("zanaElevenVoiceId");
    if (savedVoice) setElevenVoiceId(savedVoice);
  }, []);

  // Check GitHub status on session
  useEffect(() => {
    if (!sessionId) return;
    (async () => {
      try {
        await checkGitHubStatus();
      } catch {}
    })();
  }, [sessionId]);

  // Handle OAuth callback redirect
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("githubAuth") === "success") {
      // OAuth callback successful - close popup window
      (async () => {
        try {
          // Check status to update UI
          await checkGitHubStatus();
        } catch {}
        // Close the popup
        window.close();
      })();
    }
  }, []);

  // Load available voices and pick a pleasant English voice by preference
  useEffect(() => {
    const updateVoices = () => {
      const synth = window.speechSynthesis;
      if (!synth) return;
      const voices = synth.getVoices();
      const preferredOrder = [
        "Serena", // subtle UK English
        "Samantha",
        "Karen", // softer AU
        "Tessa",
        "Victoria",
        "Google US English",
        "Google UK English Female",
        "Alex",
      ];
      const pick =
        voices.find((v) => preferredOrder.some((n) => v.name.includes(n))) ||
        voices.find((v) => v.lang?.toLowerCase().startsWith("en"));
      if (pick) setPreferredVoiceName(pick.name);
    };
    updateVoices();
    if (typeof window !== "undefined" && "speechSynthesis" in window) {
      window.speechSynthesis.onvoiceschanged = updateVoices;
    }
  }, []);

  // Minimal unlock for TTS on some browsers
  useEffect(() => {
    if (greetingSpoken) return;
    const onFirstInteract = () => {
      try {
        (window as any).speechSynthesis?.resume?.();
      } catch {}
      window.removeEventListener("pointerdown", onFirstInteract, true);
      window.removeEventListener("touchstart", onFirstInteract, true);
      window.removeEventListener("click", onFirstInteract, true);
    };
    window.addEventListener("pointerdown", onFirstInteract, true);
    window.addEventListener("touchstart", onFirstInteract, true);
    window.addEventListener("click", onFirstInteract, true);
    return () => {
      window.removeEventListener("pointerdown", onFirstInteract, true);
      window.removeEventListener("touchstart", onFirstInteract, true);
      window.removeEventListener("click", onFirstInteract, true);
    };
  }, [greetingSpoken]);

  function rememberSession(id: string) {
    try {
      // Cookie-based sessions: session ID is managed by backend via HTTP-only cookie
      // Just update local state for UI purposes
      // eslint-disable-next-line no-console
      console.log("[session] received", { previous: sessionId, next: id });
    } catch {}
    setSessionId(id);
  }

  function isAllowedIntent(t?: string): boolean {
    return t === "show_prs" || t === "show_comments";
  }

  async function sendText(prefill?: string) {
    const trimmed = (prefill ?? "").trim();
    if (!trimmed || streaming) return;

    setStreaming(true);
    try {
      // Cookie-based sessions: backend manages session ID via cookie
      // eslint-disable-next-line no-console
      console.log("[session] sending /api/chat");
      const res = await fetch(`${apiBase}/api/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include", // Include cookies
        body: JSON.stringify({ message: trimmed }), // No sessionId needed
      });
      const sid = res.headers.get("X-Session-Id");
      // eslint-disable-next-line no-console
      console.log("[session] /api/chat header X-Session-Id", sid);
      if (sid) rememberSession(sid);
      if (!res.ok) throw new Error("Chat failed");
      const data = await res.json();
      // eslint-disable-next-line no-console
      console.log("[session] /api/chat JSON sessionId", data?.sessionId);
      const assistant = (data?.reply as string) || "";
      const intent = data?.intent as IntentMeta;
      if (intent?.type === "require_github_auth") {
        await startGitHubAuth();
      }
      if (assistant) {
        // Always speak the assistant's reply
        speak(assistant);
        // Display only when intent is allowed (or plain clarify)
        if (isAllowedIntent(intent?.type)) {
          setMessages((m) => [
            ...m,
            { role: "assistant", content: assistant, intent },
          ]);
        }
      }
    } catch (e) {
      alert("Chat failed");
    } finally {
      setStreaming(false);
    }
  }

  async function checkGitHubStatus(): Promise<boolean> {
    try {
      // Cookie-based sessions: no query parameter needed
      const res = await fetch(`${apiBase}/api/github/status`, {
        credentials: "include", // Include cookies
      });
      if (!res.ok) return false;
      const data = await res.json();
      const authed = Boolean(data?.authenticated);
      setGhAuthed(authed);
      setGhUsername((data?.username as string) || "");
      return authed;
    } catch {
      return false;
    }
  }

  async function startGitHubAuth() {
    try {
      setGhBusy(true);
      // Cookie-based sessions: no query parameter needed
      const res = await fetch(`${apiBase}/api/github/auth`, {
        credentials: "include", // Include cookies
      });
      if (!res.ok) throw new Error("Auth init failed");
      const sidHeader = res.headers.get("X-Session-Id");
      const data = await res.json();
      const newSid = (data?.sessionId as string) || sidHeader || sessionId;
      if (newSid && newSid !== sessionId) rememberSession(newSid);
      const url = data?.url as string;
      if (!url) throw new Error("No auth URL");
      const w = window.open(url, "github-auth", "width=520,height=640");
      const start = Date.now();
      const tick = async () => {
        const ok = await checkGitHubStatus();
        if (ok) {
          try {
            w?.close();
          } catch {}
          return;
        }
        if (Date.now() - start > 90000) return;
        setTimeout(tick, 1500);
      };
      setTimeout(tick, 1500);
    } catch (e) {
      alert("GitHub auth failed. Please try again.");
    } finally {
      setGhBusy(false);
    }
  }

  async function speak(text: string) {
    if (!text) return;
    const provider = (import.meta as any).env?.VITE_TTS_PROVIDER as
      | string
      | undefined;
    if (provider === "eleven") {
      try {
        const res = await fetch(`${apiBase}/api/tts`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include", // Include cookies
          body: JSON.stringify({ text, voiceId: elevenVoiceId || undefined }),
        });
        if (!res.ok) throw new Error("TTS failed");
        const buf = await res.arrayBuffer();
        const audio = new Audio(
          URL.createObjectURL(new Blob([buf], { type: "audio/mpeg" }))
        );
        setIsSpeaking(true);
        audio.onended = () => {
          setIsSpeaking(false);
          if (voiceOnly && !recording) {
            setTimeout(() => {
              startRecording().catch(() => {});
            }, 180);
          }
        };
        audio.onerror = () => {
          setIsSpeaking(false);
          if (voiceOnly && !recording) {
            setTimeout(() => {
              startRecording().catch(() => {});
            }, 250);
          }
        };
        await audio.play();
        audioContextUnlockedRef.current = true;
        return;
      } catch (e) {
        // fallthrough to browser TTS
      }
    }
    const synth = (window as any).speechSynthesis as
      | SpeechSynthesis
      | undefined;
    if (!synth) return;
    const utt = new SpeechSynthesisUtterance(text);
    utt.rate = 0.94;
    utt.pitch = 0.95;
    utt.volume = 0.9;
    if (preferredVoiceName) {
      const v = synth.getVoices().find((x) => x.name === preferredVoiceName);
      if (v) utt.voice = v as any;
    }
    utt.onstart = () => {
      setIsSpeaking(true);
      if (recording) {
        try {
          stopRecording();
        } catch {}
      }
    };
    utt.onend = () => {
      setIsSpeaking(false);
      if (voiceOnly && !recording) {
        setTimeout(() => {
          startRecording().catch(() => {});
        }, 180);
      }
    };
    utt.onerror = () => {
      setIsSpeaking(false);
      if (voiceOnly && !recording) {
        setTimeout(() => {
          startRecording().catch(() => {});
        }, 250);
      }
    };
    synth.cancel();
    synth.speak(utt);
  }

  function speakGreeting() {
    if (greetingSpoken) return;
    try {
      if (sessionStorage.getItem("gitter_greeting_spoken") === "1") return;
    } catch {}
    const line = "Hey, I am GITTER. Talk to me about your pull requests.";
    try {
      setGreetingSpoken(true);
      try {
        sessionStorage.setItem("gitter_greeting_spoken", "1");
      } catch {}
      if (!isSpeaking) speak(line);
    } catch {}
  }

  function chooseSupportedMime(): string {
    // Prefer Opus in WebM where available; fall back to mp3/m4a
    const candidates = [
      "audio/webm;codecs=opus",
      "audio/webm",
      "audio/ogg",
      "audio/mpeg",
      // Safari often only supports MP4 containers for audio recordings
      "audio/mp4",
      // WAV is a last resort (very large); many browsers don't support it with MediaRecorder
      "audio/wav",
    ];
    for (const c of candidates) {
      if ((window as any).MediaRecorder && MediaRecorder.isTypeSupported?.(c)) {
        return c;
      }
    }
    return "";
  }

  async function startRecording() {
    if (recording) return;
    const stream =
      audioStreamRef.current && audioStreamRef.current.active
        ? audioStreamRef.current
        : await navigator.mediaDevices.getUserMedia({ audio: true });
    audioStreamRef.current = stream;
    const mime = chooseSupportedMime();
    const mr = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
    setRecorderMime(mime || mr.mimeType || "");
    mediaRecorderRef.current = mr;
    chunksRef.current = [];
    mr.ondataavailable = (e) => {
      if (e.data.size > 0) chunksRef.current.push(e.data);
    };
    mr.onstop = async () => {
      const blob = new Blob(chunksRef.current, {
        type: recorderMime || mime || "audio/webm",
      });
      // Small grace delay before sending, to avoid cutting off late speech
      await new Promise((r) => setTimeout(r, 300));
      await sendVoice(blob);
      chunksRef.current = [];
      if (vadTimerRef.current) {
        window.clearInterval(vadTimerRef.current);
        vadTimerRef.current = null;
      }
      if (analyserRef.current) {
        try {
          analyserRef.current.disconnect();
        } catch {}
        analyserRef.current = null;
      }
    };
    // Start without timeslice to ensure a single, well-formed container
    // Some decoders fail with concatenated chunked blobs
    mr.start();
    setRecording(true);

    try {
      const AC: any =
        (window as any).AudioContext || (window as any).webkitAudioContext;
      const audioCtx: AudioContext = audioCtxRef.current || new AC();
      audioCtxRef.current = audioCtx;
      const source = audioCtx.createMediaStreamSource(stream);
      const analyser = audioCtx.createAnalyser();
      analyser.fftSize = 1024;
      analyser.smoothingTimeConstant = 0.85;
      source.connect(analyser);
      analyserRef.current = analyser;

      heardSpeechRef.current = false;
      silenceMsRef.current = 0;
      firstSpeechAtRef.current = null;
      const intervalMs = 120;
      const threshold = 4; // VAD energy threshold (more sensitive to quiet speech)
      const stopSilenceMs = 2600; // require longer silence to avoid cutting off natural pauses
      const minSpeechMs = 3000; // require longer total speech before stopping
      vadTimerRef.current = window.setInterval(() => {
        if (!analyserRef.current) return;
        const buf = new Uint8Array(analyserRef.current.fftSize);
        analyserRef.current.getByteTimeDomainData(buf);
        let sum = 0;
        for (let i = 0; i < buf.length; i++) sum += Math.abs(buf[i] - 128);
        const avg = sum / buf.length;
        const isVoice = avg > threshold;
        if (isVoice) {
          heardSpeechRef.current = true;
          silenceMsRef.current = 0;
          if (!firstSpeechAtRef.current) firstSpeechAtRef.current = Date.now();
        } else if (heardSpeechRef.current) {
          silenceMsRef.current += intervalMs;
          if (
            // Stop only if we've heard enough speech and then sufficient silence
            silenceMsRef.current > stopSilenceMs &&
            (firstSpeechAtRef.current
              ? Date.now() - firstSpeechAtRef.current >= minSpeechMs
              : false) &&
            mediaRecorderRef.current?.state === "recording"
          ) {
            try {
              mediaRecorderRef.current.stop();
            } catch {}
            setRecording(false);
            heardSpeechRef.current = false;
            silenceMsRef.current = 0;
          }
        }
      }, intervalMs);
    } catch {}
  }

  function stopRecording() {
    const mr = mediaRecorderRef.current;
    if (!mr) return;
    mr.stop();
    setRecording(false);
    if (vadTimerRef.current) {
      window.clearInterval(vadTimerRef.current);
      vadTimerRef.current = null;
    }
    if (analyserRef.current) {
      try {
        analyserRef.current.disconnect();
      } catch {}
      analyserRef.current = null;
    }
  }

  // UI simplified – no manual input or voice picker

  // Auto-start recording on mount for pure voice flow
  useEffect(() => {
    (async () => {
      try {
        if (!greetingSpoken) speakGreeting();
        if (sessionStorage.getItem("gitter_voice_initialized") !== "1") {
          sessionStorage.setItem("gitter_voice_initialized", "1");
          await startRecording();
        }
      } catch {}
    })();
    // Only once
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function extFromMime(m: string): string {
    if (!m) return "webm";
    if (m.includes("mp4") || m.includes("m4a")) return "m4a";
    if (m.includes("mpeg") || m.includes("mp3")) return "mp3";
    if (m.includes("ogg")) return "ogg";
    if (m.includes("wav")) return "wav";
    return "webm";
  }

  async function sendVoice(blob: Blob) {
    try {
      setStreaming(true);
      const form = new FormData();
      // Cookie-based sessions: no sessionId needed in form
      // eslint-disable-next-line no-console
      console.log("[session] sending /api/voice");
      const type = blob.type || recorderMime || "audio/webm";
      const ext = extFromMime(type);
      form.append("file", blob, `input.${ext}`);
      const res = await fetch(`${apiBase}/api/voice`, {
        method: "POST",
        credentials: "include", // Include cookies
        body: form,
      });
      const sid = res.headers.get("X-Session-Id");
      // eslint-disable-next-line no-console
      console.log("[session] /api/voice header X-Session-Id", sid);
      if (sid) rememberSession(sid);
      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText);
      }
      const data = await res.json();
      // eslint-disable-next-line no-console
      console.log("[session] /api/voice JSON sessionId", data?.sessionId);
      const tr = (data?.transcript as string) || "";
      const rp = (data?.reply as string) || "";
      const intent = data?.intent as IntentMeta;
      if (intent?.type === "require_github_auth") {
        await startGitHubAuth();
      }
      if (rp) {
        // Always speak the assistant's reply
        speak(rp);
        // Only display when intent is allowed
        if (isAllowedIntent(intent?.type)) {
          setMessages((m) => [
            ...m,
            { role: "assistant", content: rp, intent },
          ]);
        }
      }
    } catch (e) {
      console.error("Voice flow failed", e);

      if (voiceOnly && !recording) {
        setTimeout(() => {
          startRecording().catch(() => {});
        }, 300);
      }
    } finally {
      setStreaming(false);
    }
  }

  // Auto-scroll to newest assistant message
  useEffect(() => {
    const el = historyRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  return (
    <div className="container">
      <div className="header">
        <div className="brand">GITTER</div>
        <div className="top-actions">
          {!ghAuthed ? (
            <button onClick={startGitHubAuth} disabled={ghBusy}>
              {ghBusy ? "Connecting…" : "Connect GitHub"}
            </button>
          ) : (
            <span>Connected{ghUsername ? ` as ${ghUsername}` : ""}</span>
          )}
        </div>
      </div>

      <div className="split">
        {/* Voice on the left */}
        <div className="voice-pane">
          <div className="voice-center">
            <div className="orb">
              <span className="ring r1"></span>
              <span className="ring r2"></span>
              <span className="ring r3"></span>
            </div>
            {streaming ? (
              <div className="status-text">Thinking....</div>
            ) : isSpeaking ? (
              <div className="status-text">Speaking…</div>
            ) : recording ? (
              <div className="status-text">Listening…</div>
            ) : (
              <div className="status-text">Ready</div>
            )}
          </div>
        </div>

        {/* Content on the right: scrollable assistant history */}
        <div className="content-card">
          <div className="content-body">
            <div className="chat" ref={historyRef}>
              {messages
                .filter((m) => m.role === "assistant")
                .map((m, i) => (
                  <div key={i} className="msg-block">
                    <div className="msg assistant">
                      <div>{m.content}</div>
                      {renderIntentContent(m.intent)}
                    </div>
                    <div className="msg-divider"></div>
                  </div>
                ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
