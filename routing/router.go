package routing

import (
	"math/rand"
	"sort"

	"liapi/config"
)

// HealthChecker is satisfied by stats.Health; keeps routing independent.
type HealthChecker interface {
	IsHealthy(name string) bool
}

type Router struct {
	holder *config.Holder
	health HealthChecker
}

func NewRouter(holder *config.Holder, health HealthChecker) *Router {
	return &Router{holder: holder, health: health}
}

// ResolveModel resolves aliases (config.Aliases) to the real model name.
// Falls back to the original name when no alias is configured.
func (rt *Router) ResolveModel(model string) string {
	if v, ok := rt.holder.Get().Aliases[model]; ok && v != "" {
		return v
	}
	return model
}

// Candidates returns upstreams that can serve the model, ordered by
// priority (ascending); within the same priority, weighted random shuffle.
// Unhealthy upstreams are skipped when cfg.SkipUnhealthy is on — but only if
// at least one healthy candidate remains (fallback to full list otherwise).
func (rt *Router) Candidates(model string) []config.Upstream {
	cfg := rt.holder.Get()

	var matched []config.Upstream
	for _, u := range cfg.Upstreams {
		if !u.Enabled() {
			continue
		}
		if u.Matches(model) {
			matched = append(matched, u)
		}
	}
	if len(matched) == 0 {
		return nil
	}

	groups := map[int][]config.Upstream{}
	var prios []int
	for _, u := range matched {
		p := u.Priority
		if _, ok := groups[p]; !ok {
			prios = append(prios, p)
		}
		groups[p] = append(groups[p], u)
	}
	sort.Ints(prios)

	out := make([]config.Upstream, 0, len(matched))
	for _, p := range prios {
		out = append(out, weightedShuffle(groups[p])...)
	}

	if cfg.SkipUnhealthy && rt.health != nil {
		var healthy []config.Upstream
		for _, u := range out {
			if rt.health.IsHealthy(u.Name) {
				healthy = append(healthy, u)
			}
		}
		if len(healthy) > 0 {
			out = healthy
		}
	}
	return out
}

// weightedShuffle expands indices by weight, shuffles, then dedupes while
// preserving encounter order (e.g. [A,A,A,B] -> A then B with A first).
func weightedShuffle(us []config.Upstream) []config.Upstream {
	if len(us) <= 1 {
		return us
	}
	expanded := make([]int, 0, len(us)*3)
	for i, u := range us {
		w := u.Weight
		if w <= 0 {
			w = 1
		}
		if w > 1000 {
			w = 1000
		}
		for k := 0; k < w; k++ {
			expanded = append(expanded, i)
		}
	}
	rand.Shuffle(len(expanded), func(i, j int) {
		expanded[i], expanded[j] = expanded[j], expanded[i]
	})
	out := make([]config.Upstream, 0, len(us))
	seen := make(map[int]bool, len(us))
	for _, idx := range expanded {
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, us[idx])
	}
	return out
}
