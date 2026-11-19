import clsx from "clsx";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  Check,
  CheckCircle2,
  Cpu,
  FlaskConical,
  Mic,
  Pause,
  Play,
  RefreshCw,
  Save,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Trash2,
  Volume2,
} from "lucide-react";
import { api, type AssistantConfig, type AssistantSettings, type VoicemailConfig, type VoicemailItem } from "../lib/api";
import { Alert, Badge, Button, Card, CardBody, CardHeader, Collapsible, Field, Input, Segmented, Select, Spinner, Switch, Textarea, useToast } from "./ui";

// Realtime speech models on Volcano Engine and Model Studio.
const models = [
  { id: "doubao-seeduplex-3.0", name: "豆包 Seeduplex 3.0", note: "推荐 · 火山引擎端到端实时语音大模型，超拟人全双工" },
  { id: "qwen3.8-omni-flash-realtime", name: "Qwen3.8 Omni Flash", note: "阿里云百炼 · 最新，反应快，能听懂“嗯”“喂”不打断" },
  { id: "qwen3.5-omni-plus-realtime", name: "Qwen3.5 Omni Plus", note: "阿里云百炼 · 理解和表达更好，费用更高" },
  { id: "qwen3.5-omni-flash-realtime", name: "Qwen3.5 Omni Flash", note: "阿里云百炼 · 便宜、快" },
  { id: "qwen3-omni-flash-realtime", name: "Qwen3 Omni Flash", note: "阿里云百炼 · 旧版，按声音停顿断句" },
];

type Voice = { id: string; name: string };
// Mandarin voices of each model family.
const voicesDoubao: Voice[] = [
  { id: "zh_female_vv_jupiter_bigtts", name: "灿灿 Vivi 2.0（活泼女声 · 推荐）" },
  { id: "zh_female_xiaohe_jupiter_bigtts", name: "小何 2.0（甜美活泼女声）" },
  { id: "zh_male_yunzhou_jupiter_bigtts", name: "云舟 2.0（清爽沉稳男声）" },
  { id: "zh_male_xiaotian_jupiter_bigtts", name: "小天 2.0（清爽磁性男声）" },
];
const voices38: Voice[] = [
  { id: "Tina", name: "甜甜 Tina（温暖女声）" },
  { id: "Liora Mira", name: "清欢 Liora Mira（温柔女声）" },
  { id: "Mia", name: "舒然 Mia（温柔女声）" },
  { id: "Katerina", name: "卡捷琳娜 Katerina（成熟女声）" },
  { id: "Raymond", name: "林川野 Raymond（清亮男声）" },
  { id: "Ryan", name: "甜茶 Ryan（男声）" },
  { id: "Cindy", name: "林欣宜 Cindy（台湾口音女声）" },
];
const voices35: Voice[] = [
  { id: "Tina", name: "甜甜 Tina（温暖女声）" },
  { id: "Serena", name: "苏瑶 Serena（女声）" },
  { id: "Liora Mira", name: "清欢 Liora Mira（温柔女声）" },
  { id: "Mia", name: "舒然 Mia（温柔女声）" },
  { id: "Maia", name: "四月 Maia（女声）" },
  { id: "Katerina", name: "卡捷琳娜 Katerina（成熟女声）" },
  { id: "Ethan", name: "晨煦 Ethan（男声）" },
  { id: "Theo Calm", name: "予安 Theo Calm（沉稳男声）" },
  { id: "Raymond", name: "林川野 Raymond（清亮男声）" },
  { id: "Evan", name: "江晨 Evan（男声）" },
  { id: "Cindy", name: "林欣宜 Cindy（台湾口音女声）" },
];
const voices3: Voice[] = [
  { id: "Cherry", name: "Cherry（女声，默认）" },
  { id: "Jennifer", name: "Jennifer（女声）" },
  { id: "Katerina", name: "Katerina（成熟女声）" },
  { id: "Ethan", name: "Ethan（男声）" },
  { id: "Ryan", name: "Ryan（男声）" },
  { id: "Elias", name: "Elias（男声）" },
];

function voicesFor(model: string): Voice[] {
  if (model.includes("doubao") || model.includes("seeduplex")) return voicesDoubao;
  if (model.startsWith("qwen3-omni")) return voices3;
  if (model.startsWith("qwen3.5")) return voices35;
  return voices38;
}

const DOUBAO_ENDPOINT = "wss://openspeech.bytedance.com/api/v3/duplex/realtime/dialogue";
const isDoubaoModel = (m: string) => m.includes("doubao") || m.includes("seeduplex");

// The endpoint choice: derived from the classifier, a region of the
// workspace, or a URL typed in.
type EndpointMode = "auto" | "ap-southeast-1" | "cn-beijing" | "custom";
const regionURL = /^wss:\/\/([^./]+)\.(ap-southeast-1|cn-beijing)\.maas\.aliyuncs\.com\/api-ws\/v1\/realtime$/;
const buildRegionURL = (workspace: string, region: string) => `wss://${workspace || "{WorkspaceId}"}.${region}.maas.aliyuncs.com/api-ws/v1/realtime`;

function endpointMode(url: string): EndpointMode {
  if (!url || url === DOUBAO_ENDPOINT) return "auto";
  const m = url.match(regionURL);
  return m ? (m[2] as EndpointMode) : "custom";
}

const defaultVoicemail = (model: string): VoicemailConfig => ({
  enabled: false,
  ringDelaySec: 10,
  voice: voicesFor(model)[0].id,
  speechRate: 1.1,
  style: "warm",
  text: "您好，我现在不方便接听电话，请在听到提示音后直接留言，我看到后会尽快给您回电，谢谢。",
  activeId: "",
  items: [],
});

const fields = (d: AssistantConfig): AssistantConfig => {
  const vm = d.voicemail ?? defaultVoicemail(d.model);
  return {
    enabled: d.enabled,
    ringDelaySec: d.ringDelaySec,
    greeting: d.greeting,
    instructions: d.instructions,
    model: d.model,
    voice: d.voice,
    maxDurationSec: d.maxDurationSec,
    url: d.url,
    speechRate: d.speechRate,
    ownerName: d.ownerName,
    style: d.style,
    discloseAI: d.discloseAI,
    dailyLimit: d.dailyLimit,
    perCallerMax: d.perCallerMax,
    maxTurns: d.maxTurns,
    record: d.record,
    keepDays: d.keepDays,
    voicemail: {
      enabled: vm.enabled ?? false,
      ringDelaySec: vm.ringDelaySec ?? 10,
      voice: vm.voice || voicesFor(d.model)[0].id,
      speechRate: vm.speechRate || 1.1,
      style: vm.style || "warm",
      text: vm.text || defaultVoicemail(d.model).text,
      activeId: vm.activeId || (vm.items?.[0]?.id ?? ""),
      items: (vm.items ?? []).map((it) => ({ ...it })),
    },
  };
};

const num = (v: string) => Math.max(0, Math.round(Number(v) || 0));

const styleLabel: Record<string, string> = {
  warm: "亲切",
  formal: "正式",
  brief: "简洁",
};

export function AssistantSection() {
  const toast = useToast();
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ["assistant"], queryFn: () => api.get<AssistantSettings>("/api/settings/assistant") });
  const [form, setForm] = useState<AssistantConfig | null>(null);
  const [apiKey, setApiKey] = useState<string | null>(null); // null: keep the stored key
  const [mode, setMode] = useState<EndpointMode>("auto");
  const [workspace, setWorkspace] = useState("");
  const [customModel, setCustomModel] = useState(false);
  const [busy, setBusy] = useState<"save" | "test" | "generate" | null>(null);
  const [replacingId, setReplacingId] = useState<string | null>(null);
  const [newClipName, setNewClipName] = useState("");
  const [playingId, setPlayingId] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [testResult, setTestResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (data && !form) {
      setForm(fields(data));
      setMode(endpointMode(data.url));
      setWorkspace(data.url.match(regionURL)?.[1] ?? data.workspaceId);
      setCustomModel(!models.some((m) => m.id === data.model));
    }
  }, [data, form]);

  useEffect(() => {
    return () => {
      if (audioRef.current) {
        audioRef.current.pause();
        audioRef.current = null;
      }
    };
  }, []);

  if (!data || !form) {
    return (
      <Card className="flex h-40 items-center justify-center">
        <Spinner />
      </Card>
    );
  }

  const set = (patch: Partial<AssistantConfig>) => {
    setForm((prev) => (prev ? { ...prev, ...patch } : prev));
    setTestResult(null);
    setSaveError(null);
  };

  const setVoicemail = (patch: Partial<VoicemailConfig>) => {
    setForm((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        voicemail: { ...prev.voicemail, ...patch },
      };
    });
    setSaveError(null);
  };

  // 自动联动：当用户打开「语音留言」开关时，自动将「通话录音」切换为“录下所有通话”
  const toggleVoicemailEnabled = (enabled: boolean) => {
    setForm((prev) => {
      if (!prev) return prev;
      const nextRecord = enabled ? "all" : prev.record;
      return {
        ...prev,
        record: nextRecord,
        voicemail: { ...prev.voicemail, enabled },
      };
    });
    setSaveError(null);
    if (enabled && form.record !== "all") {
      toast.ok("已开启语音留言，通话录音已自动切换为“录下所有通话”");
    }
  };

  const pickModel = (model: string) => {
    const list = voicesFor(model);
    const isDoubao = isDoubaoModel(model);
    const nextVoice = list.some((v) => v.id === form.voice) ? form.voice : list[0].id;
    const nextVmVoice = list.some((v) => v.id === form.voicemail.voice) ? form.voicemail.voice : list[0].id;
    const patch: Partial<AssistantConfig> = {
      model,
      voice: nextVoice,
      voicemail: { ...form.voicemail, voice: nextVmVoice },
    };
    if (isDoubao && (form.url.includes("aliyuncs.com") || mode === "ap-southeast-1" || mode === "cn-beijing")) {
      setMode("auto");
      patch.url = "";
    } else if (!isDoubao && form.url.includes("bytedance.com")) {
      setMode("auto");
      patch.url = "";
    }
    set(patch);
  };
  const pickMode = (m: EndpointMode) => {
    setMode(m);
    if (m === "auto") set({ url: "" });
    else if (m === "custom") set({ url: form.url || (isDoubao ? DOUBAO_ENDPOINT : data.derivedEndpoint.split("?")[0]) });
    else set({ url: buildRegionURL(workspace, m) });
  };
  const changeWorkspace = (id: string) => {
    setWorkspace(id);
    if (mode === "ap-southeast-1" || mode === "cn-beijing") set({ url: buildRegionURL(id, mode) });
  };

  const payload = () => ({ ...form, apiKey: apiKey === null ? undefined : apiKey });
  const dirty = apiKey !== null || JSON.stringify(form) !== JSON.stringify(fields(data));
  const isDoubao = isDoubaoModel(form.model);
  const effective = isDoubao
    ? (form.url || DOUBAO_ENDPOINT)
    : (form.url || data.derivedEndpoint.split("?")[0]) + (form.url.includes("model=") ? "" : `?model=${form.model}`);
  const voices = voicesFor(form.model);

  const conflictError =
    form.enabled && form.voicemail.enabled
      ? "“语音留言”与“AI 接听助手”功能冲突，不能同时开启，请先关闭其中一个"
      : form.voicemail.enabled && form.record !== "all"
        ? "开启“语音留言”时，“通话录音”必须设置为“录下所有通话”"
        : null;

  const save = async () => {
    if (form.enabled && form.voicemail.enabled) {
      const msg = "“语音留言”与“AI 接听助手”功能冲突，不能同时开启，请先关闭其中一个";
      setSaveError(msg);
      toast.error(msg);
      return;
    }
    if (form.voicemail.enabled && form.record !== "all") {
      const msg = "开启“语音留言”时，“通话录音”必须设置为“录下所有通话”";
      setSaveError(msg);
      toast.error(msg);
      return;
    }
    setBusy("save");
    setSaveError(null);
    try {
      const next = await api.put<AssistantSettings>("/api/settings/assistant", payload());
      qc.setQueryData(["assistant"], next);
      setForm(fields(next));
      setApiKey(null);
      if (next.voicemail?.enabled) {
        toast.ok("语音留言已开启并保存");
      } else if (next.enabled) {
        toast.ok("AI 接听助手已开启并保存");
      } else {
        toast.ok("设置已保存");
      }
    } catch (e) {
      const msg = (e as Error).message;
      setSaveError(msg);
      toast.error(msg);
    } finally {
      setBusy(null);
    }
  };

  const test = async () => {
    setBusy("test");
    setTestResult(null);
    try {
      const r = await api.post<{ latencyMs: number; format: string }>("/api/settings/assistant/test", payload());
      setTestResult({ ok: true, text: `连接成功：${form.model}，音频格式 ${r.format}，建立会话耗时 ${r.latencyMs} ms` });
    } catch (e) {
      setTestResult({ ok: false, text: (e as Error).message });
    } finally {
      setBusy(null);
    }
  };

  const generateVoicemail = async (replaceId = "") => {
    const text = form.voicemail.text.trim();
    if (!text) {
      toast.error("请先输入要生成的语音留言文本内容");
      return;
    }
    if (!replaceId && form.voicemail.items.length >= 3) {
      toast.error("最多只能保存 3 段语音留言，请点击下方某段语音的“替换”或“删除”后再生成");
      return;
    }
    setBusy("generate");
    setReplacingId(replaceId || null);
    try {
      const next = await api.post<AssistantSettings>("/api/settings/assistant/voicemail/generate", {
        ...payload(),
        name: newClipName.trim(),
        replaceId,
      });
      qc.setQueryData(["assistant"], next);
      const nextVm = fields(next).voicemail;
      setVoicemail({
        items: nextVm.items,
        activeId: nextVm.activeId,
      });
      setNewClipName("");
      toast.ok(replaceId ? "语音留言已重新生成并替换" : "AI 语音留言已生成并保存到 data/voicemail/");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setBusy(null);
      setReplacingId(null);
    }
  };

  const deleteVoicemailItem = async (item: VoicemailItem) => {
    if (playingId === item.id && audioRef.current) {
      audioRef.current.pause();
      setPlayingId(null);
    }
    try {
      const next = await api.del<AssistantSettings>(`/api/settings/assistant/voicemail/${encodeURIComponent(item.id)}`);
      qc.setQueryData(["assistant"], next);
      const nextVm = fields(next).voicemail;
      setVoicemail({
        items: nextVm.items,
        activeId: nextVm.activeId,
      });
      toast.ok(`已删除「${item.name}」`);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const updateItemName = (id: string, name: string) => {
    setVoicemail({
      items: form.voicemail.items.map((it) => (it.id === id ? { ...it, name } : it)),
    });
  };

  const persistItemName = async (item: VoicemailItem) => {
    const trimmed = item.name.trim();
    if (!trimmed) return;
    try {
      const next = await api.patch<AssistantSettings>(`/api/settings/assistant/voicemail/${encodeURIComponent(item.id)}`, {
        name: trimmed,
        active: form.voicemail.activeId === item.id,
      });
      qc.setQueryData(["assistant"], next);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const togglePlay = (item: VoicemailItem) => {
    if (playingId === item.id && audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
      setPlayingId(null);
      return;
    }
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
    const audio = new Audio(`/api/settings/assistant/voicemail/${encodeURIComponent(item.id)}/audio?t=${encodeURIComponent(item.createdAt)}`);
    audioRef.current = audio;
    setPlayingId(item.id);
    audio.onended = () => {
      setPlayingId(null);
      audioRef.current = null;
    };
    audio.onerror = () => {
      setPlayingId(null);
      audioRef.current = null;
      toast.error("播放语音留言失败，请确认音频文件存在");
    };
    audio.play().catch((err) => {
      setPlayingId(null);
      audioRef.current = null;
      toast.error(`播放失败：${(err as Error).message}`);
    });
  };

  return (
    <div className="space-y-5">
      {conflictError && (
        <Alert tone="bad" title="设置存在冲突，无法保存">
          {conflictError}
        </Alert>
      )}

      {/* 1. AI 接听助手 */}
      <Card glow={form.enabled}>
        <CardHeader
          icon={Bot}
          title="AI 接听助手"
          subtitle="来电响铃一会儿没人接时，由 AI 接听、问清楚对方是谁和来意，挂断后在通话记录里留下小结"
          action={<Switch checked={form.enabled} onChange={(v) => set({ enabled: v })} label="自动接听来电" />}
        />
        <CardBody className="space-y-4">
          {data.problem && !dirty && (
            <Alert tone="warn" title="还不能接听">
              {data.problem}
            </Alert>
          )}

          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="响铃多久后接听" hint="这段时间里你可以先在网页上接听，或者点“不用 AI”">
              <Select value={String(form.ringDelaySec)} onChange={(e) => set({ ringDelaySec: Number(e.target.value) })}>
                {[...new Set([0, 5, 10, 15, 20, 30, form.ringDelaySec])]
                  .sort((a, b) => a - b)
                  .map((s) => (
                    <option key={s} value={s}>
                      {s === 0 ? "立即接听" : `${s} 秒`}
                    </option>
                  ))}
              </Select>
            </Field>
            <Field label="声音">
              <Select value={form.voice} onChange={(e) => set({ voice: e.target.value })}>
                {voices.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name}
                  </option>
                ))}
                {!voices.some((v) => v.id === form.voice) && <option value={form.voice}>{form.voice}</option>}
              </Select>
            </Field>
            <Field label="语速" hint="在本机加快播放，音调不变">
              <Select value={String(form.speechRate)} onChange={(e) => set({ speechRate: Number(e.target.value) })}>
                {[...new Set([1, 1.1, 1.2, 1.3, form.speechRate])]
                  .sort((a, b) => a - b)
                  .map((r) => (
                    <option key={r} value={r}>
                      {r === 1 ? "原速" : `${r} 倍${r === 1.1 ? "（默认）" : ""}`}
                    </option>
                  ))}
              </Select>
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="怎么称呼你" hint="AI 会说“我会转告张先生”；留空就不提名字">
              <Input value={form.ownerName} onChange={(e) => set({ ownerName: e.target.value })} maxLength={20} placeholder="例如 张先生、王姐" />
            </Field>
            <Field label="说话风格">
              <Segmented
                value={form.style}
                onChange={(v) => set({ style: v })}
                options={[
                  { value: "warm", label: "亲切" },
                  { value: "formal", label: "正式" },
                  { value: "brief", label: "简洁" },
                ]}
              />
            </Field>
          </div>
          <Field
            label="开场白"
            hint={
              form.discloseAI
                ? "接通后 AI 先说的话，例如“喂，您好，请问是哪位呀？”。说完它会接着表明自己是 AI 助手"
                : "接通后 AI 先说的话。像平常接电话一样就好，例如“喂，您好，请问是哪位呀？”。不用介绍自己是谁；对方认真问是不是 AI 时，它会如实回答"
            }
          >
            <Input value={form.greeting} onChange={(e) => set({ greeting: e.target.value })} maxLength={200} />
          </Field>
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium">接通后主动说明自己是 AI</p>
              <p className="mt-0.5 text-xs text-muted">有些地方的法规要求 AI 接听电话时主动表明身份，请按你所在地区的要求设置</p>
            </div>
            <Switch checked={form.discloseAI} onChange={(v) => set({ discloseAI: v })} label="接通后主动说明自己是 AI" />
          </div>
          <Field label="补充说明（可选）" hint="告诉 AI 怎么应对常见来电，例如：快递外卖请放门口；工作的事请发邮件。来电者可能套出这里写的内容，不要写住址、行程、证件号这类信息">
            <Textarea rows={3} value={form.instructions} onChange={(e) => set({ instructions: e.target.value })} maxLength={2000} placeholder="快递请放在门口，不用等我。" />
          </Field>

          <Collapsible
            icon={Cpu}
            title="模型与接口"
            subtitle={`${models.find((m) => m.id === form.model)?.name ?? form.model} · ${
              isDoubao
                ? mode === "auto"
                  ? "火山引擎官方默认"
                  : "自定义地址"
                : { auto: "跟随智能分类", "ap-southeast-1": "新加坡", "cn-beijing": "北京", custom: "自定义地址" }[mode]
            }${apiKey === null && data.hasOwnKey ? " · 单独的 API Key" : ""}`}
            defaultOpen={!!data.problem}
          >
            <div className="space-y-2">
              <div className="text-[13px] font-medium">实时语音模型</div>
              <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {models.map((m) => {
                  const active = !customModel && form.model === m.id;
                  return (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => {
                        setCustomModel(false);
                        pickModel(m.id);
                      }}
                      className={clsx("rounded-2xl px-3.5 py-3 text-left ring-1 transition", active ? "bg-accent/10 ring-accent/50" : "bg-panel-2 ring-line hover:ring-line-strong")}
                    >
                      <div className="flex items-center justify-between gap-2 text-sm font-medium">
                        {m.name}
                        {active && <CheckCircle2 className="size-4 shrink-0 text-accent" />}
                      </div>
                      <div className="mt-0.5 text-[11px] text-muted">{m.note}</div>
                    </button>
                  );
                })}
                <button
                  type="button"
                  onClick={() => setCustomModel(true)}
                  className={clsx("rounded-2xl px-3.5 py-3 text-left ring-1 transition", customModel ? "bg-accent/10 ring-accent/50" : "bg-panel-2 ring-line hover:ring-line-strong")}
                >
                  <div className="flex items-center justify-between gap-2 text-sm font-medium">
                    自定义
                    {customModel && <CheckCircle2 className="size-4 shrink-0 text-accent" />}
                  </div>
                  <div className="mt-0.5 text-[11px] text-muted">填写其他实时模型或快照版本</div>
                </button>
              </div>
              {customModel && <Input value={form.model} onChange={(e) => pickModel(e.target.value.trim())} placeholder="doubao-seeduplex-3.0 或 qwen3.8-omni-flash-realtime" className="font-mono" />}
            </div>

            <div className="space-y-2">
              <div className="text-[13px] font-medium">接口地址</div>
              <Segmented
                size="sm"
                value={isDoubao ? (mode === "custom" ? "custom" : "auto") : mode}
                onChange={pickMode}
                options={
                  isDoubao
                    ? [
                        { value: "auto", label: "火山引擎官方默认" },
                        { value: "custom", label: "自定义地址" },
                      ]
                    : [
                        { value: "auto", label: "跟随智能分类" },
                        { value: "ap-southeast-1", label: "新加坡" },
                        { value: "cn-beijing", label: "北京" },
                        { value: "custom", label: "自定义" },
                      ]
                }
              />
              {!isDoubao && (mode === "ap-southeast-1" || mode === "cn-beijing") && (
                <Field label="业务空间 ID" hint="在百炼控制台的“业务空间”里查看，形如 ws-xxxxxxxx。北京和新加坡的业务空间和 API Key 不通用">
                  <Input value={workspace} onChange={(e) => changeWorkspace(e.target.value.trim())} placeholder="ws-xxxxxxxx" className="font-mono" />
                </Field>
              )}
              {mode === "custom" && <Input value={form.url} onChange={(e) => set({ url: e.target.value.trim() })} placeholder={isDoubao ? "wss://openspeech.bytedance.com/api/v3/duplex/realtime/dialogue" : "wss://…/api-ws/v1/realtime"} className="font-mono text-xs" />}
              {mode === "auto" && !isDoubao && !data.derivedEndpoint && <p className="text-xs text-warn">“智能分类”里还没有百炼的 Base URL，请选择地域并填写业务空间 ID。</p>}
              {(form.url || isDoubao || data.derivedEndpoint) && (
                <p className="font-mono text-[11px] break-all text-subtle">
                  实际连接：{effective}
                </p>
              )}
            </div>

            <Field
              label="API Key"
              hint={
                apiKey === null && data.hasOwnKey
                  ? `已单独保存：${data.ownKeyHint}。只保存在本机（加密）。`
                  : isDoubao
                    ? "请填写火山引擎的 API Key（在火山引擎控制台 > API Key 管理中创建获取）。"
                    : data.hasSharedKey
                      ? "留空则使用“智能分类”里的 API Key。接口地域不同时需要填写对应地域的 Key。"
                      : "“智能分类”里也没有 Key，请在这里填写。"
              }
            >
              {apiKey === null && data.hasOwnKey ? (
                <div className="flex gap-2">
                  <Input value={data.ownKeyHint} disabled className="font-mono" />
                  <Button onClick={() => setApiKey("")}>更换</Button>
                </div>
              ) : (
                <div className="flex gap-2">
                  <Input type="password" value={apiKey ?? ""} onChange={(e) => setApiKey(e.target.value)} placeholder={data.hasSharedKey ? "留空：使用智能分类的 Key" : "sk-..."} className="font-mono" autoComplete="off" />
                  {apiKey !== null && (
                    <Button variant="ghost" onClick={() => setApiKey(null)}>
                      取消
                    </Button>
                  )}
                </div>
              )}
            </Field>
          </Collapsible>

          <Collapsible icon={ShieldCheck} title="防止恶意消耗" subtitle={`每天最多 ${form.dailyLimit} 通，同一号码 ${form.perCallerMax} 通，每通最多回复 ${form.maxTurns} 次、${form.maxDurationSec} 秒`}>
            <p className="text-xs text-muted">AI 只记录来意，闲聊、问答、让它写东西或改规则一律拒绝，纠缠就挂断。下面的上限超过后不再由 AI 接听，电话按普通来电响铃。</p>
            <div className="grid gap-3 sm:grid-cols-4">
              <Field label="每天最多接听">
                <Input type="number" min={1} max={500} value={form.dailyLimit} onChange={(e) => set({ dailyLimit: num(e.target.value) })} />
              </Field>
              <Field label="同一号码每天最多">
                <Input type="number" min={1} max={50} value={form.perCallerMax} onChange={(e) => set({ perCallerMax: num(e.target.value) })} />
              </Field>
              <Field label="每通最多回复几次">
                <Input type="number" min={3} max={50} value={form.maxTurns} onChange={(e) => set({ maxTurns: num(e.target.value) })} />
              </Field>
              <Field label="每通最长（秒）">
                <Input type="number" min={30} max={600} value={form.maxDurationSec} onChange={(e) => set({ maxDurationSec: num(e.target.value) })} />
              </Field>
            </div>
          </Collapsible>

          <Collapsible icon={Mic} title="通话录音" subtitle={form.record === "off" ? "不录音" : `${form.record === "all" ? "录下所有通话" : "录下 AI 接听的通话"}，保留 ${form.keepDays} 天`}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="通话录音" hint="录音保存在本机数据目录的 recordings/ 下，可以在“电话”页播放和下载。开启“语音留言”时必须设为“录下所有通话”">
                <Select value={form.record} onChange={(e) => set({ record: e.target.value as AssistantConfig["record"] })}>
                  <option value="assistant">录下 AI 接听的通话</option>
                  <option value="all">录下所有通话</option>
                  <option value="off">不录音</option>
                </Select>
              </Field>
              <Field label="录音保留天数" hint="超过天数的录音自动删除，通话文字记录保留">
                <Input type="number" min={1} max={3650} value={form.keepDays} disabled={form.record === "off"} onChange={(e) => set({ keepDays: num(e.target.value) })} />
              </Field>
            </div>
          </Collapsible>
        </CardBody>
      </Card>

      {/* 2. 语音留言版块 */}
      <Card glow={form.voicemail.enabled}>
        <CardHeader
          icon={Volume2}
          title="语音留言"
          subtitle="来电超时未接时自动接起并播放预先用 AI 生成好的语音留言，同时通过“录下所有通话”录制对方的留言内容（与 AI 接听助手互斥）"
          action={<Switch checked={form.voicemail.enabled} onChange={toggleVoicemailEnabled} label="开启语音留言" />}
        />
        <CardBody className="space-y-4">
          {form.voicemail.enabled && form.enabled && (
            <Alert tone="bad" title="与“AI 接听助手”功能冲突">
              “语音留言”与“AI 接听助手”不能同时开启，请先关闭其中一个后再保存设置。
            </Alert>
          )}
          {form.voicemail.enabled && form.record !== "all" && (
            <Alert tone="bad" title="通话录音需设为“录下所有通话”">
              语音留言正常工作的前提是“通话录音”必须设为“录下所有通话”，否则将无法保存设置。
              <div className="mt-2">
                <Button size="sm" onClick={() => set({ record: "all" })}>
                  一键切换为“录下所有通话”
                </Button>
              </div>
            </Alert>
          )}

          {/* 基础参数配置：响铃多久后接听、声音、语速、说话风格 */}
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="响铃多久后接听" hint="超过该时间未接听时自动播放选中的语音留言">
              <Select value={String(form.voicemail.ringDelaySec)} onChange={(e) => setVoicemail({ ringDelaySec: Number(e.target.value) })}>
                {[...new Set([0, 5, 10, 15, 20, 30, form.voicemail.ringDelaySec])]
                  .sort((a, b) => a - b)
                  .map((s) => (
                    <option key={s} value={s}>
                      {s === 0 ? "立即接听" : `${s} 秒`}
                    </option>
                  ))}
              </Select>
            </Field>
            <Field label="声音" hint="使用“模型与接口”中当前所选语音模型的音色">
              <Select value={form.voicemail.voice} onChange={(e) => setVoicemail({ voice: e.target.value })}>
                {voices.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name}
                  </option>
                ))}
                {!voices.some((v) => v.id === form.voicemail.voice) && <option value={form.voicemail.voice}>{form.voicemail.voice}</option>}
              </Select>
            </Field>
            <Field label="语速" hint="在本机加快播放，音调不变">
              <Select value={String(form.voicemail.speechRate)} onChange={(e) => setVoicemail({ speechRate: Number(e.target.value) })}>
                {[...new Set([1, 1.1, 1.2, 1.3, form.voicemail.speechRate])]
                  .sort((a, b) => a - b)
                  .map((r) => (
                    <option key={r} value={r}>
                      {r === 1 ? "原速" : `${r} 倍${r === 1.1 ? "（默认）" : ""}`}
                    </option>
                  ))}
              </Select>
            </Field>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="说话风格" hint="决定 AI 生成语音留言时的语气风格">
              <Segmented
                value={form.voicemail.style}
                onChange={(v) => setVoicemail({ style: v })}
                options={[
                  { value: "warm", label: "亲切" },
                  { value: "formal", label: "正式" },
                  { value: "brief", label: "简洁" },
                ]}
              />
            </Field>
            <Field label="新建语音名称（可选）" hint="生成时给这段语音起个名字，留空则自动命名为“语音留言 1/2/3”">
              <Input
                value={newClipName}
                onChange={(e) => setNewClipName(e.target.value)}
                maxLength={40}
                placeholder={`例如：工作忙未接、快递放门口（默认 语音留言 ${Math.min(3, form.voicemail.items.length + 1)}）`}
              />
            </Field>
          </div>

          {/* 文本输入与 AI生成语音 按钮 */}
          <Field
            label="语音留言文本内容"
            hint={`点击“AI生成语音”将调用当前选定的语音模型（${models.find((m) => m.id === form.model)?.name ?? form.model}），生成 .wav 格式音频保存至 data/voicemail/ 下`}
          >
            <Textarea
              rows={3}
              value={form.voicemail.text}
              onChange={(e) => setVoicemail({ text: e.target.value })}
              maxLength={500}
              placeholder="您好，我现在不方便接听电话，请在听到提示音后直接留言，我看到后会尽快给您回电，谢谢。"
            />
          </Field>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <span className="text-xs text-muted">
              已保存 <strong className="text-fg">{form.voicemail.items.length} / 3</strong> 段语音留言（保存在本机 <code className="font-mono text-fg">data/voicemail/*.wav</code>）
            </span>
            <Button
              icon={Sparkles}
              loading={busy === "generate" && !replacingId}
              disabled={busy !== null || form.voicemail.items.length >= 3}
              onClick={() => generateVoicemail("")}
            >
              {form.voicemail.items.length >= 3 ? "已达 3 段上限（可替换或删除下方语音）" : "AI生成语音"}
            </Button>
          </div>

          {/* 多段语音管理（上限 3 段：命名、选择与播放） */}
          <div className="space-y-2.5 pt-1">
            <div className="text-[13px] font-medium">已保存的语音留言（单选指定当前启用的语音）</div>
            {form.voicemail.items.length === 0 ? (
              <div className="rounded-2xl bg-panel-2 px-4 py-6 text-center text-xs text-muted ring-1 ring-line">
                还没有已保存的语音留言。请在上方输入留言文本，点击 <strong>“AI生成语音”</strong> 生成并保存（最多可保存 3 段）。
              </div>
            ) : (
              <div className="space-y-2.5">
                {form.voicemail.items.map((item, idx) => {
                  const isActive = form.voicemail.activeId === item.id;
                  const isPlaying = playingId === item.id;
                  const voiceName = voices.find((v) => v.id === item.voice)?.name ?? item.voice;
                  return (
                    <div
                      key={item.id}
                      className={clsx(
                        "rounded-2xl p-3.5 ring-1 transition",
                        isActive ? "bg-accent/8 ring-accent/40" : "bg-panel-2 ring-line hover:ring-line-strong",
                      )}
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        {/* 左侧：单选启用 + 自定义命名输入 */}
                        <div className="flex min-w-0 flex-1 items-center gap-3">
                          <button
                            type="button"
                            role="radio"
                            aria-checked={isActive}
                            onClick={() => setVoicemail({ activeId: item.id })}
                            title="设为当前启用的语音留言"
                            className={clsx(
                              "flex size-5 shrink-0 items-center justify-center rounded-full ring-1 transition",
                              isActive ? "bg-gradient-accent text-white ring-transparent" : "bg-panel-solid ring-line-strong hover:ring-accent",
                            )}
                          >
                            {isActive && <Check className="size-3" strokeWidth={3} />}
                          </button>

                          <div className="min-w-0 flex-1 space-y-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="text-xs font-mono text-subtle">#{idx + 1}</span>
                              <Input
                                value={item.name}
                                onChange={(e) => updateItemName(item.id, e.target.value)}
                                onBlur={() => persistItemName(item)}
                                maxLength={40}
                                className="h-8 max-w-xs text-sm font-medium"
                                placeholder="自定义语音名称"
                              />
                              {isActive ? (
                                <Badge tone="ok">当前启用</Badge>
                              ) : (
                                <button
                                  type="button"
                                  onClick={() => setVoicemail({ activeId: item.id })}
                                  className="text-xs text-accent hover:underline"
                                >
                                  选用此段
                                </button>
                              )}
                            </div>
                            <p className="text-xs text-fg/90 line-clamp-2">“{item.text}”</p>
                            <p className="text-[11px] text-subtle font-mono">
                              data/voicemail/{item.fileName} · {item.durationSec || 0}s · {voiceName} · {item.speechRate}x · {styleLabel[item.style] ?? item.style}
                            </p>
                          </div>
                        </div>

                        {/* 右侧：试听播放、重新生成替换、删除 */}
                        <div className="flex shrink-0 flex-wrap items-center gap-2 self-end sm:self-center">
                          <Button
                            size="sm"
                            variant={isPlaying ? "primary" : "secondary"}
                            icon={isPlaying ? Pause : Play}
                            onClick={() => togglePlay(item)}
                          >
                            {isPlaying ? "停止试听" : "试听"}
                          </Button>
                          <Button
                            size="sm"
                            variant="secondary"
                            icon={RefreshCw}
                            loading={busy === "generate" && replacingId === item.id}
                            disabled={busy !== null}
                            onClick={() => generateVoicemail(item.id)}
                            title="用上方当前文本和参数重新生成并替换此段语音"
                          >
                            替换
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            icon={Trash2}
                            disabled={busy !== null}
                            onClick={() => deleteVoicemailItem(item)}
                          >
                            删除
                          </Button>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </CardBody>
      </Card>

      {/* 底部提示与统一保存栏 */}
      <Card>
        <CardBody className="space-y-3">
          <p className="flex items-start gap-1.5 text-[11px] leading-relaxed text-subtle">
            <ShieldAlert className="mt-px size-3.5 shrink-0" />
            通话的声音会实时发送到所选服务商（火山引擎或阿里云百炼）的语音模型，无法像短信那样先去除敏感信息；通话文字记录、小结及生成的语音留言音频（data/voicemail/*.wav）只保存在本机。
          </p>
          {saveError && <p className="rounded-xl bg-bad/10 px-3 py-2 text-xs text-bad">{saveError}</p>}
          {testResult && <p className={clsx("rounded-xl px-3 py-2 text-xs", testResult.ok ? "bg-ok/10 text-ok" : "bg-bad/10 text-bad")}>{testResult.text}</p>}
          <div className="flex flex-wrap justify-end gap-2">
            <Button icon={FlaskConical} loading={busy === "test"} onClick={test}>
              测试连接
            </Button>
            <Button variant="primary" icon={Save} loading={busy === "save"} disabled={!dirty && !conflictError} onClick={save}>
              保存
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
