import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Cpu, CheckCircle2, CircleOff, RefreshCw, Pencil, Trash2, AlertCircle } from "lucide-react";
import { api, type DeviceStatus } from "../lib/api";
import { Button, Card, CardBody, CardHeader, Badge, Spinner, ConfirmModal, Input, useToast } from "./ui";

export interface ESimProfile {
  iccid: string;
  name: string;
  provider: string;
  nickname: string;
  state: "enabled" | "disabled";
  class: string;
}

export interface ESimInfo {
  supported: boolean;
  eid?: string;
  profiles?: ESimProfile[];
  error?: string;
}

export function ESimCard({ d }: { d: DeviceStatus }) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [targetICCID, setTargetICCID] = useState<string | null>(null);
  const [confirmAction, setConfirmAction] = useState<"enable" | "disable" | "delete" | null>(null);
  const [renameICCID, setRenameICCID] = useState<string | null>(null);
  const [newNick, setNewNick] = useState("");

  const { data: esim, isLoading, refetch, isFetching } = useQuery<ESimInfo>({
    queryKey: ["esim", d.id],
    queryFn: () => api.get<ESimInfo>(`/api/devices/${d.id}/esim`),
    enabled: d.online,
  });

  const mutation = useMutation({
    mutationFn: async ({ action, iccid, nickname }: { action: string; iccid: string; nickname?: string }) => {
      if (action === "nickname") {
        return api.post(`/api/devices/${d.id}/esim/nickname`, { iccid, nickname });
      }
      return api.post(`/api/devices/${d.id}/esim/${action}`, { iccid });
    },
    onSuccess: (_, vars) => {
      toast.success(vars.action === "enable" ? "已发送启用并切换指令，模组正在重新搜网..." : "操作成功");
      queryClient.invalidateQueries({ queryKey: ["esim", d.id] });
      setConfirmAction(null);
      setRenameICCID(null);
    },
    onError: (err: Error) => {
      toast.error(`操作失败: ${err.message}`);
    },
  });

  if (isLoading) {
    return (
      <Card>
        <CardBody className="flex items-center justify-center py-12">
          <Spinner />
          <span className="ml-3 text-sm text-muted">正在读取 eSIM / eUICC 信息...</span>
        </CardBody>
      </Card>
    );
  }

  if (!esim?.supported) {
    return (
      <Card>
        <CardHeader
          title="eSIM 管理 (eUICC)"
          subtitle="通过 SGP.22 协议直接管理模组上的 eSIM 配置文件"
          icon={Cpu}
          action={
            <Button size="sm" variant="outline" onClick={() => refetch()} loading={isFetching}>
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              重新检测
            </Button>
          }
        />
        <CardBody className="py-8 text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-muted/40">
            <Cpu className="h-6 w-6 text-muted" />
          </div>
          <h3 className="mt-3 text-sm font-medium">当前为普通物理 SIM 卡</h3>
          <p className="mt-1 text-xs text-muted max-w-md mx-auto">
            未检测到 eUICC / eSIM 芯片响应。如果你的卡板焊接了 eSIM 芯片或插有 5ber/estk 等可写卡，请确认模组与卡槽接触良好。
          </p>
        </CardBody>
      </Card>
    );
  }

  const profiles = esim.profiles || [];

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="eSIM 芯片信息"
          subtitle="eUICC 硬件就绪"
          icon={Cpu}
          action={
            <Button size="sm" variant="outline" onClick={() => refetch()} loading={isFetching}>
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              刷新
            </Button>
          }
        />
        <CardBody>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="text-xs text-muted">EID (eUICC Identifier)</div>
              <div className="mt-1 font-mono text-sm font-medium tracking-tight break-all">
                {esim.eid || "未读取到 EID"}
              </div>
            </div>
            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="text-xs text-muted">已安装 Profile 数量</div>
              <div className="mt-1 text-sm font-medium">
                {profiles.length} 个配置文件
              </div>
            </div>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="eSIM 配置文件列表"
          subtitle="点击即可一键切换当前激活的运营商卡配置文件"
          icon={Cpu}
        />
        <CardBody className="divide-y p-0">
          {profiles.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted">
              芯片内暂无已下载的 eSIM 配置文件
            </div>
          ) : (
            profiles.map((p) => {
              const isEnabled = p.state === "enabled";
              return (
                <div key={p.iccid} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm">
                        {p.nickname || p.name || p.provider || "未命名配置"}
                      </span>
                      {isEnabled ? (
                        <Badge tone="success" className="flex items-center gap-1">
                          <CheckCircle2 className="h-3 w-3" /> 当前启用
                        </Badge>
                      ) : (
                        <Badge tone="neutral" className="flex items-center gap-1">
                          <CircleOff className="h-3 w-3" /> 未激活
                        </Badge>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted font-mono">
                      <span>ICCID: {p.iccid}</span>
                      {p.provider && <span>运营商: {p.provider}</span>}
                      {p.class && <span>类型: {p.class}</span>}
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    {!isEnabled ? (
                      <Button
                        size="sm"
                        variant="primary"
                        onClick={() => {
                          setTargetICCID(p.iccid);
                          setConfirmAction("enable");
                        }}
                      >
                        启用并切换
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setTargetICCID(p.iccid);
                          setConfirmAction("disable");
                        }}
                      >
                        停用
                      </Button>
                    )}

                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setRenameICCID(p.iccid);
                        setNewNick(p.nickname || p.name || "");
                      }}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>

                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-500 hover:text-red-600"
                      onClick={() => {
                        setTargetICCID(p.iccid);
                        setConfirmAction("delete");
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              );
            })
          )}
        </CardBody>
      </Card>

      {/* 确认操作弹窗 */}
      <ConfirmModal
        open={confirmAction !== null}
        onClose={() => setConfirmAction(null)}
        onConfirm={() => {
          if (confirmAction && targetICCID) {
            mutation.mutate({ action: confirmAction, iccid: targetICCID });
          }
        }}
        title={
          confirmAction === "enable"
            ? "确认切换到此 eSIM 配置？"
            : confirmAction === "disable"
            ? "确认停用此 eSIM 配置？"
            : "确认永久删除此配置？"
        }
        loading={mutation.isPending}
        danger={confirmAction === "delete"}
      >
        <p className="text-sm text-muted">
          {confirmAction === "enable" && "切换后模组将软重启并使用新的卡号重新搜网注册网络。"}
          {confirmAction === "disable" && "停用后该卡槽将处于无 SIM 卡状态。"}
          {confirmAction === "delete" && "删除后此 eSIM 配置文件无法恢复，需重新向运营商申请二维码。"}
        </p>
      </ConfirmModal>

      {/* 重命名弹窗 */}
      {renameICCID && (
        <ConfirmModal
          open={true}
          onClose={() => setRenameICCID(null)}
          onConfirm={() => {
            mutation.mutate({ action: "nickname", iccid: renameICCID, nickname: newNick });
          }}
          title="修改配置文件备注名称"
          loading={mutation.isPending}
        >
          <div className="space-y-2">
            <label className="text-xs text-muted">新备注名称</label>
            <Input value={newNick} onChange={(e) => setNewNick(e.target.value)} placeholder="如: 备用联通卡" />
          </div>
        </ConfirmModal>
      )}
    </div>
  );
}
