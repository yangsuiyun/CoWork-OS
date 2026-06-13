package cap

import "context"

// HookContext is the mutable request passed through the hook pipeline. PreHooks
// may inspect and transform it (e.g. redact params) or deny, but never grant.
type HookContext struct {
	Actor    string
	TaskID   string
	Resource string
	Risk     string
	Context  map[string]any // tool params; transformable by pre-hooks
}

// HookOutcome is a pre-hook result. A denied outcome short-circuits the
// pipeline; the capability and rule checks are never reached.
type HookOutcome struct {
	Denied bool
	Reason string
}

// PreHook runs before the capability check and may deny or transform.
type PreHook interface {
	PreToolUse(ctx context.Context, hc *HookContext) (HookOutcome, error)
}

// PostHook runs after the decision for audit/observability side-effects.
type PostHook interface {
	PostToolUse(ctx context.Context, hc *HookContext, decision Decision) error
}

// HookPipeline runs ordered pre/post hooks (spec 11.2 step 1 and post-step).
type HookPipeline struct {
	pre  []PreHook
	post []PostHook
}

// NewHookPipeline constructs a pipeline from ordered pre and post hooks.
func NewHookPipeline(pre []PreHook, post []PostHook) *HookPipeline {
	return &HookPipeline{pre: pre, post: post}
}

// RunPre executes pre-hooks in order, stopping at the first deny.
func (p *HookPipeline) RunPre(ctx context.Context, hc *HookContext) (HookOutcome, error) {
	for _, h := range p.pre {
		out, err := h.PreToolUse(ctx, hc)
		if err != nil {
			return HookOutcome{}, err
		}
		if out.Denied {
			return out, nil
		}
	}
	return HookOutcome{}, nil
}

// RunPost executes post-hooks in order; the first error stops the chain.
func (p *HookPipeline) RunPost(ctx context.Context, hc *HookContext, decision Decision) error {
	for _, h := range p.post {
		if err := h.PostToolUse(ctx, hc, decision); err != nil {
			return err
		}
	}
	return nil
}

// DenyResourceHook is a built-in pre-hook that denies a configured set of
// resource classes outright (e.g. an org policy banning raw shell).
type DenyResourceHook struct {
	blocked map[string]bool
}

// NewDenyResourceHook blocks the given resource classes.
func NewDenyResourceHook(resources ...string) *DenyResourceHook {
	m := make(map[string]bool, len(resources))
	for _, r := range resources {
		m[r] = true
	}
	return &DenyResourceHook{blocked: m}
}

// PreToolUse denies if the resource is in the blocklist.
func (h *DenyResourceHook) PreToolUse(_ context.Context, hc *HookContext) (HookOutcome, error) {
	if h.blocked[hc.Resource] {
		return HookOutcome{Denied: true, Reason: "resource blocked by policy: " + hc.Resource}, nil
	}
	return HookOutcome{}, nil
}

// Guard bundles the use-time capability verifier with the hook pipeline so the
// HTTP adapter has a single authorization dependency.
type Guard struct {
	verifier *Verifier
	hooks    *HookPipeline
}

// NewGuard constructs a Guard.
func NewGuard(verifier *Verifier, hooks *HookPipeline) *Guard {
	return &Guard{verifier: verifier, hooks: hooks}
}

// RunPre runs the pre-hook chain.
func (g *Guard) RunPre(ctx context.Context, hc *HookContext) (HookOutcome, error) {
	return g.hooks.RunPre(ctx, hc)
}

// RunPost runs the post-hook chain.
func (g *Guard) RunPost(ctx context.Context, hc *HookContext, decision Decision) error {
	return g.hooks.RunPost(ctx, hc, decision)
}

// Verify exposes the capability verifier.
func (g *Guard) Verify(ctx context.Context, tenant, token string) (Capability, error) {
	return g.verifier.Verify(ctx, tenant, token)
}
