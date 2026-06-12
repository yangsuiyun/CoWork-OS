export interface WebWorkspace {
  id: string;
  name: string;
  path?: string;
}

export interface WebTask {
  id: string;
  title?: string;
  status?: string;
  workspaceId?: string;
  createdAt?: number;
  updatedAt?: number;
}

export interface WebTaskEvent {
  id?: string;
  eventId?: string;
  taskId?: string;
  type?: string;
  createdAt?: number;
  timestamp?: number;
  payload?: unknown;
}

export interface WebAccessCapability {
  id: string;
  label: string;
  status: "available" | "planned" | "desktop_only";
  transport?: "http" | "websocket" | "desktop-ipc" | "not-implemented";
  notes?: string;
}

export interface WebAccessCapabilities {
  mvpScope: "agent_task_loop";
  apiGroups: WebAccessCapability[];
  localFeatures: WebAccessCapability[];
}

export interface WebAccessLiveEvent {
  event: string;
  payload?: unknown;
  timestamp?: number;
}

export interface WebAccessPlatformAPI {
  health(): Promise<{ status: string; timestamp: number }>;
  capabilities(): Promise<WebAccessCapabilities>;
  listWorkspaces(): Promise<WebWorkspace[]>;
  listTasks(): Promise<WebTask[]>;
  createTask(input: { title: string; prompt: string; workspaceId?: string }): Promise<WebTask>;
  listTaskEvents(taskId: string): Promise<WebTaskEvent[]>;
  sendTaskMessage(input: { taskId: string; message: string }): Promise<unknown>;
  connectLiveEvents(
    onEvent: (event: WebAccessLiveEvent) => void,
    onStatus?: (status: "connecting" | "connected" | "closed" | "error") => void,
  ): () => void;
}

export function createWebAccessPlatformAPI(tokenProvider: () => string): WebAccessPlatformAPI {
  const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
    const headers = new Headers(init?.headers || {});
    headers.set("Content-Type", "application/json");
    const token = tokenProvider().trim();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    const response = await fetch(path, { ...init, headers });
    const bodyText = await response.text();
    const parsed = bodyText ? JSON.parse(bodyText) : null;
    if (!response.ok) {
      const msg =
        typeof parsed?.error === "string"
          ? parsed.error
          : `Request failed (${response.status}) for ${path}`;
      throw new Error(msg);
    }
    return parsed as T;
  };

  return {
    health: () => request("/api/health"),
    capabilities: () => request("/api/capabilities"),
    listWorkspaces: () => request("/api/workspaces"),
    listTasks: () => request("/api/tasks"),
    createTask: (input) =>
      request("/api/tasks", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    listTaskEvents: (taskId) => request(`/api/tasks/${encodeURIComponent(taskId)}/events`),
    sendTaskMessage: (input) =>
      request(`/api/tasks/${encodeURIComponent(input.taskId)}/message`, {
        method: "POST",
        body: JSON.stringify({ message: input.message }),
      }),
    connectLiveEvents: (onEvent, onStatus) => {
      const token = tokenProvider().trim();
      if (!token) {
        onStatus?.("closed");
        return () => {};
      }

      const scheme = window.location.protocol === "https:" ? "wss" : "ws";
      const socket = new WebSocket(
        `${scheme}://${window.location.host}/ws?token=${encodeURIComponent(token)}`,
      );
      let closedByClient = false;

      onStatus?.("connecting");
      socket.addEventListener("open", () => onStatus?.("connected"));
      socket.addEventListener("message", (message) => {
        try {
          const parsed = JSON.parse(String(message.data)) as WebAccessLiveEvent;
          if (parsed && typeof parsed.event === "string") {
            onEvent(parsed);
          }
        } catch {
          // Ignore malformed frames from stale or incompatible servers.
        }
      });
      socket.addEventListener("error", () => {
        if (!closedByClient) onStatus?.("error");
      });
      socket.addEventListener("close", () => {
        if (!closedByClient) onStatus?.("closed");
      });

      return () => {
        closedByClient = true;
        socket.close();
      };
    },
  };
}
