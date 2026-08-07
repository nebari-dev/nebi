package pkgmgr

import (
	"context"
	"os"
	"sort"
	"strings"
)

type environmentContextKey struct{}

// WithEnvironment attaches additional environment variables to package-manager
// commands executed with ctx. The input map is copied so callers can mutate it.
func WithEnvironment(ctx context.Context, env map[string]string) context.Context {
	if len(env) == 0 {
		return ctx
	}
	copied := make(map[string]string, len(env))
	for key, value := range env {
		copied[key] = value
	}
	return context.WithValue(ctx, environmentContextKey{}, copied)
}

// environmentFromContext returns a copy of additional package-manager
// environment variables attached to ctx.
func environmentFromContext(ctx context.Context) map[string]string {
	env, ok := ctx.Value(environmentContextKey{}).(map[string]string)
	if !ok || len(env) == 0 {
		return nil
	}
	copied := make(map[string]string, len(env))
	for key, value := range env {
		copied[key] = value
	}
	return copied
}

// CommandEnvironment returns an os.Environ-compatible slice with ctx variables
// merged in. It returns nil when no extra variables are configured, letting
// exec.Command inherit the process environment normally.
func CommandEnvironment(ctx context.Context) []string {
	extra := environmentFromContext(ctx)
	if len(extra) == 0 {
		return nil
	}
	return mergeEnvironment(os.Environ(), extra)
}

// mergeEnvironment overlays extra variables onto base and returns a stable
// KEY=value slice suitable for exec.Cmd.Env.
func mergeEnvironment(base []string, extra map[string]string) []string {
	merged := make(map[string]string, len(base)+len(extra))
	for _, entry := range base {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+merged[key])
	}
	return result
}
