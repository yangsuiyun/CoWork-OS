# CoWork OS v2 — 开发进度

> 权威设计：`../docs/cowork-os-complete-design-spec.html`。本文件记录实现进度，
> 随每个里程碑更新。最后更新：2026-06-14（V2 审查阻断项修复：租户键、命令契约、查询分页、Graph 不变量）。

## 总览

| 里程碑 | 范围 | 状态 |
|--------|------|------|
| M0 | 契约冻结（OpenAPI + 事件/能力 Schema + 读模型 DDL + 权限矩阵 + codegen + 边界守卫） | ✅ 完成 |
| M1 | Walking Skeleton：Task 聚合端到端（命令 → event_log/outbox → 投影 → 读模型 → WS 推送 → 瘦客户端）+ 鉴权/租户/RLS | ✅ 完成 |
| M2 | 能力授权 + Pre/PostToolUse Hook + 外部会话 API + 前端瘦客户端 | ✅ 完成 |
| M3 | 本地 Agent Runner / 云沙箱执行 | 🚧 契约层完成（LocalRunnerSession 聚合）；运行时（反向通道/沙箱）独立立项，待二选一决策 |
| M4 | 编排 + 外部 Agent + 自学习 | 🚧 进行中（Workspace + ApprovalRequest + OrchestrationGraph + SkillCandidate 聚合已落地；ExternalAgentSession 待运行时） |
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
- **PI-2** append-only 事件存储，单调可见 `global_seq`（advisory lock）；事件流唯一性按 `tenant_id + stream_id + stream_seq` 约束。
- **PI-3** Task 聚合纯函数 `decide/apply` + 不变量（INV-1/2/3）。
- **PI-4** 命令处理 + HTTP API（JWT 鉴权 + 租户作用域）；`idempotencyKey` 与 `expectedStreamSeq` 已接入统一命令入口。
- **PI-5** 读模型投影器 + 确定性 Rebuild；所有读模型主键按租户内唯一设计。
- **PI-6** 最小权限 DB 角色强制 RLS（`cowork_app` RLS / `cowork_projector` BYPASSRLS）；投影器写入显式携带 `tenant_id`，避免 BYPASSRLS 下跨租户命中；运行时要求显式配置 `COWORK_PROJECTOR_DATABASE_URL`，不回退到请求路径 DSN。
- **PI-7** 基于 `LISTEN/NOTIFY` 的 WebSocket 实时事件流；支持 OpenAPI `cursor` 参数并保留 `from` 别名。
- **PI-8** codegen 契约类型作为唯一事实源接线。
- **PI-9** Task 审批 / 工件命令与投影补全。

### M2 能力授权 + Hook + 外部 API + 前端
- **PI-10** 能力令牌：HMAC 签名 + 使用时版本化撤销（`cap_revocation`，`migrations/0004`）。
- **PI-11** 确定性权限决策引擎（规则矩阵 + 不可覆盖硬拒绝 / 强制 ask）。
- **PI-12** `/v1/actions`：能力 + 权限双重门控端点。
- **PI-13** 可插拔 Hook 流水线（`PreToolUse` 拒绝/改写 + `PostToolUse` 审计），经 `cap.Guard` 接入；顺序：pre-hooks → 能力 → 规则 → post-hooks。
- **外部 API** `/v1/sessions`：create / get / cancel + SSE 事件流，会话即 Task。
- **前端** `web/`：React + Vite + TS 瘦客户端（任务列表、创建、WS 实时流），TS 类型由契约生成。
- **查询分页** `/v1/query/{name}`：已实现 `limit` / `cursor` 参数和 `nextCursor` 返回，覆盖 tasks/workspaces/approvals/graphNodes/skillCandidates/runners。

### M4 进行中 — Workspace + ApprovalRequest + OrchestrationGraph + SkillCandidate 聚合
- 事件契约：`WorkspaceCreated` / `PermissionsChanged`；`ApprovalRequested` / `ApprovalResolved`（独立 `approval:` 流）；`GraphSplit` / `NodeDispatched` / `NodeUpdated` / `ResultMerged`（`graph:` 流）；`SkillCandidateProposed` / `SkillCandidatePublished` / `SkillCandidateRejected`（`skillcandidate:` 流）。
- 读模型：`rm_workspaces`（`0005`）、`rm_approvals`（`0006`，含 `task_id`）、`rm_graph_nodes`（`0007`，每节点一行）、`rm_skill_candidates`（`0008`），均 RLS + 最小权限授权。
- 聚合：
  - `internal/kernel/workspace`（`CreateWorkspace` / `UpdatePermissions`，权限单调版本化）。
  - `internal/kernel/approval`（`RequestApproval` / `ApproveApproval` / `RejectApproval`，已决议终态——hard-deny 不可覆盖）。
  - `internal/kernel/graph`（编排 DAG，spec 12.1）：本地子代理与远程 Agent 共用节点抽象，仅 `dispatchTarget` 不同；远程节点以 `remoteTaskId` 收敛；不变量——节点 ID / dispatch target / 依赖存在性 / 无环校验，节点终态不可改，已 merge 图关闭，节点结果状态仅允许 `done|failed`。
  - `internal/kernel/skillcandidate`（自学习 Review-First，spec 13.2）：反思只产候选，须人工 review 才发布；review 决议终态不可覆盖。
- 应用层：`Service.Handle` 按聚合分派的 reducer 闭包（聚合无关）；`QueryWorkspaces` / `QueryApprovals` / `QueryGraphNodes` / `QuerySkillCandidates`。
- 投影：projector 按 stream 前缀路由；新增 `applyToGraph` / `applyToSkillCandidates`，均单调幂等。
- 查询：`/v1/query/{workspaces,approvals,graphNodes,skillCandidates}`，均支持统一分页参数。
- 设计决策：审批全量迁出 Task（pre-v1.0 无兼容层），单一权威事件源；`Switch` 命令属会话态、不产生聚合事件，暂缓；ExternalAgentSession（actor+租约+心跳+ACP/MCP 防腐）依赖运行时与外部协议，与 M3 运行时一并独立立项。

### M3 契约层 — LocalRunnerSession 聚合
- 事件契约：`RunnerRegistered` / `RunnerHeartbeat` / `RunnerStale`（`runner:` 流，spec 20.4）。
- 读模型：`rm_runners`（`0009`，跟踪最新心跳脉冲与存活态），RLS + 最小权限授权。
- 聚合：`internal/kernel/runner`（`RegisterRunner` / `RunnerHeartbeat` / `MarkRunnerStale`）：心跳脉冲单调；心跳/置 stale 须运行中；stale 后须重新注册（注册即恢复）。
- 投影 `applyToRunners`（register 用 upsert 支持恢复）；查询 `/v1/query/runners`。
- 设计决策：仅落地会话生命周期的事件契约 + 聚合纯函数；真实反向 gRPC/WS 隧道与存活看门狗独立立项，待「Local Agent Runner vs 云沙箱」二选一决策（spec 20）。

## 代码规模（快照）

- SQL 迁移 9，事件 Schema 24（含 Graph 4 / SkillCandidate 3 / Runner 3）。
- kernel 包：`app` / `events` / `projector` / `task` / `workspace` / `approval` / `graph` / `skillcandidate` / `runner`。
- 用户态：`cap`（能力、撤销、权限引擎、Hook、Guard）。
- 适配器：`adapter/http`（commands / query / stream / actions / sessions）。
- 测试：聚合纯函数单测（task/workspace/approval/graph/skillcandidate/runner）+ 投影/RLS/会话/动作集成测试（需 Postgres）。
- 契约 codegen：`server/pkg/contracts/events_gen.go` 已随新增 Schema 重新生成（`make codegen-check` 零漂移）。
- 验证快照：`go test ./...` 通过；`npm ci --registry https://repo.huaweicloud.com/repository/npm/ && npm run build` 通过（Node/npm 版本提示 warning，不影响构建）。

## 待办（按 spec 顺序）

1. **ExternalAgentSession**：`OpenSession/Enqueue/Heartbeat/Cancel`（演进，spec 12.2）；依赖运行时 + ACP/MCP 防腐适配器，需单独立项。
2. **M3 运行时**：在「Local Agent Runner 反向通道」与「服务端云沙箱」间二选一并实现真实执行器（spec 20，头号架构决策）。
3. **M4 编排运行时**：依赖就绪调度引擎（消费 `rm_graph_nodes`，驱动 `DispatchNode`/`UpdateNode`），与外部 Agent 节点收敛。
4. **M4 自学习运行时**：Reflection / Dreaming 产候选（驱动 `ProposeSkillCandidate`）+ 渐进式技能加载索引。
5. **前端**：工作区视图、审批交互、编排图视图、候选 review 面板、Runner 状态面板。

## 本地开发

见 `README.md` 的 Development 一节（后端 DSN、前端代理、`make codegen`）。
