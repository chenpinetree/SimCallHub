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
        title="手机与电脑 SIP 客户端配置图文指引"
        subtitle="推荐使用免费开源的 Linphone 或 MicroSIP"
        defaultOpen
      >
        <div className="space-y-4 p-5 text-sm text-muted leading-relaxed">
          <div className="rounded-xl bg-panel-2 p-4 ring-1 ring-line space-y-2">
            <p className="font-semibold text-fg flex items-center gap-2">
              <Smartphone className="size-4 text-accent" /> 手机端（iOS / Android 推荐使用 Linphone）：
            </p>
            <ol className="list-decimal list-inside space-y-1 text-xs">
              <li>在 App Store 或各大应用商店搜索并安装 <strong>Linphone</strong>（开源免费，支持后台保活振铃）；</li>
              <li>打开 App，点击 <strong>“使用 SIP 账号”</strong>（Use SIP Account）；</li>
              <li>
                <strong>用户名（Username）</strong> 填写：<code className="text-fg font-mono">{username}</code>
              </li>
              <li>
                <strong>SIP 域名（SIP Domain）</strong> 填写：<code className="text-fg font-mono">{currentHost}:{sipPort}</code>
              </li>
              <li>
                <strong>密码（Password）</strong> 填写：<code className="text-fg font-mono">{password}</code>
              </li>
              <li>
                <strong>传输方式（Transport）</strong> 勾选：<strong className="text-fg">UDP</strong>；
              </li>
              <li>点击 <strong>“登录 / 连接”</strong>，顶部状态变绿显示“已连接”即表示接入成功！</li>
            </ol>
          </div>

          <div className="rounded-xl bg-panel-2 p-4 ring-1 ring-line space-y-2">
            <p className="font-semibold text-fg flex items-center gap-2">
              <Laptop className="size-4 text-accent" /> 电脑端（Windows / Mac 推荐使用 MicroSIP 或 Telephone）：
            </p>
            <ol className="list-decimal list-inside space-y-1 text-xs">
              <li>Windows 用户可下载极简轻量的 <strong>MicroSIP</strong>；Mac 用户可在 App Store 免费下载 <strong>Telephone</strong>；</li>
              <li>在账号设置中新增 SIP 账号；</li>
              <li>
                <strong>SIP Server / Proxy</strong> 填入 <code className="text-fg font-mono">{currentHost}:{sipPort}</code>；
              </li>
              <li>
                <strong>Username / User</strong> 填入 <code className="text-fg font-mono">{username}</code>，密码填入设置的密码；
              </li>
              <li>保存后即可随时在电脑上接听/拨打手机号码。</li>
            </ol>
          </div>

          <Alert tone="ok" title="局域网与远程使用提示">
            如果希望在离开家时用 4G/5G 网络接听家里的固话，只需在路由器或通过 Tailscale / DDNS 把 UDP 5060 端口映射出去，手机软电话就能随时随地在全球各地接听家里的 4G 电话！
          </Alert>
        </div>
      </Collapsible>
    </div>
  );
}
