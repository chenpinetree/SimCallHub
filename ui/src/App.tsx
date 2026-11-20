import { Component, type ErrorInfo, type ReactNode, useEffect } from "react";
import { Route, Routes } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./lib/api";
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
  const session = useQuery({ queryKey: ["session"], queryFn: () => api.get<{ authenticated: boolean }>("/api/session"), staleTime: Infinity });
  const authed = session.data?.authenticated === true;
  const live = useLiveUpdates(authed);

  // Any 401 (expired session, password changed elsewhere) returns to login.
  useEffect(() => {
    const onUnauthorized = () => qc.setQueryData(["session"], { authenticated: false });
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
  if (!authed) return <Login onLogin={() => qc.setQueryData(["session"], { authenticated: true })} />;

  return (
    <ErrorBoundary>
      <Routes>
        <Route element={<Layout live={live} />}>
          <Route index element={<Dashboard />} />
          <Route path="messages" element={<Messages />} />
          <Route path="calls" element={<Calls />} />
          <Route path="contacts" element={<Contacts />} />
          <Route path="device" element={<Device />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="logs" element={<Logs />} />
          <Route path="*" element={<Dashboard />} />
        </Route>
      </Routes>
    </ErrorBoundary>
  );
}
