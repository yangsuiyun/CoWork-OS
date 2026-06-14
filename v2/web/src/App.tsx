import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  cancelManagedSession,
  createTask,
  createWorkspace,
  createManagedSession,
  getToken,
  loadSnapshot,
  resolveApproval,
  setToken,
  streamEvents,
  streamTaskEvents,
  type ApprovalView,
  type CommittedEvent,
  type GraphNodeView,
  type RunnerView,
  type SkillCandidateView,
  type Snapshot,
  type TaskView,
} from "./api";
import "./App.css";

type View = "brief" | "tasks" | "approvals" | "graph" | "skills" | "runners" | "events";

const EMPTY_SNAPSHOT: Snapshot = {
  tasks: [],
  workspaces: [],
  approvals: [],
  graphNodes: [],
  skillCandidates: [],
  runners: [],
};

const VIEWS: { id: View; label: string }[] = [
  { id: "brief", label: "Command Brief" },
  { id: "tasks", label: "Tasks" },
  { id: "approvals", label: "Approvals" },
  { id: "graph", label: "Graph" },
  { id: "skills", label: "Skills" },
  { id: "runners", label: "Runners" },
  { id: "events", label: "Events" },
];

export default function App() {
  const [token, setTokenState] = useState(getToken());
  const [snapshot, setSnapshot] = useState<Snapshot>(EMPTY_SNAPSHOT);
  const [events, setEvents] = useState<CommittedEvent[]>([]);
  const [taskEvents, setTaskEvents] = useState<CommittedEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [activeView, setActiveView] = useState<View>("brief");
  const [prompt, setPrompt] = useState("");
  const [workspace, setWorkspace] = useState("default");
  const [workspaceName, setWorkspaceName] = useState("");
  const [sessionPrompt, setSessionPrompt] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const cursorRef = useRef(0);

  const refresh = useCallback(async () => {
    if (!getToken()) return;
    try {
      setSnapshot(await loadSnapshot());
      setError("");
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);

  useEffect(() => {
    if (!token) return;
    refresh();
    const close = streamEvents(
      cursorRef.current,
      (e) => {
        cursorRef.current = e.globalSeq;
        setEvents((prev) => [e, ...prev].slice(0, 50));
        refresh();
      },
      setConnected,
    );
    return close;
  }, [token, refresh]);

  useEffect(() => {
    if (!selectedTaskId || !token) {
      setTaskEvents([]);
      return;
    }
    setTaskEvents([]);
    return streamTaskEvents(
      selectedTaskId,
      (e) => setTaskEvents((prev) => [e, ...prev].slice(0, 30)),
      setError,
    );
  }, [selectedTaskId, token]);

  const stats = useMemo(() => {
    const activeTasks = snapshot.tasks.filter((t) => ["pending", "planned", "running", "awaiting_approval"].includes(t.status)).length;
    const pendingApprovals = snapshot.approvals.filter((a) => a.status === "pending").length;
    const staleRunners = snapshot.runners.filter((r) => r.status === "stale").length;
    return { activeTasks, pendingApprovals, staleRunners };
  }, [snapshot]);

  const selectedTask = useMemo(
    () => snapshot.tasks.find((task) => task.id === selectedTaskId) ?? null,
    [selectedTaskId, snapshot.tasks],
  );

  const onSaveToken = (t: string) => {
    setToken(t);
    setTokenState(t);
  };

  const onCreate = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!prompt.trim()) return;
    try {
      setBusy(true);
      await createTask(prompt, workspace);
      setPrompt("");
      await refresh();
      setError("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onCreateWorkspace = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!workspaceName.trim()) return;
    const id = workspaceName.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "workspace";
    try {
      setBusy(true);
      await createWorkspace(id, workspaceName.trim());
      setWorkspace(id);
      setWorkspaceName("");
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onCreateSession = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!sessionPrompt.trim()) return;
    try {
      setBusy(true);
      const session = await createManagedSession(sessionPrompt, workspace);
      setSessionId(session.id);
      setSelectedTaskId(session.taskId ?? session.id);
      setSessionPrompt("");
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onResolveApproval = async (approval: ApprovalView, decision: "approve" | "reject") => {
    try {
      setBusy(true);
      await resolveApproval(approval.id, decision, `Resolved from web UI: ${decision}`);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onCancelSession = async () => {
    if (!selectedTaskId) return;
    try {
      setBusy(true);
      await cancelManagedSession(selectedTaskId);
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">CO</span>
          <div>
            <strong>CoWork OS</strong>
            <small>{connected ? "live event stream" : "offline"}</small>
          </div>
        </div>

        <label className="field">
          <span>JWT token</span>
          <input value={token} placeholder="tid + sub claims" onChange={(e) => onSaveToken(e.target.value)} />
        </label>

        <nav className="nav-list">
          {VIEWS.map((view) => (
            <button key={view.id} className={activeView === view.id ? "active" : ""} onClick={() => setActiveView(view.id)}>
              {view.label}
            </button>
          ))}
        </nav>

        <button className="ghost-button" onClick={refresh} disabled={!token || busy}>
          Refresh snapshot
        </button>
      </aside>

      <main className="main">
        <header className="topbar">
          <div>
            <p className="eyebrow">v2 Mission Control</p>
            <h1>{VIEWS.find((view) => view.id === activeView)?.label}</h1>
          </div>
          <div className="connection">
            <span className={connected ? "dot live" : "dot"} />
            {connected ? "Live" : "Disconnected"}
          </div>
        </header>

        {error && <div className="error-banner">{error}</div>}

        <section className="composer-grid">
          <Panel title="New task">
            <form onSubmit={onCreate} className="stack">
              <input value={workspace} onChange={(e) => setWorkspace(e.target.value)} placeholder="workspaceId" />
              <textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="What should the agent do?" />
              <button disabled={busy || !token || !prompt.trim()}>Create task</button>
            </form>
          </Panel>

          <Panel title="Managed session">
            <form onSubmit={onCreateSession} className="stack">
              <textarea value={sessionPrompt} onChange={(e) => setSessionPrompt(e.target.value)} placeholder="Create durable run via /v1/sessions" />
              <button disabled={busy || !token || !sessionPrompt.trim()}>Start session</button>
              {sessionId && <small className="muted">Last session: {sessionId}</small>}
            </form>
          </Panel>

          <Panel title="Workspace">
            <form onSubmit={onCreateWorkspace} className="stack">
              <input value={workspaceName} onChange={(e) => setWorkspaceName(e.target.value)} placeholder="Workspace name" />
              <button disabled={busy || !token || !workspaceName.trim()}>Create workspace</button>
            </form>
          </Panel>
        </section>

        {activeView === "brief" && (
          <CommandBrief snapshot={snapshot} stats={stats} events={events} onOpenView={setActiveView} onSelectTask={setSelectedTaskId} />
        )}
        {activeView === "tasks" && <TasksView tasks={snapshot.tasks} selectedTaskId={selectedTaskId} onSelectTask={setSelectedTaskId} />}
        {activeView === "approvals" && <ApprovalsView approvals={snapshot.approvals} busy={busy} onResolve={onResolveApproval} />}
        {activeView === "graph" && <GraphView nodes={snapshot.graphNodes} />}
        {activeView === "skills" && <SkillCandidatesView candidates={snapshot.skillCandidates} />}
        {activeView === "runners" && <RunnersView runners={snapshot.runners} />}
        {activeView === "events" && <EventsView events={events} />}
      </main>

      <aside className="detail-panel">
        <TaskDetail task={selectedTask} events={taskEvents} onCancel={onCancelSession} busy={busy} />
      </aside>
    </div>
  );
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="panel">
      <h2>{title}</h2>
      {children}
    </section>
  );
}

function CommandBrief({
  snapshot,
  stats,
  events,
  onOpenView,
  onSelectTask,
}: {
  snapshot: Snapshot;
  stats: { activeTasks: number; pendingApprovals: number; staleRunners: number };
  events: CommittedEvent[];
  onOpenView: (view: View) => void;
  onSelectTask: (id: string) => void;
}) {
  const latestTasks = snapshot.tasks.slice(0, 5);
  return (
    <div className="content-grid">
      <button className="metric-card" onClick={() => onOpenView("tasks")}><strong>{stats.activeTasks}</strong><span>active tasks</span></button>
      <button className="metric-card attention" onClick={() => onOpenView("approvals")}><strong>{stats.pendingApprovals}</strong><span>pending approvals</span></button>
      <button className="metric-card" onClick={() => onOpenView("graph")}><strong>{snapshot.graphNodes.length}</strong><span>graph nodes</span></button>
      <button className="metric-card danger" onClick={() => onOpenView("runners")}><strong>{stats.staleRunners}</strong><span>stale runners</span></button>
      <Panel title="Latest tasks">
        <ListEmpty show={latestTasks.length === 0} label="No tasks in the read model yet." />
        {latestTasks.map((task) => <TaskRow key={task.id} task={task} selected={false} onSelect={onSelectTask} />)}
      </Panel>
      <Panel title="Recent event stream">
        <ListEmpty show={events.length === 0} label="Waiting for committed events." />
        {events.slice(0, 8).map((event) => <EventRow key={`${event.streamId}:${event.streamSeq}`} event={event} />)}
      </Panel>
    </div>
  );
}

function TasksView({ tasks, selectedTaskId, onSelectTask }: { tasks: TaskView[]; selectedTaskId: string | null; onSelectTask: (id: string) => void }) {
  return (
    <Panel title={`Tasks (${tasks.length})`}>
      <ListEmpty show={tasks.length === 0} label="No tasks yet." />
      {tasks.map((task) => <TaskRow key={task.id} task={task} selected={task.id === selectedTaskId} onSelect={onSelectTask} />)}
    </Panel>
  );
}

function TaskRow({ task, selected, onSelect }: { task: TaskView; selected: boolean; onSelect: (id: string) => void }) {
  return (
    <button className={selected ? "data-row selected" : "data-row"} onClick={() => onSelect(task.id)}>
      <StatusPill status={task.status} />
      <span className="row-title">{task.title || task.id}</span>
      <span>{task.workspaceId || "no workspace"}</span>
      <span>#{task.updatedSeq}</span>
    </button>
  );
}

function ApprovalsView({ approvals, busy, onResolve }: { approvals: ApprovalView[]; busy: boolean; onResolve: (approval: ApprovalView, decision: "approve" | "reject") => void }) {
  return (
    <Panel title={`Approvals (${approvals.length})`}>
      <ListEmpty show={approvals.length === 0} label="No approvals are waiting." />
      {approvals.map((approval) => (
        <div className="data-row" key={approval.id}>
          <StatusPill status={approval.status} />
          <span className="row-title">{approval.kind}</span>
          <span>{approval.risk}</span>
          <span>{approval.taskId}</span>
          {approval.status === "pending" && (
            <span className="row-actions">
              <button disabled={busy} onClick={() => onResolve(approval, "approve")}>Approve</button>
              <button disabled={busy} className="danger-button" onClick={() => onResolve(approval, "reject")}>Reject</button>
            </span>
          )}
        </div>
      ))}
    </Panel>
  );
}

function GraphView({ nodes }: { nodes: GraphNodeView[] }) {
  return (
    <Panel title={`Graph nodes (${nodes.length})`}>
      <ListEmpty show={nodes.length === 0} label="No orchestration graph nodes yet." />
      {nodes.map((node) => (
        <div className="data-row" key={`${node.graphId}:${node.nodeId}`}>
          <StatusPill status={node.status} />
          <span className="row-title">{node.nodeId}</span>
          <span>{node.dispatchTarget}</span>
          <span>{node.outcome || "no outcome"}</span>
        </div>
      ))}
    </Panel>
  );
}

function SkillCandidatesView({ candidates }: { candidates: SkillCandidateView[] }) {
  return (
    <Panel title={`Skill candidates (${candidates.length})`}>
      <ListEmpty show={candidates.length === 0} label="No skill candidates proposed." />
      {candidates.map((candidate) => (
        <div className="data-row" key={candidate.id}>
          <StatusPill status={candidate.status} />
          <span className="row-title">{candidate.name}</span>
          <span>{candidate.summary || "No summary"}</span>
          <span>{candidate.reviewedBy || "unreviewed"}</span>
        </div>
      ))}
    </Panel>
  );
}

function RunnersView({ runners }: { runners: RunnerView[] }) {
  return (
    <Panel title={`Local runners (${runners.length})`}>
      <ListEmpty show={runners.length === 0} label="No local runners registered." />
      {runners.map((runner) => (
        <div className="data-row" key={runner.id}>
          <StatusPill status={runner.status} />
          <span className="row-title">{runner.id}</span>
          <span>{runner.workspaceId}</span>
          <span>pulse {runner.lastPulse}</span>
        </div>
      ))}
    </Panel>
  );
}

function EventsView({ events }: { events: CommittedEvent[] }) {
  return (
    <Panel title={`Live events (${events.length})`}>
      <ListEmpty show={events.length === 0} label="No events received on the websocket." />
      {events.map((event) => <EventRow key={`${event.streamId}:${event.streamSeq}`} event={event} />)}
    </Panel>
  );
}

function TaskDetail({ task, events, busy, onCancel }: { task: TaskView | null; events: CommittedEvent[]; busy: boolean; onCancel: () => void }) {
  if (!task) {
    return (
      <section className="panel sticky">
        <h2>Task detail</h2>
        <p className="muted">Select a task to inspect its session stream.</p>
      </section>
    );
  }
  return (
    <section className="panel sticky">
      <h2>Task detail</h2>
      <div className="detail-title">{task.title || task.id}</div>
      <StatusPill status={task.status} />
      <dl className="detail-list">
        <dt>ID</dt><dd>{task.id}</dd>
        <dt>Workspace</dt><dd>{task.workspaceId || "none"}</dd>
        <dt>Origin</dt><dd>{task.origin}</dd>
        <dt>Risk</dt><dd>{task.risk}</dd>
      </dl>
      <button className="danger-button wide" disabled={busy || ["completed", "failed", "cancelled"].includes(task.status)} onClick={onCancel}>
        Cancel session
      </button>
      <h3>Session events</h3>
      <ListEmpty show={events.length === 0} label="No session events streamed yet." />
      {events.map((event) => <EventRow key={`${event.streamId}:${event.streamSeq}`} event={event} compact />)}
    </section>
  );
}

function EventRow({ event, compact = false }: { event: CommittedEvent; compact?: boolean }) {
  return (
    <div className={compact ? "event-row compact" : "event-row"}>
      <span>#{event.globalSeq}</span>
      <strong>{event.type}</strong>
      <small>{event.streamId}</small>
    </div>
  );
}

function StatusPill({ status }: { status: string }) {
  return <span className={`status ${statusTone(status)}`}>{status || "unknown"}</span>;
}

function ListEmpty({ show, label }: { show: boolean; label: string }) {
  return show ? <p className="empty">{label}</p> : null;
}

function statusTone(status: string): string {
  if (["completed", "published", "active"].includes(status)) return "ok";
  if (["failed", "cancelled", "rejected", "stale"].includes(status)) return "danger";
  if (["awaiting_approval", "pending", "proposed"].includes(status)) return "attention";
  return "info";
}
