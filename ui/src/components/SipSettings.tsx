import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  PhoneCall,
  CheckCircle2,
  CircleOff,
  Smartphone,
  Laptop,
  KeyRound,
  Save,
  Radio,
  HelpCircle,
  Eye,
  EyeOff,
  Server,
  Network,
  User,
  Shield,
  Layers,
  CheckCircle,
} from "lucide-react";
import { api, type SipSettings } from "../lib/api";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Badge,
  Spinner,
  Input,
  Field,
  useToast,
  CopyButton,
  Collapsible,
  Alert,
} from "./ui";

export function SipSection() {
  const qc = useQueryClient();
  const toast = useToast();

  const { data: sip, isLoading } = useQuery<SipSettings>({
    queryKey: ["sip-settings"],
    queryFn: () => api.get<SipSettings>("/api/settings/sip"),
    refetchInterval: 5000, // 5秒轮询在线客户端
  });

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [ringTimeout, setRingTimeout] = useState(20);
  const [showPass, setShowPass] = useState(false);

  useEffect(() => {
    if (sip) {
      setUsername(sip.username || "1001");
      setPassword(sip.password || "12345678");
      setRingTimeout(sip.ring_timeout || 20);
    }
  }, [sip]);

  const mutation = useMutation({
    mutationFn: (data: { username: string; password: string; ring_timeout: number }) =>
      api.put<SipSettings>("/api/settings/sip", data),
    onSuccess: (updated) => {
      toast.ok("SIP 分机配置已保存并生效");
      qc.setQueryData(["sip-settings"], updated);
    },
    onError: (err: Error) => {
      toast.error(`保存失败: ${err.message}`);
    },
  });

  // 获取当前访问的局域网 IP
  const currentHost = typeof window !== "undefined" ? window.location.hostname : "127.0.0.1";
  const sipPort = "5060";
  const clients = sip?.clients ?? [];
  const hasOnlineClient = clients.length > 0;

  if (isLoading && !sip) {
    return (
      <div className="flex items-center justify-center py-16">
        <Spinner className="size-6 text-accent" />
        <span className="ml-3 text-sm text-muted">正在加载 SIP 分机设置...</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 1. 在线分机状态卡片 */}
      <Card>
        <CardHeader
          icon={PhoneCall}
          title={
            <div className="flex items-center gap-2">
              <span>分机实时在线状态</span>
              {hasOnlineClient ? (
                <Badge tone="ok" className="flex items-center gap-1">
                  <CheckCircle2 className="size-3" /> {clients.length} 个客户端在线
                </Badge>
              ) : (
                <Badge tone="muted" className="flex items-center gap-1">
                  <CircleOff className="size-3" /> 暂无在线客户端
                </Badge>
              )}
            </div>
          }
          subtitle="当前已注册到本网关的手机或电脑软电话分机"
        />
        <CardBody className="space-y-4">
          {hasOnlineClient ? (
            <div className="divide-y divide-line rounded-xl bg-panel-2 ring-1 ring-line">
              {clients.map((c, i) => (
                <div key={i} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex items-center gap-3">
                    <div className="flex size-10 items-center justify-center rounded-xl bg-ok/10 text-ok ring-1 ring-ok/20">
                      {c.user_agent.toLowerCase().includes("iphone") || c.user_agent.toLowerCase().includes("android") ? (
                        <Smartphone className="size-5" />
                      ) : (
                        <Laptop className="size-5" />
                      )}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-sm">分机 {c.username}</span>
                        <span className="text-xs text-muted font-mono">({c.addr})</span>
                      </div>
                      <p className="text-xs text-muted mt-0.5 font-mono break-all">{c.user_agent}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 self-end sm:self-center">
                    <Badge tone="ok" className="font-mono text-xs">
                      保活中: {c.expires_in_sec}s
                    </Badge>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="rounded-xl bg-panel-2 p-5 text-center ring-1 ring-line">
              <Radio className="mx-auto size-8 text-subtle" />
              <p className="mt-2 text-sm font-medium">尚未连接软电话客户端</p>
              <p className="mt-1 text-xs text-muted max-w-md mx-auto">
                请在手机（如 Linphone）或电脑上配置分机账号，连接成功后即可在此实时查看设备状态并免提接听 4G 来电。
              </p>
            </div>
          )}
        </CardBody>
      </Card>

      {/* 2. 连接参数一键复制 */}
      <Card>
        <CardHeader
          icon={Radio}
          title="分机连接参数（客户端配置）"
          subtitle="在 Linphone、MicroSIP 或其他 SIP 软电话中填入以下参数即可注册"
        />
        <CardBody>
          <div className="grid gap-3 sm:grid-cols-2">
            {/* 服务器地址 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <Server className="size-3.5" /> 域名 / 服务器 (Domain / Server)
                </span>
                <p className="font-mono text-sm font-medium mt-0.5 truncate">{currentHost}:{sipPort}</p>
              </div>
              <CopyButton text={`${currentHost}:${sipPort}`} label="复制" />
            </div>

            {/* 分机账号 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <User className="size-3.5" /> 分机账号 / 用户名 (User / Extension)
                </span>
                <p className="font-mono text-sm font-medium mt-0.5">{username}</p>
              </div>
              <CopyButton text={username} label="复制" />
            </div>

            {/* 分机密码 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <KeyRound className="size-3.5" /> 分机密码 (Password)
                </span>
                <div className="flex items-center gap-2 mt-0.5">
                  <p className="font-mono text-sm font-medium">
                    {showPass ? password : "••••••••"}
                  </p>
                  <button
                    type="button"
                    onClick={() => setShowPass(!showPass)}
                    className="text-muted hover:text-fg transition p-0.5"
                    title={showPass ? "隐藏密码" : "显示密码"}
                  >
                    {showPass ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  </button>
                </div>
              </div>
              <CopyButton text={password} label="复制" />
            </div>

            {/* 传输协议 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <Network className="size-3.5" /> 传输协议 (Transport Protocol)
                </span>
                <p className="font-mono text-sm font-medium mt-0.5">UDP (推荐) 或 TCP</p>
              </div>
              <CopyButton text="UDP" label="复制" />
            </div>

            {/* 音频编解码 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <Layers className="size-3.5" /> 支持音频编解码器 (Audio Codecs)
                </span>
                <p className="font-mono text-sm font-medium mt-0.5">PCMU (G.711u) / PCMA (G.711a)</p>
              </div>
              <CopyButton text="PCMU, PCMA" label="复制" />
            </div>

            {/* 一键完整 SIP URI */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <Shield className="size-3.5" /> 完整 SIP 地址 (SIP Address)
                </span>
                <p className="font-mono text-sm font-medium mt-0.5 truncate">
                  sip:{username}@{currentHost}:{sipPort}
                </p>
              </div>
              <CopyButton text={`sip:${username}@${currentHost}:${sipPort}`} label="复制" />
            </div>
          </div>
        </CardBody>
      </Card>

      {/* 3. 分机参数修改与呼叫行为设置 */}
      <Card>
        <CardHeader
          icon={KeyRound}
          title="分机账号与呼叫策略修改"
          subtitle="修改分机认证密码及呼叫转移超时，修改后立即在底层生效并持久保存"
        />
        <CardBody className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="分机账号" hint="默认为 1001，纯数字或英文">
              <Input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="1001"
              />
            </Field>

            <Field label="分机连接密码" hint="客户端连接校验密码">
              <Input
                type="text"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="设置分机连接密码"
              />
            </Field>
          </div>

          <Field
            label="分机振铃超时（秒）"
            hint="当模组收到外线来电时，分机软电话响铃超过此秒数未接听，系统将自动切入 AI 接听助手"
          >
            <div className="flex items-center gap-3">
              <Input
                type="number"
                min={5}
                max={60}
                value={ringTimeout}
                onChange={(e) => setRingTimeout(Math.max(5, Math.min(60, Number(e.target.value))))}
                className="w-32"
              />
              <span className="text-sm text-muted">秒（建议 15~30 秒）</span>
            </div>
          </Field>

          <div className="flex justify-end pt-2">
            <Button
              variant="primary"
              icon={Save}
              loading={mutation.isPending}
              onClick={() => {
                mutation.mutate({
                  username: username.trim(),
                  password: password.trim(),
                  ring_timeout: Number(ringTimeout),
                });
              }}
            >
              保存 SIP 设置
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* 4. 客户端配置向导 */}
      <Collapsible
        icon={HelpCircle}
        title="手机与电脑 SIP 客户端配置指引"
        subtitle="手机端推荐使用免费的 Zoiper 或支持推送的 Groundwire；电脑端推荐 MicroSIP"
        defaultOpen
      >
        <div className="space-y-6 p-5 text-sm leading-relaxed">
          {/* 手机端 Zoiper 配置指引 */}
          <div className="rounded-2xl bg-panel-2 p-5 ring-1 ring-line space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 pb-2 border-b border-line">
              <div className="flex items-center gap-2">
                <div className="flex size-7 items-center justify-center rounded-lg bg-accent/10 text-accent ring-1 ring-accent/20">
                  <Smartphone className="size-4" />
                </div>
                <h4 className="font-semibold text-fg text-base">
                  手机端配置（iOS / Android 推荐使用 Zoiper）
                </h4>
              </div>
              <span className="text-xs text-muted">
                免费推荐 <strong>Zoiper</strong>，需要锁屏推送推荐付费 App <strong>Groundwire</strong>
              </span>
            </div>

            {/* Step 1 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">1</span>
                <span>新建 SIP 账号：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3 ring-1 ring-line text-xs text-muted flex items-center flex-wrap gap-1.5">
                <span>进入 App 底部</span>
                <code className="text-fg font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Settings</code>
                <span>➔</span>
                <code className="text-fg font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Accounts</code>
                <span>➔ 点击右上角</span>
                <code className="text-fg font-semibold bg-panel-2 px-1.5 py-0.5 rounded">+</code>
                <span>➔ 弹窗选择</span>
                <code className="text-ok font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Yes</code>
                <span>➔ 选择</span>
                <code className="text-fg font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Manual configuration</code>
                <span>➔ 选择</span>
                <code className="text-accent font-semibold bg-panel-2 px-1.5 py-0.5 rounded">SIP account</code>
              </div>
            </div>

            {/* Step 2 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">2</span>
                <span>填入账号基础信息：</span>
              </p>
              <div className="ml-7 divide-y divide-line rounded-xl bg-panel-solid ring-1 ring-line text-xs overflow-hidden">
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Account name</span>
                  <span className="text-fg flex-1 font-mono">自定义（如 SimCallHub）</span>
                  <span className="text-subtle text-[11px]">随便填，方便识别</span>
                </div>
                <div className="flex items-center justify-between p-2.5 bg-accent/5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">Domain</span>
                  <span className="text-accent font-mono font-semibold flex-1">{currentHost}:{sipPort}</span>
                  <CopyButton text={`${currentHost}:${sipPort}`} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">User name</span>
                  <span className="text-fg font-mono font-semibold flex-1">{username}</span>
                  <CopyButton text={username} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">Password</span>
                  <span className="text-fg font-mono font-semibold flex-1">{password}</span>
                  <CopyButton text={password} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">Caller ID</span>
                  <span className="text-fg font-mono flex-1">{currentHost}:{sipPort}</span>
                  <CopyButton text={`${currentHost}:${sipPort}`} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Auth Username</span>
                  <span className="text-fg font-mono flex-1">{username}</span>
                  <span className="text-subtle text-[11px]">填 {username} 或留空跟随上面</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Use Outbound Proxy</span>
                  <span className="text-muted font-medium flex-1">关闭（灰色开关）</span>
                  <span className="text-subtle text-[11px]">务必保持关闭</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Outbound Proxy</span>
                  <span className="text-subtle flex-1 italic">留空无需填写</span>
                  <span className="text-subtle text-[11px]">留空</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Enable video FMTP</span>
                  <span className="text-muted flex-1">保持默认即可</span>
                  <span className="text-subtle text-[11px]">默认</span>
                </div>
              </div>
            </div>

            {/* Step 3 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">3</span>
                <span>Features（按键音 DTMF 设置）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3 ring-1 ring-line text-xs space-y-1.5">
                <p className="text-fg font-medium">
                  点击进入 <code className="bg-panel-2 px-1.5 py-0.5 rounded text-accent">Features</code> ➔ 选中 <code className="bg-panel-2 px-1.5 py-0.5 rounded text-ok font-semibold">inband</code>（带内音频），或保持 <code className="bg-panel-2 px-1.5 py-0.5 rounded">rfc-2833</code>。
                </p>
                <p className="text-subtle">
                  💡 <strong>强烈推荐切换为 inband</strong>：确保拨号给运营商（如按 1 查话费、按键导航）时，EC20 模组能够准确接收 DTMF 频段按键音。
                </p>
              </div>
            </div>

            {/* Step 4 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">4</span>
                <span>Network Settings（网络与 NAT 关键设置）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-3">
                <div className="grid gap-2.5 sm:grid-cols-2">
                  <div className="p-2.5 rounded-lg bg-panel-2 ring-1 ring-line">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-fg">Enable IPv6</span>
                      <Badge tone="muted">关闭（灰色）</Badge>
                    </div>
                    <p className="text-[11px] text-muted mt-1">
                      原因：避免手机在双栈 Wi-Fi 下优先尝试走公网 IPv6 解析，忽略了局域网内网 IPv4。
                    </p>
                  </div>

                  <div className="p-2.5 rounded-lg bg-panel-2 ring-1 ring-line">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-fg">Transports</span>
                      <Badge tone="ok">UDP</Badge>
                    </div>
                    <p className="text-[11px] text-muted mt-1">
                      原因：标准 SIP 采用 UDP 5060，响应迅速无长连接握手包袱。
                    </p>
                  </div>

                  <div className="p-2.5 rounded-lg bg-panel-2 ring-1 ring-line">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-fg">Enable STUN</span>
                      <Badge tone="bad">关闭（灰色）</Badge>
                    </div>
                    <p className="text-[11px] text-muted mt-1">
                      原因：开启时手机会连公网 stun.zoiper.com 获取外网 IP。在家庭局域网内开启会导致手机把内网 IP 换成公网 IP 发送，造成注册失败或通话单向无声。局域网务必关闭 STUN。
                    </p>
                  </div>

                  <div className="p-2.5 rounded-lg bg-panel-2 ring-1 ring-line">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-fg">Enable on start</span>
                      <Badge tone="ok">开启（绿色）</Badge>
                    </div>
                    <p className="text-[11px] text-muted mt-1">
                      原因：保证打开 App 时自动连接注册，防止错过分机来电。
                    </p>
                  </div>

                  <div className="p-2.5 rounded-lg bg-panel-2 ring-1 ring-line sm:col-span-2">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-fg">TLS Settings</span>
                      <Badge tone="muted">关闭（灰色）</Badge>
                    </div>
                    <p className="text-[11px] text-muted mt-1">
                      原因：传输通道使用的是 UDP，底层未启用 SIP TLS 加密引擎，关闭避免额外校验开销。
                    </p>
                  </div>
                </div>

                <div className="flex items-center gap-2 pt-1 text-ok font-medium">
                  <CheckCircle className="size-4 shrink-0" />
                  <span>设置完成后，点击右上角黑色【Save】保存，再点击左上角【&lt; Accounts】返回到上一页。</span>
                </div>
              </div>
            </div>

            {/* Step 5 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">5</span>
                <span>Audio Codecs（音频编解码器设置）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center gap-2">
                  <Badge tone="ok">仅保留 u-law (PCMU) 与 a-law (PCMA)</Badge>
                  <span className="text-muted">拖动至最顶部，关闭其他全部编码</span>
                </div>
                <div className="rounded-lg bg-panel-2 p-2.5 ring-1 ring-line text-muted leading-relaxed text-[11px] space-y-1">
                  <p>
                    • <strong>硬件原生匹配</strong>：移远 EC20 4G 模组与中国移动/联通/电信标准语音固网采用的正是 G.711 u-law（8000Hz）与 G.711 a-law，SimCallHub 原生直接处理这两个编码。
                  </p>
                  <p>
                    • <strong>避免无效协商</strong>：GSM、Speex、iLBC 是上古时期的有损压缩编码，网关并不支持。若开启并排在前面，每次打电话都要先协商失败再退回 G.711，容易导致接通延迟或偶尔单向无声。仅留这两项可实现秒级秒通。
                  </p>
                </div>
              </div>
            </div>

            {/* Step 6 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">6</span>
                <span>Video codecs（视频编解码器设置）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center gap-2">
                  <Badge tone="muted">关闭 VP8 与 H.264（全部置灰）</Badge>
                </div>
                <p className="text-[11px] text-muted leading-relaxed">
                  <strong>原因</strong>：SimCallHub 连接 4G 模块为纯蜂窝语音电话网关，运营商手机网络通话纯为音频，完全没有视频流。若开启 VP8，Zoiper 呼叫时会在 SIP SDP 信令中声明支持视频通道（m=video），不仅产生无效信令冗余和潜在兼容问题，还会唤醒手机摄像头硬件管线增加发热耗电。彻底关闭后纯语音直通，更省电且接听更迅速。
                </p>
              </div>
            </div>

            {/* Step 7 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">7</span>
                <span>Instant Messaging（即时消息与状态）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center gap-2">
                  <Badge tone="muted">全部关闭（将 IM Enabled 关掉）</Badge>
                </div>
                <p className="text-[11px] text-muted leading-relaxed">
                  <strong>原因</strong>：IM 是用于类似企业聊天软件（如 Skype）在 SIP 里收发文字聊天消息的；Presence 是探测对方是否“在线/离开/忙碌”。SimCallHub 是电话网关，您的短信收发在 Web 后台已有独立的分类管理系统，不需要通过 Zoiper 传文本。
                </p>
              </div>
            </div>

            {/* Step 8 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-ok/20 text-ok text-xs font-semibold">8</span>
                <span>一键注册与完成验证：</span>
              </p>
              <div className="ml-7 rounded-xl bg-ok/10 border border-ok/30 p-3.5 text-xs space-y-2">
                <p className="font-medium text-ok">
                  返回到账号设置主界面，点击大绿色按钮 <strong>【Register】</strong>。
                </p>
                <div className="flex items-center gap-2 text-fg">
                  <CheckCircle2 className="size-4 text-ok" />
                  <span>注册成功后，顶部会显示绿色的 <strong>Registration status: OK</strong>！</span>
                </div>
                <p className="text-muted text-[11px]">
                  此时您已可在页面顶部的“分机实时在线状态”中看到您的手机设备已在线，任何打入 SIM 卡的电话均可免提接听或录音。
                </p>
              </div>
            </div>
          </div>

          {/* 电脑端配置指引 */}
          <div className="rounded-2xl bg-panel-2 p-5 ring-1 ring-line space-y-3">
            <div className="flex items-center gap-2 pb-2 border-b border-line">
              <div className="flex size-7 items-center justify-center rounded-lg bg-accent/10 text-accent ring-1 ring-accent/20">
                <Laptop className="size-4" />
              </div>
              <h4 className="font-semibold text-fg text-base">
                电脑端配置（Windows / Mac 推荐使用 MicroSIP 或 Telephone）
              </h4>
            </div>
            <ol className="list-decimal list-inside space-y-1.5 text-xs text-muted">
              <li>Windows 用户可下载极简轻量的 <strong>MicroSIP</strong>；Mac 用户可在 App Store 免费下载 <strong>Telephone</strong>；</li>
              <li>在账号设置中新增 SIP 账号；</li>
              <li>
                <strong>SIP Server / Proxy</strong> 填入 <code className="text-fg font-mono">{currentHost}:{sipPort}</code>；
              </li>
              <li>
                <strong>Username / User</strong> 填入 <code className="text-fg font-mono">{username}</code>，密码填入设置的分机密码；
              </li>
              <li>保存后即可随时在电脑上免提接听/拨打手机号码。</li>
            </ol>
          </div>

          <Alert tone="ok" title="局域网与远程使用提示">
            如果希望在离开家时用 4G/5G 网络接听家里的固话，只需在路由器端口映射或通过 Tailscale / DDNS 把 UDP 5060 端口映射出去，手机软电话就能随时随地在全球各地接听家里的 4G 电话！
          </Alert>
        </div>
      </Collapsible>
    </div>
  );
}
