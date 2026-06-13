# CoWork OS v2 — 开发进度

> 权威设计：`../docs/cowork-os-complete-design-spec.html`。本文件记录实现进度，
> 随每个里程碑更新。最后更新：2026-06-13。

## 总览

| 里程碑 | 范围 | 状态 |
|--------|------|------|
| M0 | 契约冻结（OpenAPI + 事件/能力 Schema + 读模型 DDL + 权限矩阵 + codegen + 边界守卫） | ✅ 完成 |
| M1 | Walking Skeleton：Task 聚合端到端（命令 → event_log/outbox → 投影 → 读模型 → WS 推送 → 瘦客户端）+ 鉴权/租户/RLS | ✅ 完成 |
| M2 | 能力授权 + Pre/PostToolUse Hook + 外部会话 API + 前端瘦客户端 | ✅ 完成 |
| M3 | 本地 Agent Runner / 云沙箱执行 | ⏳ 未开始（需运行时决策） |
| M4 | 编排 + 外部 Agent + 自学习 | 🚧 进行中（Workspace 聚合已落地） |
| M5 | 桌面端收敛 | ⏳ 未开始 |

## 已完成明细

### M0 契约基础
- `contracts/openapi.yaml`：命令/查询/动作/会话 HTTP 契约（OpenAPI 3.1）。
- `contracts/events/*.schema.json`：事件载荷 JSON Schema（13 个，含 Task / Approval / Artifact / Workspace）。
- `contracts/capability.schema.json`：能力令牌信封。
- 读模型 DDL（`migrations/0001`–`0002`）+ 权限规则矩阵（`permission-rules.yaml`）。
- codegen：`oapi-codegen`（Go）+ `go-jsonschema`（Go）+ `openapi-typescript`（TS），`make codegen-check` 做漂移守卫。
- `depguard` 强制 DAG：`kernel <- cap <- adapter`，下层不得反向依赖。

### M1 Walking Skeleton（PI-1 ~ PI-9）
- **PI-1** Go 服务骨架 + DB 就绪探针 `healthz`。
- **PI-2** append-only 事件存储，单调可见 `global_seq`（advisory lock）。
- **PI-3** Task 聚合纯函数 `decide/apply` + 不变量（INV-1/2/3）。
- **PI-4** 命令处理 + HTTP API（JWT 鉴权 + 租户作用域）。
- **PI-5** 读模型投影器 + 确定性 Rebuild。
- **PI-6** 最小权限 DB 角色强制 RLS（`cowork_app` RLS / `cowork_projector` BYPASSRLS）。
- **PI-7** 基于 `LISTEN/NOTIFY` 的 WebSocket 实时事件流。
- **PI-8** codegen 契约类型作为唯一事实源接线。
- **PI-9** Task 审批 / 工件命令与投影补全。

### M2 能力授权 + Hook + 外部 API + 前端
- **PI-10** 能力令牌：HMAC 签名 + 使用时版本化撤销（`cap_revocation`，`migrations/0004`）。
- **PI-11** 确定性权限决策引擎（规则矩阵 + 不可覆盖硬拒绝 / 强制 ask）。
- **PI-12** `/v1/actions`：能力 + 权限双重门控端点。
- **PI-13** 可插拔 Hook 流水线（`PreToolUse` 拒绝/改写 + `PostToolUse` 审计），经 `cap.Guard` 接入；顺序：pre-hooks → 能力 → 规则 → post-hooks。
- **外部 API** `/v1/sessions`：create / get / cancel + SSE 事件流，会话即 Task。
- **前端** `web/`：React + Vite + TS 瘦客户端（任务列表、创建、WS 实时流），TS 类型由契约生成。

### M4 进行中 — Workspace 聚合
- 事件契约：`WorkspaceCreated` / `PermissionsChanged`（`contracts/events/`）。
- 读模型：`rm_workspaces`（`migrations/0005`，RLS + 最小权限授权）。
- 聚合：`internal/kernel/workspace`，命令 `CreateWorkspace` / `UpdatePermissions`，权限单调版本化（capability-first）。
- 应用层：`Service.Handle` 重构为按聚合分派的 reducer 闭包（聚合无关）；`QueryWorkspaces`。
- 投影：projector 按 stream 前缀路由 + `applyToWorkspaces`；`/v1/query/workspaces`。
- 设计决策：`Switch` 命令属会话态、不产生聚合事件，暂缓。

## 代码规模（快照）

- Go 源文件 34（测试 13），SQL 迁移 5，事件 Schema 13。
- kernel 包：`app` / `events` / `projector` / `task` / `workspace`。
- 用户态：`cap`（能力、撤销、权限引擎、Hook、Guard）。
- 适配器：`adapter/http`（commands / query / stream / actions / sessions）。
- 测试：聚合纯函数单测 + 投影/RLS/会话/动作集成测试（需 Postgres）。

## 待办（按 spec 顺序）

1. **ApprovalRequest 聚合**：将审批抽为独立聚合，`data_export` 独立通道（当前审批以 Task 事件承载）。
2. **OrchestrationGraph**：`Split/DispatchNode/MergeResult`，远程节点以 `remoteTaskId` 收敛（需先定契约）。
3. **ExternalAgentSession**：`OpenSession/Enqueue/Heartbeat/Cancel`（演进，需先定契约）。
4. **M3 本地 Agent Runner**：执行运行时（LLM / 沙箱）— 需单独立项定契约。
5. **前端**：工作区视图、审批交互、会话面板。

## 本地开发

见 `README.md` 的 Development 一节（后端 DSN、前端代理、`make codegen`）。
