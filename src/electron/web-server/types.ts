/**
 * Web Access types — hosted mode for browser-based access.
 */

export interface WebAccessConfig {
  enabled: boolean;
  port: number;
  host: string;
  token: string;
  allowedOrigins: string[];
}

export const DEFAULT_WEB_ACCESS_CONFIG: WebAccessConfig = {
  enabled: false,
  port: 3847,
  host: "127.0.0.1",
  token: "",
  allowedOrigins: ["http://localhost:3847"],
};

export interface WebAccessRoute {
  method: "GET" | "POST" | "PUT" | "DELETE";
  path: string;
  ipcChannel: string;
  extractParams?: (req: Any) => Any;
}

export interface WebAccessStatus {
  running: boolean;
  url?: string;
  port?: number;
  connectedClients: number;
  startedAt?: number;
}

export type WebAccessCapabilityStatus = "available" | "planned" | "desktop_only";

export interface WebAccessCapability {
  id: string;
  label: string;
  status: WebAccessCapabilityStatus;
  transport?: "http" | "websocket" | "desktop-ipc" | "not-implemented";
  notes?: string;
}

export interface WebAccessCapabilities {
  mvpScope: "agent_task_loop";
  apiGroups: WebAccessCapability[];
  localFeatures: WebAccessCapability[];
}

export const WEB_ACCESS_CAPABILITIES: WebAccessCapabilities = {
  mvpScope: "agent_task_loop",
  apiGroups: [
    {
      id: "tasks",
      label: "Tasks and follow-up messages",
      status: "available",
      transport: "http",
      notes: "Create tasks, list tasks, load task details, and send follow-up messages.",
    },
    {
      id: "task-events",
      label: "Task event history and live task timeline",
      status: "available",
      transport: "websocket",
      notes: "History is served over HTTP; live timeline events stream over WebSocket.",
    },
    {
      id: "workspaces",
      label: "Workspace selection",
      status: "available",
      transport: "http",
      notes: "Web MVP uses existing server-side workspaces instead of local folder pickers.",
    },
    {
      id: "accounts",
      label: "Managed account status",
      status: "available",
      transport: "http",
      notes: "Read-only in WebAccess; writes remain desktop/admin-only for now.",
    },
  ],
  localFeatures: [
    {
      id: "files",
      label: "File browser and artifact preview",
      status: "planned",
      transport: "not-implemented",
      notes: "Needs server-side workspace file APIs, upload/download, and path sandboxing.",
    },
    {
      id: "terminal",
      label: "Terminal tabs",
      status: "planned",
      transport: "not-implemented",
      notes: "Needs WebSocket PTY sessions, command audit, and per-workspace permission checks.",
    },
    {
      id: "browser-workbench",
      label: "Browser Workbench",
      status: "planned",
      transport: "not-implemented",
      notes: "Electron webview must be replaced by a remote Playwright/CDP browser service.",
    },
    {
      id: "desktop-shell",
      label: "Window controls, tray, Finder, auto-update",
      status: "desktop_only",
      transport: "desktop-ipc",
      notes: "These remain desktop-only or map to browser/PWA equivalents.",
    },
  ],
};
