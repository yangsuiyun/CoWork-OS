package cap

import (
	"context"
	"testing"
)

// transformHook redacts a "secret" param to prove pre-hooks can mutate context.
type transformHook struct{}

func (transformHook) PreToolUse(_ context.Context, hc *HookContext) (HookOutcome, error) {
	if hc.Context != nil {
		if _, ok := hc.Context["secret"]; ok {
			hc.Context["secret"] = "[redacted]"
		}
	}
	return HookOutcome{}, nil
}

// recordPost captures decisions for assertion.
type recordPost struct{ seen []Decision }

func (r *recordPost) PostToolUse(_ context.Context, _ *HookContext, d Decision) error {
	r.seen = append(r.seen, d)
	return nil
}

func TestHookPipelineDenyShortCircuits(t *testing.T) {
	ctx := context.Background()
	post := &recordPost{}
	p := NewHookPipeline([]PreHook{NewDenyResourceHook("shell"), transformHook{}}, []PostHook{post})

	hc := &HookContext{Resource: "shell", Context: map[string]any{"secret": "x"}}
	out, err := p.RunPre(ctx, hc)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Denied {
		t.Fatalf("expected deny for blocked resource, got %+v", out)
	}
	// Deny short-circuits before the transform hook runs.
	if hc.Context["secret"] != "x" {
		t.Fatalf("transform hook should not have run after deny, got %v", hc.Context["secret"])
	}
}

func TestHookPipelineTransformAndPost(t *testing.T) {
	ctx := context.Background()
	post := &recordPost{}
	p := NewHookPipeline([]PreHook{NewDenyResourceHook("shell"), transformHook{}}, []PostHook{post})

	hc := &HookContext{Resource: "fs.read", Context: map[string]any{"secret": "x"}}
	out, err := p.RunPre(ctx, hc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Denied {
		t.Fatalf("fs.read should not be denied")
	}
	if hc.Context["secret"] != "[redacted]" {
		t.Fatalf("transform hook should have redacted secret, got %v", hc.Context["secret"])
	}
	if err := p.RunPost(ctx, hc, Allow); err != nil {
		t.Fatal(err)
	}
	if len(post.seen) != 1 || post.seen[0] != Allow {
		t.Fatalf("post hook should observe Allow, got %+v", post.seen)
	}
}
