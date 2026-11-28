import { useState, type FormEvent } from "react";
import { Lock, ShieldCheck, User } from "lucide-react";
import { api, ApiError, type SessionInfo } from "../lib/api";
import { Brand } from "../components/Layout";
import { Button, Input } from "../components/ui";

export function Login({ onLogin }: { onLogin: (session: SessionInfo) => void }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await api.post<SessionInfo & { ok: boolean }>("/api/login", {
        username: username.trim(),
        password,
      });
      onLogin({
        authenticated: true,
        username: res.username,
        role: res.role,
        permissions: res.permissions,
        deviceScope: res.deviceScope,
        allowedDevices: res.allowedDevices,
        defaultPasswordWarning: res.defaultPasswordWarning,
      });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "登录失败");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="relative flex min-h-full items-center justify-center px-4 py-12">
      <div className="grid-lines pointer-events-none absolute inset-0 [mask-image:radial-gradient(ellipse_at_center,black_20%,transparent_70%)] opacity-60" />
      <div className="animate-in relative w-full max-w-sm">
        <div className="mb-8 flex justify-center">
          <Brand />
        </div>
        <form onSubmit={submit} className="glass space-y-5 rounded-3xl p-7">
          <div>
            <h1 className="text-xl font-semibold tracking-tight">欢迎回来</h1>
            <p className="mt-1 text-sm text-muted">输入账号与密码，进入短信与设备控制台。</p>
          </div>
          <div className="space-y-3.5">
            <div className="space-y-1.5">
              <label htmlFor="username" className="text-[13px] font-medium">
                用户名
              </label>
              <div className="relative">
                <User className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-subtle" />
                <Input
                  id="username"
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="h-11 pl-10"
                  placeholder="admin"
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <label htmlFor="password" className="text-[13px] font-medium">
                密码
              </label>
              <div className="relative">
                <Lock className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-subtle" />
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="h-11 pl-10"
                  placeholder="••••••••"
                />
              </div>
              {error && <p className="text-xs text-bad">{error}</p>}
            </div>
          </div>
          <Button type="submit" variant="primary" size="lg" className="w-full" loading={busy} disabled={!username.trim() || !password}>
            登录
          </Button>
          <p className="flex items-start gap-2 text-xs leading-relaxed text-muted">
            <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-ok" />
            <span>
              首次安装默认管理员账号为 <code className="font-mono text-fg">admin</code>，默认密码 <code className="font-mono text-fg">admin</code>。忘记密码可在服务器上执行 <code className="font-mono text-fg">simcallhub passwd</code> 重置。
            </span>
          </p>
        </form>
      </div>
    </div>
  );
}
