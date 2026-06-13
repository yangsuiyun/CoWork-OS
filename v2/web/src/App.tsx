import { useCallback, useEffect, useRef, useState } from "react";
import {
  createTask,
  getToken,
  listTasks,
  setToken,
  streamEvents,
  type CommittedEvent,
  type TaskRow,
} from "./api";

export default function App() {
  const [token, setTokenState] = useState(getToken());
  const [tasks, setTasks] = useState<TaskRow[]>([]);
  const [events, setEvents] = useState<CommittedEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [workspace, setWorkspace] = useState("default");
  const [error, setError] = useState("");
  const cursorRef = useRef(0);

  const refresh = useCallback(async () => {
    if (!getToken()) return;
    try {
      const page = await listTasks();
      setTasks((page.items as unknown as TaskRow[]) ?? []);
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

  const onSaveToken = (t: string) => {
    setToken(t);
    setTokenState(t);
  };

  const onCreate = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!prompt.trim()) return;
    try {
      await createTask(prompt, workspace);
      setPrompt("");
      setError("");
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div style={styles.page}>
      <header style={styles.header}>
        <h1 style={styles.h1}>CoWork OS</h1>
        <span style={{ ...styles.dot, background: connected ? "#22c55e" : "#9ca3af" }} />
        <span style={styles.conn}>{connected ? "live" : "offline"}</span>
      </header>

      <section style={styles.card}>
        <label style={styles.label}>JWT token</label>
        <input
          style={styles.input}
          value={token}
          placeholder="paste a dev JWT (tid + sub claims)"
          onChange={(e) => onSaveToken(e.target.value)}
        />
      </section>

      {error && <div style={styles.error}>{error}</div>}

      <section style={styles.card}>
        <h2 style={styles.h2}>New task</h2>
        <form onSubmit={onCreate} style={styles.form}>
          <input
            style={styles.input}
            value={workspace}
            onChange={(e) => setWorkspace(e.target.value)}
            placeholder="workspaceId"
          />
          <input
            style={styles.input}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="What should the agent do?"
          />
          <button style={styles.button} type="submit">
            Create
          </button>
        </form>
      </section>

      <section style={styles.grid}>
        <div style={styles.card}>
          <h2 style={styles.h2}>Tasks ({tasks.length})</h2>
          <ul style={styles.list}>
            {tasks.map((t) => (
              <li key={t.id} style={styles.row}>
                <span style={styles.badge(t.status)}>{t.status}</span>
                <span style={styles.taskTitle}>{t.title || t.id}</span>
                <span style={styles.meta}>{t.workspaceId}</span>
              </li>
            ))}
            {tasks.length === 0 && <li style={styles.empty}>No tasks yet.</li>}
          </ul>
        </div>

        <div style={styles.card}>
          <h2 style={styles.h2}>Live events</h2>
          <ul style={styles.list}>
            {events.map((e) => (
              <li key={`${e.streamId}:${e.streamSeq}`} style={styles.row}>
                <span style={styles.seq}>#{e.globalSeq}</span>
                <span style={styles.evType}>{e.type}</span>
                <span style={styles.meta}>{e.streamId}</span>
              </li>
            ))}
            {events.length === 0 && <li style={styles.empty}>Waiting for events…</li>}
          </ul>
        </div>
      </section>
    </div>
  );
}

const styles = {
  page: { maxWidth: 920, margin: "0 auto", padding: 24, fontFamily: "system-ui, sans-serif", color: "#111827" } as const,
  header: { display: "flex", alignItems: "center", gap: 10, marginBottom: 16 } as const,
  h1: { fontSize: 22, margin: 0 } as const,
  h2: { fontSize: 15, margin: "0 0 10px", color: "#374151" } as const,
  dot: { width: 10, height: 10, borderRadius: "50%" } as const,
  conn: { fontSize: 13, color: "#6b7280" } as const,
  card: { background: "#fff", border: "1px solid #e5e7eb", borderRadius: 12, padding: 16, marginBottom: 16 } as const,
  label: { display: "block", fontSize: 12, color: "#6b7280", marginBottom: 6 } as const,
  error: { background: "#fef2f2", color: "#b91c1c", border: "1px solid #fecaca", borderRadius: 8, padding: "8px 12px", marginBottom: 16, fontSize: 13 } as const,
  input: { width: "100%", padding: "8px 10px", border: "1px solid #d1d5db", borderRadius: 8, fontSize: 14, boxSizing: "border-box" } as const,
  form: { display: "grid", gridTemplateColumns: "1fr 2fr auto", gap: 8 } as const,
  button: { padding: "8px 16px", border: "none", borderRadius: 8, background: "#2563eb", color: "#fff", fontSize: 14, cursor: "pointer" } as const,
  grid: { display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 } as const,
  list: { listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 6 } as const,
  row: { display: "flex", alignItems: "center", gap: 8, fontSize: 13 } as const,
  taskTitle: { flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" } as const,
  evType: { flex: 1, fontFamily: "monospace" } as const,
  meta: { color: "#9ca3af", fontSize: 12 } as const,
  seq: { color: "#6b7280", fontVariantNumeric: "tabular-nums", width: 48 } as const,
  empty: { color: "#9ca3af", fontStyle: "italic" } as const,
  badge: (status: string) =>
    ({
      padding: "2px 8px",
      borderRadius: 999,
      fontSize: 11,
      fontWeight: 600,
      color: "#fff",
      background:
        status === "completed" ? "#22c55e" : status === "failed" ? "#ef4444" : status === "cancelled" ? "#6b7280" : status === "awaiting_approval" ? "#f59e0b" : "#3b82f6",
    }) as const,
};
