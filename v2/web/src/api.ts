import type { components } from "./contracts/openapi";

export type CommittedEvent = components["schemas"]["CommittedEvent"];
export type DomainError = components["schemas"]["DomainError"];
export type ReadModelPage = components["schemas"]["ReadModelPage"];

// TaskRow is the read-model shape returned under ReadModelPage.items. The
// contract types items as generic objects, so we narrow the fields we render.
export interface TaskRow {
  id: string;
  workspaceId: string;
  status: string;
  title: string;
  risk: string;
  origin: string;
  updatedSeq: number;
}

const TOKEN_KEY = "cowork_token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setToken(tok: string): void {
  localStorage.setItem(TOKEN_KEY, tok);
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${getToken()}`,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const err = data as DomainError | null;
    throw new Error(err?.message ?? `HTTP ${res.status}`);
  }
  return data as T;
}

export function listTasks(): Promise<ReadModelPage> {
  return request<ReadModelPage>("GET", "/v1/query/tasks");
}

export function createTask(prompt: string, workspaceId: string): Promise<unknown> {
  return request("POST", "/v1/commands", {
    type: "CreateTask",
    payload: { canonicalPrompt: prompt, workspaceId, origin: "api", risk: "low" },
  });
}

// streamEvents opens the WebSocket live stream from the given cursor. Browsers
// cannot set headers on WS, so the JWT travels as a query param (server allows
// this fallback). Returns a close function.
export function streamEvents(
  from: number,
  onEvent: (e: CommittedEvent) => void,
  onStatus: (connected: boolean) => void,
): () => void {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const url = `${proto}://${location.host}/v1/stream?from=${from}&token=${encodeURIComponent(getToken())}`;
  const ws = new WebSocket(url);
  ws.onopen = () => onStatus(true);
  ws.onclose = () => onStatus(false);
  ws.onmessage = (msg) => {
    try {
      onEvent(JSON.parse(msg.data) as CommittedEvent);
    } catch {
      // Ignore non-JSON frames (e.g. keepalives).
    }
  };
  return () => ws.close();
}
