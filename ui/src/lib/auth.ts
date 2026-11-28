import { useQuery } from "@tanstack/react-query";
import { api, type PermissionKey, type SessionInfo, type UserPermissions } from "./api";

export const defaultSubUserPermissions: UserPermissions = {
  messages: true,
  calls: true,
  contacts: true,
  device: false,
  logs: false,
  settingsNotify: false,
  settingsAssistant: false,
  settingsSip: false,
  settingsCloud: false,
  settingsClassifier: false,
  settingsDevices: false,
  settingsGeneral: false,
};

export function useSession() {
  return useQuery({
    queryKey: ["session"],
    queryFn: () => api.get<SessionInfo>("/api/session"),
    staleTime: 60_000,
  });
}

export function isAdmin(session?: SessionInfo): boolean {
  return session?.authenticated === true && session.role === "admin";
}

export function hasPerm(session: SessionInfo | undefined, key: PermissionKey): boolean {
  if (!session?.authenticated) return false;
  if (session.role === "admin") return true;
  if (key === "admin") return false;
  return session.permissions?.[key] === true;
}

export function hasAnySettingsPerm(session?: SessionInfo): boolean {
  if (!session?.authenticated) return false;
  if (session.role === "admin") return true;
  const p = session.permissions;
  if (!p) return false;
  return (
    p.settingsNotify ||
    p.settingsAssistant ||
    p.settingsSip ||
    p.settingsCloud ||
    p.settingsClassifier ||
    p.settingsDevices ||
    p.settingsGeneral
  );
}
