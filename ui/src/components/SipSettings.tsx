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
                <span>填入对应分机信息（多分机模式下可添加多个 Account 分别对应不同 SIM 卡）：</span>
              </p>
              <div className="ml-7 divide-y divide-line rounded-xl bg-panel-solid ring-1 ring-line text-xs overflow-hidden">
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Account name</span>
                  <span className="text-fg flex-1 font-mono">自定义（如 SIM1-1001、eSIM-1002）</span>
                  <span className="text-subtle text-[11px]">方便识别是哪张卡</span>
                </div>
                <div className="flex items-center justify-between p-2.5 bg-accent/5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">Domain</span>
                  <span className="text-accent font-mono font-semibold flex-1">{currentHost}:{sipPort}</span>
                  <CopyButton text={`${currentHost}:${sipPort}`} label="复制" size="sm" />
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">User name</span>
                  <span className="text-fg flex-1 font-mono">上方列表对应的分机号（如 1001）</span>
                  <span className="text-subtle text-[11px]">分机账号</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0 font-medium text-fg">Password</span>
                  <span className="text-fg flex-1 font-mono">上方列表对应的分机密码</span>
                  <span className="text-subtle text-[11px]">分机密码</span>
                </div>
                <div className="flex items-center justify-between p-2.5">
                  <span className="text-muted w-36 shrink-0">Caller ID</span>
                  <span className="text-fg flex-1 font-mono">与 User name 保持一致</span>
                  <span className="text-subtle text-[11px]">分机号</span>
                </div>
              </div>
            </div>

            {/* Step 3 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">3</span>
                <span>核心网络与保活设置（必填，防止后台断连）：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-2">
                <div className="flex items-start gap-2">
                  <CheckCircle className="size-4 text-ok shrink-0 mt-0.5" />
                  <div>
                    <strong className="text-fg">Auth. username (认证用户名)：</strong>
                    <span className="text-muted">必须与 User name 完全一致（如 1001），不可留空。</span>
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <CheckCircle className="size-4 text-ok shrink-0 mt-0.5" />
                  <div>
                    <strong className="text-fg">Network Settings ➔ Transport type：</strong>
                    <span className="text-muted">选择 <strong>UDP</strong>。</span>
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <CheckCircle className="size-4 text-ok shrink-0 mt-0.5" />
                  <div>
                    <strong className="text-fg">Registration expiry：</strong>
                    <span className="text-muted">推荐填写 <strong>300</strong> 秒（5分钟保活一次，稳定性最佳）。</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Step 4 */}
            <div className="space-y-2">
              <p className="font-medium text-fg flex items-center gap-2 text-[13px]">
                <span className="flex size-5 items-center justify-center rounded-full bg-accent/20 text-accent text-xs font-semibold">4</span>
                <span>启用与注册：</span>
              </p>
              <div className="ml-7 rounded-xl bg-panel-solid p-3.5 ring-1 ring-line text-xs space-y-1.5">
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
