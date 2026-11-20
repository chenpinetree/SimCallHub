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
  Server,
  Network,
  Shield,
  Layers,
  CheckCircle,
  Plus,
  Trash2,
  Wand2,
} from "lucide-react";
import { api, type SipSettings, type SipExtensionConfig } from "../lib/api";
import { useStatus } from "../lib/hooks";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Badge,
  Spinner,
  Input,
  Select,
  Field,
  useToast,
  CopyButton,
  Collapsible,
  Alert,
} from "./ui";

export function SipSection() {
  const qc = useQueryClient();
  const toast = useToast();

  const { data: status } = useStatus();
  const devices = status?.devices ?? [];

  const { data: sip, isLoading } = useQuery<SipSettings>({
    queryKey: ["sip-settings"],
    queryFn: () => api.get<SipSettings>("/api/settings/sip"),
    refetchInterval: 5000, // 5秒轮询在线客户端
  });

  const [extensions, setExtensions] = useState<SipExtensionConfig[]>([]);
  const [ringTimeout, setRingTimeout] = useState(20);
  const [showPassMap, setShowPassMap] = useState<Record<number, boolean>>({});

  useEffect(() => {
    if (sip) {
      setRingTimeout(sip.ring_timeout || 20);
      if (sip.extensions && sip.extensions.length > 0) {
        setExtensions(sip.extensions);
      } else {
        setExtensions([
          {
            id: "default",
            username: sip.username || "1001",
            password: sip.password || "12345678",
            device_id: "",
            name: "默认主分机",
          },
        ]);
      }
    }
  }, [sip]);

  const mutation = useMutation({
    mutationFn: (data: { extensions: SipExtensionConfig[]; ring_timeout: number; username: string; password: string }) =>
      api.put<SipSettings>("/api/settings/sip", data),
    onSuccess: (updated) => {
      toast.ok("SIP 多分机与设备绑定配置已保存并生效");
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

  // 添加分机
  const handleAddExtension = () => {
    const existingNums = extensions.map((e) => parseInt(e.username, 10)).filter((n) => !isNaN(n));
    const nextNum = existingNums.length > 0 ? Math.max(...existingNums) + 1 : 1001;
    const randomPass = Math.random().toString().slice(2, 10);
    setExtensions([
      ...extensions,
      {
        id: `ext_${Date.now()}`,
        username: nextNum.toString(),
        password: randomPass,
        device_id: "",
        name: `分机 ${nextNum}`,
      },
    ]);
  };

  // 一键为所有在线设备自动分配分机
  const handleAutoAssign = () => {
    if (devices.length === 0) {
      toast.error("当前未检测到在线 4G 模组设备");
      return;
    }
    const newExts: SipExtensionConfig[] = devices.map((d, idx) => {
      const extNum = (1001 + idx).toString();
      const op = d.modem.operator ? ` (${d.modem.operator})` : "";
      return {
        id: `ext_${d.id}`,
        username: extNum,
        password: extensions[idx]?.password || "12345678",
        device_id: d.id,
        name: `${d.name || "设备"}${op}`,
      };
    });
    setExtensions(newExts);
    toast.ok(`已为 ${devices.length} 台设备一键规划分机 (1001 ~ ${1000 + devices.length})，点击“保存配置”即可生效！`);
  };

  const handleUpdateExt = (idx: number, field: keyof SipExtensionConfig, val: string) => {
    setExtensions((prev) => {
      const next = [...prev];
      next[idx] = { ...next[idx], [field]: val };
      return next;
    });
  };

  const handleRemoveExt = (idx: number) => {
    if (extensions.length <= 1) {
      toast.error("至少需要保留一个分机账号");
      return;
    }
    setExtensions((prev) => prev.filter((_, i) => i !== idx));
  };

  const toggleShowPass = (idx: number) => {
    setShowPassMap((prev) => ({ ...prev, [idx]: !prev[idx] }));
  };

  // 辅助：获取设备显示标签
  const getDeviceLabel = (devId: string) => {
    if (!devId) return "全部设备 (自动选择 / 来电全响)";
    const d = devices.find((x) => x.id === devId);
    if (!d) return `设备 ${devId}`;
    const op = d.modem.operator ? ` [${d.modem.operator}]` : "";
    const tail = d.identity.iccid ? `..${d.identity.iccid.slice(-6)}` : d.id.slice(-6);
    return `${d.name || d.id}${op} (${tail})`;
  };

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
              {clients.map((c, i) => {
                const boundExt = extensions.find((e) => e.username === c.username);
                const boundDev = boundExt ? getDeviceLabel(boundExt.device_id) : "未指定";
                return (
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
                          <Badge tone="info" className="text-xs">
                            出口绑定: {boundDev}
                          </Badge>
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
                );
              })}
            </div>
          ) : (
            <div className="rounded-xl bg-panel-2 p-5 text-center ring-1 ring-line">
              <Radio className="mx-auto size-8 text-subtle" />
              <p className="mt-2 text-sm font-medium">尚未连接软电话客户端</p>
              <p className="mt-1 text-xs text-muted max-w-md mx-auto">
                请在手机（如 Zoiper / Groundwire）或电脑上配置分机账号，连接成功后即可在此实时查看设备状态并免提接听 4G 来电。
              </p>
            </div>
          )}
        </CardBody>
      </Card>

      {/* 2. 多分机管理与设备绑定卡片 (方案 B: 多分机独立绑定) */}
      <Card>
        <CardHeader
          icon={KeyRound}
          title="多分机账号与 4G 设备绑定管理"
          subtitle="为每个 SIM 卡模组分配独立 SIP 分机，软电话拨号走绑定设备呼出，来电精准响铃对应分机"
          action={
            <div className="flex items-center gap-2">
              {devices.length > 1 && (
                <Button size="sm" variant="outline" icon={Wand2} onClick={handleAutoAssign}>
                  一键为所有设备分配分机
                </Button>
              )}
              <Button size="sm" variant="outline" icon={Plus} onClick={handleAddExtension}>
                添加分机
              </Button>
            </div>
          }
        />
        <CardBody className="space-y-4">
          <div className="space-y-3">
            {extensions.map((ext, idx) => {
              const isOnline = clients.some((c) => c.username === ext.username);
              const showPass = !!showPassMap[idx];
              const paramText = `服务器: ${currentHost}:${sipPort}\n分机号: ${ext.username}\n密码: ${ext.password}\n传输协议: UDP\nSIP URI: sip:${ext.username}@${currentHost}:${sipPort}`;

              return (
                <div
                  key={ext.id || idx}
                  className="rounded-xl border bg-panel-2/60 p-4 transition-all hover:border-line-strong"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2 pb-3 border-b border-line">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-sm">分机 #{idx + 1}</span>
                      {isOnline ? (
                        <Badge tone="ok" className="text-xs">
                          ● 在线
                        </Badge>
                      ) : (
                        <Badge tone="muted" className="text-xs">
                          离线
                        </Badge>
                      )}
                      <span className="text-xs text-muted">
                        (SIP: <code className="font-mono text-fg">{ext.username}</code>)
                      </span>
                    </div>

                    <div className="flex items-center gap-1.5">
                      <CopyButton text={paramText} label="复制配置参数" size="sm" />
                      {extensions.length > 1 && (
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={Trash2}
                          onClick={() => handleRemoveExt(idx)}
                          className="text-danger hover:text-danger hover:bg-danger/10"
                        >
                          删除
                        </Button>
                      )}
                    </div>
                  </div>

                  <div className="grid gap-3 pt-3 sm:grid-cols-2 lg:grid-cols-4">
                    {/* 分机号 */}
                    <div>
                      <label className="text-xs text-muted block mb-1">分机用户名 (User)</label>
                      <Input
                        value={ext.username}
                        onChange={(e) => handleUpdateExt(idx, "username", e.target.value.trim())}
                        placeholder="例如 1001"
                        className="font-mono"
                      />
                    </div>

                    {/* 分机密码 */}
                    <div>
                      <div className="flex items-center justify-between mb-1">
                        <label className="text-xs text-muted">分机密码 (Password)</label>
                        <button
                          type="button"
                          onClick={() => toggleShowPass(idx)}
                          className="text-[11px] text-muted hover:text-fg"
                        >
                          {showPass ? "隐藏" : "显示"}
                        </button>
                      </div>
                      <Input
                        type={showPass ? "text" : "password"}
                        value={ext.password}
                        onChange={(e) => handleUpdateExt(idx, "password", e.target.value.trim())}
                        placeholder="分机密码"
                        className="font-mono"
                      />
                    </div>

                    {/* 绑定出口设备 */}
                    <div>
                      <label className="text-xs text-muted block mb-1">绑定的 4G 模组 / SIM 卡</label>
                      <Select
                        value={ext.device_id}
                        onChange={(e) => handleUpdateExt(idx, "device_id", e.target.value)}
                        className="w-full text-xs"
                      >
                        <option value="">全部设备 (默认 / 未指定)</option>
                        {devices.map((d) => {
                          const op = d.modem.operator ? ` [${d.modem.operator}]` : "";
                          const tail = d.identity.iccid ? `..${d.identity.iccid.slice(-6)}` : d.id.slice(-6);
                          return (
                            <option key={d.id} value={d.id}>
                              {d.name || d.id}
                              {op} ({tail})
                            </option>
                          );
                        })}
                      </Select>
                    </div>

                    {/* 备注名称 */}
                    <div>
                      <label className="text-xs text-muted block mb-1">备注说明</label>
                      <Input
                        value={ext.name || ""}
                        onChange={(e) => handleUpdateExt(idx, "name", e.target.value)}
                        placeholder="如: 主力卡、香港 eSIM"
                      />
                    </div>
                  </div>
                </div>
              );
            })}
          </div>

          <div className="pt-2 border-t border-line flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
            <Field
              label="分机振铃超时（秒）"
              hint="收到外线来电时，对应分机响铃超过此秒数未接听，系统将自动切入 AI 接听助手"
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

            <Button
              variant="primary"
              icon={Save}
              loading={mutation.isPending}
              onClick={() => {
                const cleaned = extensions.map((e) => ({
                  ...e,
                  username: e.username.trim(),
                  password: e.password.trim(),
                }));
                if (cleaned.some((e) => !e.username || !e.password)) {
                  toast.error("所有分机的账号和密码均不能为空");
                  return;
                }
                mutation.mutate({
                  extensions: cleaned,
                  ring_timeout: Number(ringTimeout),
                  username: cleaned[0].username,
                  password: cleaned[0].password,
                });
              }}
            >
              保存分机设置
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* 3. 快速接入参数卡片 */}
      <Card>
        <CardHeader
          icon={Radio}
          title="SIP 网关通用连接参数"
          subtitle="在 Zoiper、MicroSIP 等客户端配置时所通用的服务器地址与音频协议"
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

            {/* 多分机说明 */}
            <div className="flex items-center justify-between rounded-xl bg-panel-2 p-3.5 ring-1 ring-line">
              <div className="min-w-0 pr-2">
                <span className="text-xs text-muted flex items-center gap-1.5">
                  <Shield className="size-3.5" /> 多分机路由隔离 (方案 B)
                </span>
                <p className="text-xs font-medium text-fg mt-0.5">
                  已启用多分机隔离：每张卡独占分机号，呼出呼入自动精准路由
                </p>
              </div>
              <Badge tone="ok">已就绪</Badge>
            </div>
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

            {/* 1. 新建账号 */}
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
                <span>➔ 弹出的窗口选择</span>
                <code className="text-ok font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Yes</code>
                <span>➔ 选择</span>
                <code className="text-fg font-semibold bg-panel-2 px-1.5 py-0.5 rounded">Manual configuration</code>
                <span>➔ 选择</span>
                <code className="text-accent font-semibold bg-panel-2 px-1.5 py-0.5 rounded">SIP account</code>
              </div>
            </div>

            {/* 2. 基础连接参数 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">2</span>
                <span>填入基本连接参数：</span>
              </p>
              <div className="ml-7 divide-y divide-line rounded-xl bg-panel-solid ring-1 ring-line text-xs overflow-hidden">
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0 font-medium">Account name</span>
                  <span className="text-fg flex-1 font-mono">账号别名，随便填，方便自己识别（如 SIM1-1001）</span>
                </div>
                <div className="flex items-center justify-between p-2.5 bg-accent/5">
                  <span className="text-muted w-44 shrink-0 font-medium text-fg">Domain</span>
                  <span className="text-accent font-mono font-semibold flex-1">{currentHost}:{sipPort}</span>
                  <CopyButton text={`${currentHost}:${sipPort}`} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0 font-medium text-fg">User name</span>
                  <span className="text-fg flex-1 font-mono">填写 分机用户名 (User)（如上方列表中的分机号）</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0 font-medium text-fg">Password</span>
                  <span className="text-fg flex-1 font-mono">填写 分机密码 (Password)</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0">Caller ID</span>
                  <span className="text-fg flex-1 font-mono">与 User name 保持一致</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0">Auth Username</span>
                  <span className="text-subtle flex-1 font-mono">留空，默认会跟随上面的 User name</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0">Use Outbound Proxy</span>
                  <span className="text-muted flex-1 font-mono">保持关闭（灰色关闭状态）</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0">Outbound Proxy</span>
                  <span className="text-subtle flex-1 font-mono">留空无需填写</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-44 shrink-0">Enable video FMTP</span>
                  <span className="text-muted flex-1 font-mono">保持默认即可</span>
                </div>
              </div>
            </div>

            {/* 3. 音频信令配置 (Features) */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">3</span>
                <span>进入 Features (特性功能配置)：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-1.5">
                <div className="flex items-start gap-2">
                  <CheckCircle className="size-4 text-accent shrink-0 mt-0.5" />
                  <div>
                    <span className="text-fg font-medium">DTMF 传信方式：</span>
                    <span className="text-muted ml-1">
                      选中 <code className="text-ok font-semibold bg-panel-2 px-1 rounded">inband</code>（带内音频），或保持 <code className="text-fg bg-panel-2 px-1 rounded">rfc-2833</code>。
                    </span>
                    <div className="mt-1 text-[11px] text-accent font-medium">
                      ★ 强烈推荐切换为 inband（按键 IVR 语音菜单兼容性最佳）。
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* 4. 网络设置 (Network Settings) */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">4</span>
                <span>进入 Network Settings (网络设置)：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-3">
                <div className="space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-fg">Enable IPv6</span>
                    <Badge tone="muted">建议关闭（灰色）</Badge>
                  </div>
                  <p className="text-[11px] text-muted leading-normal">
                    <strong>原因：</strong>避免手机优先尝试走公网 IPv6 解析而忽略了局域网的 IPv4。
                  </p>
                </div>

                <div className="space-y-1 pt-2 border-t border-line/60">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-fg">Transports (传输协议)</span>
                    <Badge tone="info">UDP</Badge>
                  </div>
                </div>

                <div className="space-y-1 pt-2 border-t border-line/60">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-fg">Enable STUN</span>
                    <Badge tone="muted">关闭（灰色）</Badge>
                  </div>
                  <p className="text-[11px] text-muted leading-normal">
                    <strong>原因：</strong>当前开启时，手机会连到公网的 stun.zoiper.com 获取外网 IP。但在家庭局域网内，开启 STUN 会导致手机把自己的内网 IP 换成公网 IP 发送，造成注册失败或接听没声音。局域网务必关闭 STUN。
                  </p>
                </div>

                <div className="space-y-1 pt-2 border-t border-line/60">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-fg">Enable on start</span>
                    <Badge tone="ok">保持绿色开启</Badge>
                  </div>
                  <p className="text-[11px] text-muted leading-normal">
                    保证打开 App 时自动连接并注册分机。
                  </p>
                </div>

                <div className="space-y-1 pt-2 border-t border-line/60">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-fg">TLS Settings</span>
                    <Badge tone="muted">关闭（灰色）</Badge>
                  </div>
                  <p className="text-[11px] text-muted leading-normal">
                    <strong>原因：</strong>传输通道选的是 UDP，TLS 引擎无需启动。
                  </p>
                </div>

                <div className="rounded-lg bg-panel-2 p-2.5 text-xs text-muted flex items-center justify-between">
                  <span>设置完成后，点击右上角黑色的 <strong>【Save】</strong> 保存，并点击左上角 <strong>&lt; Accounts</strong> 返回上一页。</span>
                </div>
              </div>
            </div>

            {/* 5. 音频编码 (Audio Codecs) */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">5</span>
                <span>进入 Audio Codecs (音频编码器优化)：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center justify-between flex-wrap gap-2">
                  <span className="font-semibold text-fg">
                    仅保留开启：<code className="text-ok font-mono bg-panel-2 px-1.5 py-0.5 rounded">u-law (PCMU)</code> 与 <code className="text-ok font-mono bg-panel-2 px-1.5 py-0.5 rounded">a-law (PCMA)</code>
                  </span>
                  <Badge tone="warn">按住右侧三条杠拖到最上方，关闭其他所有编码</Badge>
                </div>
                <div className="space-y-1.5 pt-2 border-t border-line/60 text-[11px] text-muted leading-relaxed">
                  <p>
                    <strong>• 硬件原生匹配：</strong>移远 EC20 4G 模组与中国移动/联通/电信标准语音固网采用的正是 G.711 u-law (8000Hz) 与 G.711 a-law，SimCallHub 原生支持这两个编码。
                  </p>
                  <p>
                    <strong>• 避免无效协商：</strong>GSM、Speex、iLBC 是上古时期的有损网络压缩编码，网关并不支持。如果开启且排在前面，每次打电话双方都要先协商失败再退回 G.711，容易导致接通慢甚至偶尔无声。只留这两个能实现秒级直通。
                  </p>
                </div>
              </div>
            </div>

            {/* 6. 视频编解码 (Video codecs) */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">6</span>
                <span>进入 Video codecs (关闭视频编码器)：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-fg">把 VP8 也点击关闭</span>
                  <Badge tone="muted">切换为灰色关闭状态</Badge>
                </div>
                <p className="text-[11px] text-muted leading-relaxed pt-1 border-t border-line/60">
                  <strong>原因：</strong>SimCallHub 是连接移远 4G 模块的蜂窝电话网关，运营商手机网络通话是纯音频（G.711 / PCM），完全没有视频数据流。如果开启了 VP8，Zoiper 每次拨打或接听电话时，都会在 SIP 信令（SDP 报文）中声明“我支持视频通话（m=video）”。这不仅增加了网络开销，甚至可能导致某些纯语音 SIP 引擎在解析媒体描述时出现兼容性问题。彻底关闭视频编码器后，Zoiper 会以纯语音客户端模式运行，不会唤醒 iOS 系统的摄像头管线或硬件视频编解码模块，呼叫建立更快、更省电。
                </p>
              </div>
            </div>

            {/* 7. 即时消息 (Instant Messaging) */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">7</span>
                <span>进入 Instant Messaging (即时消息设置)：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-fg">IM Enabled (即时消息)</span>
                  <Badge tone="muted">全部关闭（关掉 IM Enabled）</Badge>
                </div>
                <p className="text-[11px] text-muted leading-relaxed pt-1 border-t border-line/60">
                  <strong>原因：</strong>IM 是用于类似企业聊天软件（如 Skype for Business）在 SIP 里收发文本聊天消息的；Presence 是探测对方是否“在线/离开/忙碌”。SimCallHub 是电话网关，你的短信收发在 Web 后台已有独立的分类系统，不需要通过 Zoiper 传文本。
                </p>
              </div>
            </div>

            {/* 8. 注册与连接验证 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-ok/20 text-ok text-xs font-semibold">8</span>
                <span>回到设置主界面进行注册：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <p className="font-medium text-fg">
                  最后回到账号设置主界面，点击大绿色按钮 <strong>【Register】</strong>。
                </p>
                <div className="flex items-center gap-2 text-ok font-medium">
                  <CheckCircle2 className="size-4" />
                  <span>注册成功后，顶部会显示绿色的 <strong>Registration status: OK</strong>！</span>
                </div>
                <p className="text-muted text-[11px]">
                  此时您已可在本页面顶部的“分机实时在线状态”中看到您的手机设备已在线，任何打入对应 SIM 卡的电话均可免提接听，拨号呼出也会走绑定的 4G 卡。
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
                <strong>Username / User</strong> 填入对应的分机号，密码填入对应分机的密码；
              </li>
              <li>保存后即可随时在电脑上免提接听/拨打手机号码。</li>
            </ol>
          </div>

          <Alert tone="ok" title="多分机独立使用小贴士">
            在手机 Zoiper 或电脑客户端中，您可以同时添加多个账号（例如添加一个账号 1001 绑定卡 1，再添加一个账号 1002 绑定 eSIM）。拨打电话时，在拨号盘直接切换想要使用的账号，即可自动使用对应的 SIM 卡呼出，完全独立互不影响！
          </Alert>
        </div>
      </Collapsible>
    </div>
  );
}
