import clsx from "clsx";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Cpu,
  KeyRound,
  Pencil,
  Plus,
  Save,
  Shield,
  Trash2,
  UserCheck,
  UserPlus,
  UserX,
  Users,
  X,
} from "lucide-react";
import {
  api,
  type DeviceScope,
  type UserAccount,
  type UserPermissions,
  type UsersResponse,
} from "../lib/api";
import { defaultSubUserPermissions } from "../lib/auth";
import { knownDevices } from "../lib/devices";
import { useStatus } from "../lib/hooks";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  ConfirmModal,
  Empty,
  Field,
  Input,
  Segmented,
  Spinner,
  Switch,
  useToast,
} from "./ui";

const mainPermItems: { key: keyof UserPermissions; label: string; hint: string }[] = [
  { key: "messages", label: "短信", hint: "收发、搜索、标记和删除短信" },
  { key: "calls", label: "电话", hint: "拨打、接听、录音回放与通话记录" },
  { key: "contacts", label: "通讯录", hint: "查看与管理联系人及黄页号码" },
  { key: "device", label: "设备", hint: "模组状态、SIM/eSIM、重启与 AT 控制台" },
  { key: "logs", label: "日志", hint: "查看系统运行日志与告警事件" },
];

const settingsPermItems: { key: keyof UserPermissions; label: string; hint: string }[] = [
  { key: "settingsNotify", label: "消息推送", hint: "Telegram、飞书、钉钉、企业微信推送规则" },
  { key: "settingsAssistant", label: "AI 接听", hint: "AI 语音代接与语音留言设置" },
  { key: "settingsSip", label: "SIP 电话", hint: "VoIP 分机账号与连接状态管理" },
  { key: "settingsCloud", label: "iOS 与云中继", hint: "直连 / 云中继切换与 App 配对" },
  { key: "settingsClassifier", label: "智能分类", hint: "大模型配置、分类规则与调用记录" },
  { key: "settingsDevices", label: "设备（设置）", hint: "SIM 卡自动解锁 PIN 配置" },
  { key: "settingsGeneral", label: "通用", hint: "界面模式与浅色/深色主题设置" },
];

const allPermLabels: Record<keyof UserPermissions, string> = {
  messages: "短信",
  calls: "电话",
  contacts: "通讯录",
  device: "设备",
  logs: "日志",
  settingsNotify: "设置·消息推送",
  settingsAssistant: "设置·AI接听",
  settingsSip: "设置·SIP电话",
  settingsCloud: "设置·云中继",
  settingsClassifier: "设置·智能分类",
  settingsDevices: "设置·设备",
  settingsGeneral: "设置·通用",
};

interface SubFormState {
  editingUsername: string | null; // null = create mode
  username: string;
  password: string;
  note: string;
  disabled: boolean;
  deviceScope: DeviceScope;
  allowedDevices: string[];
  permissions: UserPermissions;
}

function emptySubForm(): SubFormState {
  return {
    editingUsername: null,
    username: "",
    password: "",
    note: "",
    disabled: false,
    deviceScope: "all",
    allowedDevices: [],
    permissions: { ...defaultSubUserPermissions },
  };
}

export function UserSettingsSection() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: status } = useStatus();
  const devices = knownDevices(status);

  const { data, isPending } = useQuery<UsersResponse>({
    queryKey: ["users"],
    queryFn: () => api.get<UsersResponse>("/api/users"),
  });

  const admin = data?.items.find((u) => u.role === "admin");
  const subUsers = (data?.items ?? []).filter((u) => u.role !== "admin");

  // Admin edit state
  const [adminName, setAdminName] = useState("");
  const [adminPassword, setAdminPassword] = useState("");
  const [adminConfirm, setAdminConfirm] = useState("");
  const [adminNote, setAdminNote] = useState("");

  useEffect(() => {
    if (admin) {
      setAdminName(admin.username);
      setAdminNote(admin.note || "");
    }
  }, [admin?.username, admin?.note]);

  // Sub-account create/edit drawer/card state
  const [form, setForm] = useState<SubFormState | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<UserAccount | null>(null);

  const refreshAll = () => {
    qc.invalidateQueries({ queryKey: ["users"] });
    qc.invalidateQueries({ queryKey: ["session"] });
  };

  const saveAdmin = useMutation({
    mutationFn: () => {
      if (!admin) throw new Error("未找到管理员账号");
      const payload: Record<string, unknown> = {
        username: adminName.trim(),
        note: adminNote.trim(),
      };
      if (adminPassword) payload.password = adminPassword;
      return api.put<UserAccount>(`/api/users/${encodeURIComponent(admin.username)}`, payload);
    },
    onSuccess: () => {
      toast.ok("管理员账号已更新");
      setAdminPassword("");
      setAdminConfirm("");
      refreshAll();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const saveSub = useMutation({
    mutationFn: (f: SubFormState) => {
      const payload: Record<string, unknown> = {
        username: f.username.trim(),
        note: f.note.trim(),
        disabled: f.disabled,
        deviceScope: f.deviceScope,
        allowedDevices: f.deviceScope === "selected" ? f.allowedDevices : [],
        permissions: f.permissions,
      };
      if (f.password) payload.password = f.password;
      if (f.editingUsername === null) {
        return api.post<UserAccount>("/api/users", payload);
      }
      return api.put<UserAccount>(`/api/users/${encodeURIComponent(f.editingUsername)}`, payload);
    },
    onSuccess: (_, f) => {
      toast.ok(f.editingUsername === null ? `已添加子账号：${f.username.trim()}` : `已更新子账号：${f.username.trim()}`);
      setForm(null);
      refreshAll();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const toggleSubDisabled = useMutation({
    mutationFn: (u: UserAccount) =>
      api.put<UserAccount>(`/api/users/${encodeURIComponent(u.username)}`, {
        disabled: !u.disabled,
      }),
    onSuccess: (updated) => {
      toast.ok(updated.disabled ? `已停用子账号 ${updated.username}` : `已启用子账号 ${updated.username}`);
      refreshAll();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const deleteSub = useMutation({
    mutationFn: (username: string) => api.del(`/api/users/${encodeURIComponent(username)}`),
    onSuccess: (_, username) => {
      toast.ok(`已删除子账号：${username}`);
      setDeleteTarget(null);
      if (form?.editingUsername === username) setForm(null);
      refreshAll();
    },
    onError: (e: Error) => toast.error(e.message),
  });

  if (isPending || !data) {
    return (
      <Card className="flex h-40 items-center justify-center">
        <Spinner />
      </Card>
    );
  }

  const adminDirty =
    !!admin &&
    (adminName.trim() !== admin.username ||
      adminNote.trim() !== (admin.note || "") ||
      adminPassword.length > 0);
  const adminPassMismatch = adminPassword.length > 0 && adminPassword !== adminConfirm;
  const adminPassTooShort = adminPassword.length > 0 && adminPassword.length < 4;

  const openEditSub = (u: UserAccount) => {
    setForm({
      editingUsername: u.username,
      username: u.username,
      password: "",
      note: u.note || "",
      disabled: u.disabled,
      deviceScope: u.deviceScope,
      allowedDevices: [...(u.allowedDevices ?? [])],
      permissions: { ...u.permissions },
    });
  };

  const toggleDeviceSelection = (id: string) => {
    if (!form) return;
    const exists = form.allowedDevices.includes(id);
    setForm({
      ...form,
      allowedDevices: exists ? form.allowedDevices.filter((x) => x !== id) : [...form.allowedDevices, id],
    });
  };

  const deviceLabel = (id: string) => {
    const found = devices.find((d) => d.id === id);
    if (!found) return id;
    return found.name || id;
  };

  const subFormInvalid =
    !form ||
    form.username.trim().length < 2 ||
    (form.editingUsername === null && form.password.length < 4) ||
    (form.editingUsername !== null && form.password.length > 0 && form.password.length < 4);

  return (
    <div className="space-y-5">
      {data.defaultPasswordWarning && (
        <Alert tone="warn" title="正在使用默认管理员账号与密码">
          当前管理员账号仍为默认 <code className="font-mono">admin / admin</code>，建议在下方修改管理员用户名或密码以保障设备与短信安全。
        </Alert>
      )}

      {/* Admin account card */}
      <Card>
        <CardHeader
          icon={Shield}
          title="管理员账号"
          subtitle="系统固定保留唯一管理员账号，拥有 100% 全部功能与所有设备权限，不可删除"
          action={<Badge tone="ok">100% 全权限</Badge>}
        />
        <CardBody className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="管理员用户名" hint="2～32 位字符，支持修改（修改后当前登录会自动续期，其他设备退出）">
              <Input
                value={adminName}
                onChange={(e) => setAdminName(e.target.value)}
                placeholder="admin"
                autoComplete="username"
              />
            </Field>
            <Field label="备注（可选）" hint="例如：主管理员">
              <Input
                value={adminNote}
                onChange={(e) => setAdminNote(e.target.value)}
                placeholder="例如：NAS 主管理员"
              />
            </Field>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Field
              label="新密码（留空则不修改）"
              hint="至少 4 位，修改后其他设备上的管理员会话将自动退出"
              error={adminPassTooShort ? "密码至少 4 位" : undefined}
            >
              <Input
                type="password"
                autoComplete="new-password"
                value={adminPassword}
                onChange={(e) => setAdminPassword(e.target.value)}
                placeholder="留空保持当前密码不变"
              />
            </Field>
            <Field
              label="确认新密码"
              error={adminPassMismatch ? "两次输入的密码不一致" : undefined}
            >
              <Input
                type="password"
                autoComplete="new-password"
                value={adminConfirm}
                onChange={(e) => setAdminConfirm(e.target.value)}
                placeholder="再次输入新密码"
                disabled={!adminPassword}
              />
            </Field>
          </div>
          <div className="flex items-center gap-3">
            <Button
              variant="primary"
              icon={Save}
              loading={saveAdmin.isPending}
              disabled={!adminDirty || adminName.trim().length < 2 || adminPassMismatch || adminPassTooShort}
              onClick={() => saveAdmin.mutate()}
            >
              保存管理员信息
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Sub-accounts card */}
      <Card>
        <CardHeader
          icon={Users}
          title="子账号管理"
          subtitle="为家人或团队成员创建受限子账号，可按功能模块与 SIM 卡/设备精细授权"
          action={
            <Button
              size="sm"
              variant="primary"
              icon={UserPlus}
              onClick={() => setForm(emptySubForm())}
            >
              添加子账号
            </Button>
          }
        />
        <CardBody className="space-y-4">
          {form && (
            <div className="space-y-5 rounded-2xl bg-panel-2/80 p-4 ring-1 ring-accent/40 sm:p-5">
              <div className="flex items-center justify-between gap-2 border-b border-line pb-3">
                <div className="flex items-center gap-2 text-sm font-semibold">
                  {form.editingUsername === null ? <Plus className="size-4 text-accent" /> : <Pencil className="size-4 text-accent" />}
                  {form.editingUsername === null ? "新建子账号" : `编辑子账号：${form.editingUsername}`}
                </div>
                <Button size="sm" variant="ghost" icon={X} onClick={() => setForm(null)}>
                  取消
                </Button>
              </div>

              <div className="grid gap-4 md:grid-cols-3">
                <Field label="用户名" hint="2～32 位字符，不可与已有账号重复">
                  <Input
                    value={form.username}
                    onChange={(e) => setForm({ ...form, username: e.target.value })}
                    placeholder="例如 family1"
                  />
                </Field>
                <Field
                  label={form.editingUsername === null ? "登录密码" : "重置密码（留空不改）"}
                  hint="至少 4 位，子账号密码统一由管理员管理"
                >
                  <Input
                    type="password"
                    autoComplete="new-password"
                    value={form.password}
                    onChange={(e) => setForm({ ...form, password: e.target.value })}
                    placeholder={form.editingUsername === null ? "请输入登录密码" : "留空保持原密码"}
                  />
                </Field>
                <Field label="备注（可选）" hint="便于区分使用人或用途">
                  <Input
                    value={form.note}
                    onChange={(e) => setForm({ ...form, note: e.target.value })}
                    placeholder="例如：家人副卡、客服收验证码"
                  />
                </Field>
              </div>

              <div className="flex items-center justify-between gap-3 rounded-xl bg-panel px-3.5 py-2.5 ring-1 ring-line">
                <div>
                  <div className="text-sm font-medium">账号状态</div>
                  <div className="text-xs text-muted">{form.disabled ? "已停用：无法登录且现有登录会立即失效" : "正常启用：可按下方权限登录使用"}</div>
                </div>
                <Switch
                  checked={!form.disabled}
                  label="启用子账号"
                  onChange={(on) => setForm({ ...form, disabled: !on })}
                />
              </div>

              {/* Device / SIM scope */}
              <div className="space-y-3 rounded-2xl bg-panel p-4 ring-1 ring-line">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <Cpu className="size-4 text-accent" />
                    <div>
                      <div className="text-sm font-medium">可用设备 / SIM 卡范围</div>
                      <div className="text-xs text-muted">限制该子账号仅查看和操作指定模组的短信、通话与设备状态</div>
                    </div>
                  </div>
                  <Segmented
                    size="sm"
                    value={form.deviceScope}
                    onChange={(v) => setForm({ ...form, deviceScope: v })}
                    options={[
                      { value: "all", label: "全部设备" },
                      { value: "selected", label: "指定设备" },
                    ]}
                  />
                </div>
                {form.deviceScope === "selected" && (
                  <div className="space-y-2 pt-1">
                    {devices.length === 0 ? (
                      <p className="text-xs text-muted">当前尚未发现任何 4G 模组设备，连接模组后可在此勾选。</p>
                    ) : (
                      <div className="grid gap-2 sm:grid-cols-2">
                        {devices.map((d) => {
                          const checked = form.allowedDevices.includes(d.id);
                          return (
                            <button
                              key={d.id}
                              type="button"
                              onClick={() => toggleDeviceSelection(d.id)}
                              className={clsx(
                                "flex items-center gap-3 rounded-xl px-3 py-2.5 text-left text-xs ring-1 transition",
                                checked ? "bg-accent/10 ring-accent" : "bg-panel-2/60 ring-line hover:ring-line-strong",
                              )}
                            >
                              <span
                                className={clsx(
                                  "flex size-4 shrink-0 items-center justify-center rounded border",
                                  checked ? "border-accent bg-accent text-white" : "border-line-strong bg-panel",
                                )}
                              >
                                {checked && <Check className="size-3" />}
                              </span>
                              <div className="min-w-0 flex-1">
                                <div className="truncate font-medium text-fg">{d.name || d.id}</div>
                                <div className="truncate font-mono text-[11px] text-muted">
                                  {d.identity.number || d.modem.operator || "未读取号码"} · {d.id.slice(-6)}
                                </div>
                              </div>
                              <Badge tone={d.online && d.connected ? "ok" : "muted"}>
                                {d.online && d.connected ? "在线" : "离线"}
                              </Badge>
                            </button>
                          );
                        })}
                      </div>
                    )}
                    {form.allowedDevices.length === 0 && (
                      <p className="text-xs text-warn">未勾选任何设备时，该子账号将看不到任何 SIM 卡的短信和通话。</p>
                    )}
                  </div>
                )}
              </div>

              {/* Feature permissions */}
              <div className="space-y-4">
                <div className="space-y-2">
                  <div className="text-xs font-semibold tracking-wide text-muted">主功能权限（新建默认开启：短信、电话、通讯录）</div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {mainPermItems.map((item) => (
                      <div key={item.key} className="flex items-center justify-between gap-3 rounded-xl bg-panel px-3.5 py-2.5 ring-1 ring-line">
                        <div className="min-w-0">
                          <div className="text-sm font-medium">{item.label}</div>
                          <div className="truncate text-[11px] text-muted">{item.hint}</div>
                        </div>
                        <Switch
                          checked={form.permissions[item.key]}
                          label={item.label}
                          onChange={(v) =>
                            setForm({
                              ...form,
                              permissions: { ...form.permissions, [item.key]: v },
                            })
                          }
                        />
                      </div>
                    ))}
                  </div>
                </div>

                <div className="space-y-2">
                  <div className="text-xs font-semibold tracking-wide text-muted">设置页面权限（默认关闭，「账号管理」仅限管理员）</div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {settingsPermItems.map((item) => (
                      <div key={item.key} className="flex items-center justify-between gap-3 rounded-xl bg-panel px-3.5 py-2.5 ring-1 ring-line">
                        <div className="min-w-0">
                          <div className="text-sm font-medium">{item.label}</div>
                          <div className="truncate text-[11px] text-muted">{item.hint}</div>
                        </div>
                        <Switch
                          checked={form.permissions[item.key]}
                          label={item.label}
                          onChange={(v) =>
                            setForm({
                              ...form,
                              permissions: { ...form.permissions, [item.key]: v },
                            })
                          }
                        />
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2.5 pt-1">
                <Button
                  variant="primary"
                  icon={Save}
                  loading={saveSub.isPending}
                  disabled={subFormInvalid}
                  onClick={() => saveSub.mutate(form)}
                >
                  {form.editingUsername === null ? "创建子账号" : "保存修改"}
                </Button>
                <Button variant="ghost" onClick={() => setForm(null)}>
                  取消
                </Button>
              </div>
            </div>
          )}

          {subUsers.length === 0 ? (
            !form && (
              <Empty icon={Users} title="还没有子账号">
                点击右上角「添加子账号」，即可按功能模块和 SIM 卡精细分配访问权限。
              </Empty>
            )
          ) : (
            <div className="space-y-3">
              {subUsers.map((u) => {
                const enabledPerms = (Object.keys(allPermLabels) as (keyof UserPermissions)[]).filter(
                  (k) => u.permissions[k],
                );
                return (
                  <div
                    key={u.username}
                    className="flex flex-col gap-3 rounded-2xl bg-panel-2/60 p-4 ring-1 ring-line sm:flex-row sm:items-start sm:justify-between"
                  >
                    <div className="min-w-0 flex-1 space-y-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm font-semibold text-fg">{u.username}</span>
                        {u.disabled ? (
                          <Badge tone="bad">已停用</Badge>
                        ) : (
                          <Badge tone="ok">启用中</Badge>
                        )}
                        <Badge tone="info">
                          {u.deviceScope === "selected"
                            ? `指定设备 (${u.allowedDevices.length})`
                            : "全部设备"}
                        </Badge>
                        {u.note && <span className="text-xs text-muted">· {u.note}</span>}
                      </div>

                      {u.deviceScope === "selected" && u.allowedDevices.length > 0 && (
                        <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted">
                          <Cpu className="size-3.5 text-subtle" />
                          <span>授权卡槽：</span>
                          {u.allowedDevices.map((id) => (
                            <span key={id} className="rounded-md bg-panel px-1.5 py-0.5 font-mono text-fg ring-1 ring-line">
                              {deviceLabel(id)}
                            </span>
                          ))}
                        </div>
                      )}

                      <div className="flex flex-wrap gap-1.5">
                        {enabledPerms.length === 0 ? (
                          <span className="text-xs text-subtle">未开启任何功能权限</span>
                        ) : (
                          enabledPerms.map((k) => (
                            <span
                              key={k}
                              className="rounded-full bg-panel px-2.5 py-0.5 text-[11px] text-muted ring-1 ring-line"
                            >
                              {allPermLabels[k]}
                            </span>
                          ))
                        )}
                      </div>
                    </div>

                    <div className="flex shrink-0 flex-wrap items-center gap-1.5">
                      <Button size="sm" variant="secondary" icon={Pencil} onClick={() => openEditSub(u)}>
                        编辑 / 改密
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={u.disabled ? UserCheck : UserX}
                        loading={toggleSubDisabled.isPending}
                        onClick={() => toggleSubDisabled.mutate(u)}
                      >
                        {u.disabled ? "启用" : "停用"}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={Trash2}
                        className="text-bad hover:text-bad"
                        onClick={() => setDeleteTarget(u)}
                      >
                        删除
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          <p className="flex items-start gap-2 text-xs leading-relaxed text-muted">
            <KeyRound className="mt-0.5 size-3.5 shrink-0 text-subtle" />
            <span>
              权限变更会实时生效；修改子账号用户名、重置密码或停用账号时，该子账号在所有设备上的登录会话都会立即失效。
            </span>
          </p>
        </CardBody>
      </Card>

      <ConfirmModal
        open={!!deleteTarget}
        title={`确定删除子账号「${deleteTarget?.username ?? ""}」？`}
        description="删除后该子账号将立即被踢出登录，且无法恢复。短信与通话数据不会受影响。"
        confirmLabel="确定删除"
        danger
        loading={deleteSub.isPending}
        onConfirm={() => deleteTarget && deleteSub.mutate(deleteTarget.username)}
        onClose={() => setDeleteTarget(null)}
      />
    </div>
  );
}
