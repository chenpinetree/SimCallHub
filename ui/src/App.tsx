import { Component, type ErrorInfo, type ReactNode, useEffect } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { hasAnySettingsPerm, hasPerm, useSession } from "./lib/auth";
import { useLiveUpdates } from "./lib/hooks";
import { Layout } from "./components/Layout";
import { Spinner } from "./components/ui";
import { Login } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { Messages } from "./pages/Messages";
import { Calls } from "./pages/Calls";
import { Contacts } from "./pages/Contacts";
import { Device } from "./pages/Device";
import { SettingsPage } from "./pages/Settings";
import { Logs } from "./pages/Logs";

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("UI render error:", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-full items-center justify-center p-6 text-center">
          <div className="max-w-md space-y-3 rounded-2xl bg-panel p-6 ring-1 ring-line">
            <h2 className="text-base font-semibold text-fg">页面渲染出现异常</h2>
            <p className="text-xs text-muted break-words">{this.state.error.message || "未知错误"}</p>
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="rounded-xl bg-accent px-4 py-2 text-sm font-medium text-white hover:opacity-90"
            >
              刷新页面
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export function App() {
  const qc = useQueryClient();
  const session = useSession();
  const sess = session.data;
  const authed = sess?.authenticated === true;
  const live = useLiveUpdates(authed);

  // Any 401 (expired session, password changed or account disabled) returns to login.
  useEffect(() => {
    const onUnauthorized = () => {
      qc.setQueryData(["session"], { authenticated: false });
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== "session" });
    };
    window.addEventListener("simcallhub:unauthorized", onUnauthorized);
    return () => window.removeEventListener("simcallhub:unauthorized", onUnauthorized);
  }, [qc]);

  if (session.isPending) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner className="size-7" />
      </div>
    );
  }
  if (!authed) {
    return (
      <Login
        onLogin={(info) => {
          qc.removeQueries({ predicate: (q) => q.queryKey[0] !== "session" });
          qc.setQueryData(["session"], info);
        }}
      />
    );
  }

  return (
    <ErrorBoundary>
      <Routes>
        <Route element={<Layout live={live} />}>
          <Route index element={<Dashboard />} />
          <Route path="messages" element={hasPerm(sess, "messages") ? <Messages /> : <Navigate to="/" replace />} />
          <Route path="calls" element={hasPerm(sess, "calls") ? <Calls /> : <Navigate to="/" replace />} />
          <Route path="contacts" element={hasPerm(sess, "contacts") ? <Contacts /> : <Navigate to="/" replace />} />
          <Route path="device" element={hasPerm(sess, "device") ? <Device /> : <Navigate to="/" replace />} />
          <Route path="settings" element={hasAnySettingsPerm(sess) ? <SettingsPage /> : <Navigate to="/" replace />} />
          <Route path="logs" element={hasPerm(sess, "logs") ? <Logs /> : <Navigate to="/" replace />} />
          <Route path="*" element={<Dashboard />} />
        </Route>
      </Routes>
    </ErrorBoundary>
  );
}
