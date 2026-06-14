import type { components } from "./contracts/openapi";

export type CommittedEvent = components["schemas"]["CommittedEvent"];
export type DomainError = components["schemas"]["DomainError"];
export type ReadModelPage = components["schemas"]["ReadModelPage"];
export type ManagedSession = components["schemas"]["ManagedSession"];

// Read models are returned under ReadModelPage.items. The OpenAPI contract keeps
// items generic, so this file is the single client-side narrowing point.
export interface TaskView {
  id: string;
  workspaceId: string;
  status: string;
  title: string;
  risk: string;
  origin: string;
  updatedSeq: number;
}

export interface WorkspaceView {
  id: string;
  name: string;
  permissions?: { paths?: string[]; domains?: string[] } | Record<string, unknown>;
  permissionsVersion: number;
  updatedSeq: number;
}

export interface ApprovalView {
  id: string;
  taskId: string;
  kind: string;
  risk: string;
  status: string;
  resolvedBy: string;
  updatedSeq: number;
}

export interface GraphNodeView {
  graphId: string;
  nodeId: string;
  taskId: string;
  dispatchTarget: string;
  remoteTaskId: string;
  status: string;
  outcome: string;
  updatedSeq: number;
}

export interface SkillCandidateView {
  id: string;
  name: string;
  sourceTaskId: string;
  summary: string;
  status: string;
  reviewedBy: string;
  updatedSeq: number;
}

export interface RunnerView {
  id: string;
  workspaceId: string;
  status: string;
  lastPulse: number;
  updatedSeq: number;
}

export interface Snapshot {
  tasks: TaskView[];
  workspaces: WorkspaceView[];
  approvals: ApprovalView[];
  graphNodes: GraphNodeView[];
  skillCandidates: SkillCandidateView[];
  runners: RunnerView[];
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

async function query<T>(name: string): Promise<T[]> {
  const page = await request<ReadModelPage>("GET", `/v1/query/${name}`);
  return (page.items as T[] | undefined) ?? [];
}

export function listTasks(): Promise<TaskView[]> {
  return query<TaskView>("tasks");
}

export function listWorkspaces(): Promise<WorkspaceView[]> {
  return query<WorkspaceView>("workspaces");
}

export function listApprovals(): Promise<ApprovalView[]> {
  return query<ApprovalView>("approvals");
}

export function listGraphNodes(): Promise<GraphNodeView[]> {
  return query<GraphNodeView>("graphNodes");
}

export function listSkillCandidates(): Promise<SkillCandidateView[]> {
  return query<SkillCandidateView>("skillCandidates");
}

export function listRunners(): Promise<RunnerView[]> {
  return query<RunnerView>("runners");
}

export async function loadSnapshot(): Promise<Snapshot> {
  const [tasks, workspaces, approvals, graphNodes, skillCandidates, runners] = await Promise.all([
    listTasks(),
    listWorkspaces(),
    listApprovals(),
    listGraphNodes(),
    listSkillCandidates(),
    listRunners(),
  ]);
  return { tasks, workspaces, approvals, graphNodes, skillCandidates, runners };
}

export function dispatchCommand(type: string, payload: Record<string, unknown>): Promise<{ events: CommittedEvent[] }> {
  return request("POST", "/v1/commands", { type, payload });
}

export function createTask(prompt: string, workspaceId: string): Promise<unknown> {
  return dispatchCommand("CreateTask", {
    canonicalPrompt: prompt,
    workspaceId,
    origin: "api",
    risk: "low",
  });
}

export function createWorkspace(workspaceId: string, name: string): Promise<unknown> {
  return dispatchCommand("CreateWorkspace", { workspaceId, name });
}

export function resolveApproval(approvalId: string, decision: "approve" | "reject", reason: string): Promise<unknown> {
  const command = decision === "approve" ? "ApproveApproval" : "RejectApproval";
  return dispatchCommand(command, { approvalId, reason });
}

export function createManagedSession(prompt: string, workspaceId: string): Promise<ManagedSession> {
  const body = workspaceId ? { prompt, workspaceId } : { prompt };
  return request<ManagedSession>("POST", "/v1/sessions", body);
}

export function getManagedSession(id: string): Promise<ManagedSession> {
  return request<ManagedSession>("GET", `/v1/sessions/${encodeURIComponent(id)}`);
}

export function cancelManagedSession(id: string): Promise<{ events: CommittedEvent[] }> {
  return request("POST", `/v1/sessions/${encodeURIComponent(id)}/cancel`);
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

export function streamTaskEvents(taskId: string, onEvent: (e: CommittedEvent) => void, onError: (message: string) => void): () => void {
  const source = new EventSource(`/v1/sessions/${encodeURIComponent(taskId)}/events?token=${encodeURIComponent(getToken())}`);
  source.onmessage = (msg) => onEvent(JSON.parse(msg.data) as CommittedEvent);
  source.onerror = () => onError("Task event stream disconnected");
  return () => source.close();
}
