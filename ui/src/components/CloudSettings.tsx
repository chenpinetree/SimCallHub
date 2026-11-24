import clsx from "clsx";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BellRing,
  CheckCircle2,
  Cloud,
  CloudOff,
  KeyRound,
  Link2,
  Send,
  ShieldCheck,
  Smartphone,
  Unlink,
  Wifi,
} from "lucide-react";
import { api, type CloudMode, type CloudSettings } from "../lib/api";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  Collapsible,
  ConfirmModal,
  CopyButton,
  Field,
  Input,
  Spinner,
  Switch,
  useToast,
} from "./ui";

function tunnelBadge(cloud: CloudSettings) {
  if (cloud.mode === "direct") {
    return <Badge tone="muted">免费直连（离线零外部连接）</Badge>;
  }
  switch (cloud.status.state) {
    case "connected":
      return <Badge tone="ok">云中继已连接</Badge>;
    case "connecting":
      return <Badge tone="warn">正在连接云端…</Badge>;
    case "unpaired":
      return <Badge tone="warn">待配对绑定</Badge>;
    case "error":
      return <Badge tone="bad">连接异常</Badge>;
    default:
      return <Badge>未启动</Badge>;
  }
}

export function CloudSection() {
  const qc = useQueryClient();
  const toast = useToast();

  const { data: cloud, isLoading } = useQuery<CloudSettings>({
    queryKey: ["cloud-settings"],
    queryFn: () => api.get<CloudSettings>("/api/settings/cloud"),
    refetchInterval: 4000,
  });

  const [pairCode, setPairCode] = useState("");
  const [deviceName, setDeviceName] = useState("SimCallHub");
  const [apiBase, setApiBase] = useState("https://api.sim-callhub.com");
  const [relayBase, setRelayBase] = useState("wss://relay.sim-callhub.com");
  const [confirmUnbind, setConfirmUnbind] = useState(false);

  useEffect(() => {
    if (cloud) {
      setDeviceName(cloud.deviceName || "SimCallHub");
      setApiBase(cloud.apiBase || "https://api.sim-callhub.com");
      setRelayBase(cloud.relayBase || "wss://relay.sim-callhub.com");
    }
  }, [cloud]);

  const updateMutation = useMutation({
    mutationFn: (payload: {
      mode?: CloudMode;
      apiBase?: string;
      relayBase?: string;
      deviceName?: string;
      pushEnabled?: boolean;
    }) => api.put<CloudSettings>("/api/settings/cloud", payload),
    onSuccess: (updated, vars) => {
      qc.setQueryData(["cloud-settings"], updated);
      if (vars.mode === "direct") {
        toast.ok("已切换为免费直连模式，本地已断开所有云端连接");
      } else if (vars.mode === "cloud") {
        toast.ok("已切换为付费云中继模式");
      } else {
        toast.ok("云接入配置已更新");
      }
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const pairMutation = useMutation({
    mutationFn: () =>
      api.post<CloudSettings>("/api/settings/cloud/pair", {
        code: pairCode.trim(),
        name: deviceName.trim() || "SimCallHub",
        apiBase: apiBase.trim(),
        relayBase: relayBase.trim(),
      }),
    onSuccess: (updated) => {
      qc.setQueryData(["cloud-settings"], updated);
      setPairCode("");
      toast.ok("云端配对成功！反向隧道已自动启动");
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const unbindMutation = useMutation({
    mutationFn: () => api.post<CloudSettings>("/api/settings/cloud/unbind", {}),
    onSuccess: (updated) => {
      qc.setQueryData(["cloud-settings"], updated);
      setConfirmUnbind(false);
      toast.ok("已解绑云端设备并切换回免费直连模式");
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const testPushMutation = useMutation({
    mutationFn: () => api.post<{ ok: boolean }>("/api/settings/cloud/test-push", {}),
    onSuccess: () => toast.ok("已向云端发送测试推送，请查看 iPhone 通知栏"),
    onError: (e: Error) => toast.error(e.message),
  });

  if (isLoading || !cloud) {
    return (
      <Card className="flex h-40 items-center justify-center">
        <Spinner />
      </Card>
    );
  }

  const originURL = typeof window !== "undefined" ? window.location.origin : "https://your-nas:9081";

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          icon={Smartphone}
          title="接入模式选择（免费直连 / 付费云中继）"
          subtitle="两种模式共用同一个镜像，可随时一键无损切换"
          action={tunnelBadge(cloud)}
        />
        <CardBody className="space-y-4">
          <div className="grid gap-3 md:grid-cols-2">
            <button
              type="button"
              disabled={updateMutation.isPending}
              onClick={() => updateMutation.mutate({ mode: "direct" })}
              className={clsx(
                "flex flex-col gap-2 rounded-2xl p-4 text-left ring-1 transition",
                cloud.mode === "direct"
                  ? "bg-accent/8 ring-accent"
                  : "bg-panel-2/60 ring-line hover:ring-line-strong",
              )}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="flex items-center gap-2 text-sm font-semibold">
                  <ShieldCheck className={clsx("size-4", cloud.mode === "direct" ? "text-accent" : "text-muted")} />
                  免费直连模式（Direct）
                </span>
                {cloud.mode === "direct" && <Badge tone="ok">当前启用</Badge>}
              </div>
              <p className="text-xs leading-relaxed text-muted">
                完全离线私有化运行，<strong>绝不向任何云端服务器发起连接</strong>。适合具备公网域名、端口映射或 Tailscale 组网的用户，搭配 Bearer Token 直连 iOS 客户端。
              </p>
            </button>

            <button
              type="button"
              disabled={updateMutation.isPending}
              onClick={() => updateMutation.mutate({ mode: "cloud" })}
              className={clsx(
                "flex flex-col gap-2 rounded-2xl p-4 text-left ring-1 transition",
                cloud.mode === "cloud"
                  ? "bg-accent/8 ring-accent"
                  : "bg-panel-2/60 ring-line hover:ring-line-strong",
              )}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="flex items-center gap-2 text-sm font-semibold">
                  <Cloud className={clsx("size-4", cloud.mode === "cloud" ? "text-accent" : "text-muted")} />
                  付费云中继模式（SimCallCloud）
                </span>
                {cloud.mode === "cloud" && <Badge tone="ok">当前启用</Badge>}
              </div>
              <p className="text-xs leading-relaxed text-muted">
                无需公网 IP 与路由器端口映射。通过加密 <code className="font-mono text-fg">yamux</code> 反向隧道穿透家庭 NAT，支持外网双向实时通话与苹果原生 APNs / CallKit 来电唤醒。
              </p>
            </button>
          </div>

          {cloud.mode === "direct" && (
            <div className="space-y-3 rounded-2xl bg-panel-2/70 p-4 ring-1 ring-line">
              <div className="flex items-center gap-2 text-xs font-medium text-fg">
                <Wifi className="size-4 text-ok" />
                直连地址与 API 鉴权已就绪
              </div>
              <p className="text-xs leading-relaxed text-muted">
                当前所有外部云连接均已关闭。在 iOS 客户端选择「自建直连」后，填写你的服务地址与管理员密码即可登录使用：
              </p>
              <div className="flex items-center justify-between gap-2 rounded-xl bg-panel px-3 py-2 ring-1 ring-line">
                <code className="truncate font-mono text-xs text-fg">{originURL}</code>
                <CopyButton text={originURL} label="复制地址" />
              </div>
              <p className="text-[11px] text-subtle">
                提示：原生客户端通过 <code className="font-mono">POST /api/login</code> 获取会话 Token，并以 <code className="font-mono">Authorization: Bearer &lt;token&gt;</code> 访问短信、控制与语音 WebSocket。
              </p>
            </div>
          )}

          {cloud.mode === "cloud" && cloud.status.state === "error" && cloud.status.lastError && (
            <Alert tone="bad" title="云端中继连接异常">
              {cloud.status.lastError}
            </Alert>
          )}

          {cloud.mode === "cloud" && cloud.status.state === "connected" && (
            <Alert tone="ok" title="反向加密隧道在线">
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                <span>设备 ID：<code className="font-mono">{cloud.deviceId}</code></span>
                {cloud.status.connectedSince && (
                  <span>连接于：{new Date(cloud.status.connectedSince).toLocaleString()}</span>
                )}
              </div>
            </Alert>
          )}
        </CardBody>
      </Card>

      {cloud.mode === "cloud" && (
        <Card>
          <CardHeader
            icon={Link2}
            title={cloud.paired ? "云端设备绑定与推送设置" : "配对绑定 SimCallCloud"}
            subtitle={
              cloud.paired
                ? "设备凭证已使用本地 secret.key 加密持久化在 /data 目录"
                : "在 iOS 客户端登录后点击「添加设备」，将生成的 6 位配对码填到下方"
            }
          />
          <CardBody className="space-y-4">
            {!cloud.paired ? (
              <div className="space-y-4">
                <div className="grid gap-4 md:grid-cols-2">
                  <Field label="iOS App 配对码" hint="配对码 5 分钟内有效，使用一次后自动作废">
                    <Input
                      value={pairCode}
                      onChange={(e) => setPairCode(e.target.value)}
                      placeholder="例如 482910"
                      className="font-mono"
                    />
                  </Field>
                  <Field label="设备备注名称" hint="将显示在 iPhone 端设备列表中">
                    <Input
                      value={deviceName}
                      onChange={(e) => setDeviceName(e.target.value)}
                      placeholder="例如 家里群晖 NAS"
                    />
                  </Field>
                </div>
                <Button
                  variant="primary"
                  icon={KeyRound}
                  loading={pairMutation.isPending}
                  disabled={!pairCode.trim()}
                  onClick={() => pairMutation.mutate()}
                >
                  立即配对并连接云中继
                </Button>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="grid gap-3 rounded-2xl bg-panel-2/70 p-4 text-xs ring-1 ring-line md:grid-cols-3">
                  <div>
                    <div className="text-subtle">Cloud Device ID</div>
                    <div className="mt-1 flex items-center gap-1.5 font-mono text-fg">
                      <span className="truncate">{cloud.deviceId}</span>
                      <CopyButton text={cloud.deviceId} label="复制" />
                    </div>
                  </div>
                  <div>
                    <div className="text-subtle">设备备注名</div>
                    <div className="mt-1 font-medium text-fg">{cloud.deviceName}</div>
                  </div>
                  <div>
                    <div className="text-subtle">设备通讯密钥</div>
                    <div className="mt-1 font-mono text-fg">{cloud.hasSecret ? cloud.secretHint : "未保存"}</div>
                  </div>
                </div>

                <div className="flex items-center justify-between gap-3 rounded-2xl bg-panel-2 px-4 py-3 ring-1 ring-line">
                  <div className="flex items-start gap-3">
                    <BellRing className="mt-0.5 size-4 shrink-0 text-accent" />
                    <div>
                      <div className="text-sm font-medium">启用 iPhone 原生推送（APNs + CallKit 来电唤醒）</div>
                      <div className="text-xs text-muted">
                        新短信（遵循消息推送规则与脱敏设置）、未接来电、AI 纪要将发往 APNs；外线振铃时自动发送 VoIP Push 唤醒全屏来电界面。
                      </div>
                    </div>
                  </div>
                  <Switch
                    checked={cloud.pushEnabled}
                    label="启用 iPhone 原生推送"
                    onChange={(v) => updateMutation.mutate({ pushEnabled: v })}
                  />
                </div>

                <div className="flex flex-wrap items-center gap-2.5">
                  <Button
                    variant="secondary"
                    icon={Send}
                    loading={testPushMutation.isPending}
                    onClick={() => testPushMutation.mutate()}
                  >
                    发送测试推送到 iPhone
                  </Button>
                  <Button
                    variant="danger"
                    icon={Unlink}
                    onClick={() => setConfirmUnbind(true)}
                  >
                    解绑设备并切回直连
                  </Button>
                </div>
              </div>
            )}

            <Collapsible title="高级：云端服务器地址（默认官方 SimCallCloud，一般无需修改）">
              <div className="space-y-3 pt-2">
                <div className="grid gap-4 md:grid-cols-2">
                  <Field label="API 接口地址">
                    <Input
                      value={apiBase}
                      onChange={(e) => setApiBase(e.target.value)}
                      disabled={cloud.paired}
                      className="font-mono text-xs"
                    />
                  </Field>
                  <Field label="WebSocket 中继地址">
                    <Input
                      value={relayBase}
                      onChange={(e) => setRelayBase(e.target.value)}
                      disabled={cloud.paired}
                      className="font-mono text-xs"
                    />
                  </Field>
                </div>
                {cloud.paired ? (
                  <p className="flex items-center gap-1.5 text-[11px] text-subtle">
                    <CloudOff className="size-3.5" />
                    如需更改服务器地址，请先解绑当前设备凭证。
                  </p>
                ) : (
                  <Button
                    size="sm"
                    variant="secondary"
                    icon={CheckCircle2}
                    loading={updateMutation.isPending}
                    onClick={() =>
                      updateMutation.mutate({
                        apiBase: apiBase.trim(),
                        relayBase: relayBase.trim(),
                        deviceName: deviceName.trim(),
                      })
                    }
                  >
                    保存服务器地址
                  </Button>
                )}
              </div>
            </Collapsible>
          </CardBody>
        </Card>
      )}

      <ConfirmModal
        open={confirmUnbind}
        title="确定解绑云端设备？"
        description="解绑后将清除本地保存的设备密钥、断开与 SimCallCloud 的中继隧道，并切换回免费直连模式。你也可以在 iOS App 中释放该设备槽位。"
        confirmLabel="确定解绑"
        danger
        loading={unbindMutation.isPending}
        onConfirm={() => unbindMutation.mutate()}
        onClose={() => setConfirmUnbind(false)}
      />
    </div>
  );
}
