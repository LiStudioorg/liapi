package routing

import (
	"math/rand"
	"regexp"
	"sort"
	"sync"

	"liapi/config"
)

// HealthChecker is satisfied by stats.Health; keeps routing independent.
// LatencyMS is optional (0 when unknown) and used by the latency strategy.
type HealthChecker interface {
	IsHealthy(name string) bool
	LatencyMS(name string) int64
}

type Router struct {
	holder *config.Holder
	health HealthChecker

	ruleMu  sync.Mutex
	ruleCfg *config.Config
	ruleRes []*regexp.Regexp
}

func NewRouter(holder *config.Holder, health HealthChecker) *Router {
	return &Router{holder: holder, health: health}
}

// ResolveModel resolves aliases (config.Aliases) to the real model name.
// Falls back to the original name when no alias is configured.
func (rt *Router) ResolveModel(model string) string {
	m, _ := rt.ResolveAlias(model)
	return m
}

// ResolveAlias applies the exact aliases map first, then the first matching
// alias rule (exact or regex). Returns the resolved model name and any
// forced parameter overrides from the matched rule.
func (rt *Router) ResolveAlias(model string) (string, map[string]any) {
	cfg := rt.holder.Get()
	if v, ok := cfg.Aliases[model]; ok && v != "" {
		return v, nil
	}
	res, params := rt.matchRule(cfg, model)
	if res != "" {
		return res, params
	}
	return model, nil
}

func (rt *Router) matchRule(cfg *config.Config, model string) (string, map[string]any) {
	if len(cfg.AliasRules) == 0 {
		return "", nil
	}
	res := rt.compiled(cfg)
	for i, r := range cfg.AliasRules {
		if !r.Regex {
			if r.Pattern == model {
				return r.Model, r.Params
			}
			continue
		}
		if i < len(res) && res[i] != nil && res[i].MatchString(model) {
			return r.Model, r.Params
		}
	}
	return "", nil
}

// compiled lazily compiles regex rules and caches per config pointer
// (hot reload swaps the pointer, invalidating the cache).
func (rt *Router) compiled(cfg *config.Config) []*regexp.Regexp {
	rt.ruleMu.Lock()
	defer rt.ruleMu.Unlock()
	if rt.ruleCfg == cfg && rt.ruleRes != nil {
		return rt.ruleRes
	}
	out := make([]*regexp.Regexp, len(cfg.AliasRules))
	for i, r := range cfg.AliasRules {
		if r.Regex {
			re, err := regexp.Compile(r.Pattern)
			if err == nil {
				out[i] = re
			}
		}
	}
	rt.ruleCfg = cfg
	rt.ruleRes = out
	return out
}

// Candidates returns upstreams that can serve the model.
//
// Selection order:
//  1. fallbacks[model] — explicit ordered chain (validated in config).
//  2. otherwise: priority ascending; within the same priority the configured
//     route_strategy orders/weights: "" | priority (weighted random),
//     latency (weight scaled by inverse probe latency), cost (cheapest first
//     using prices[model], ties keep weight).
//
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

	var out []config.Upstream
	if chain, ok := cfg.Fallbacks[model]; ok && len(chain) > 0 {
		out = rt.chainOrder(cfg, chain, matched)
	} else {
		out = rt.strategyOrder(cfg, model, matched)
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

// chainOrder maps an explicit fallback chain onto matched upstreams,
// preserving chain order. Names not in matched (model mismatch) are skipped.
func (rt *Router) chainOrder(cfg *config.Config, chain []string, matched []config.Upstream) []config.Upstream {
	byName := make(map[string]config.Upstream, len(matched))
	for _, u := range matched {
		byName[u.Name] = u
	}
	out := make([]config.Upstream, 0, len(chain))
	for _, name := range chain {
		if u, ok := byName[name]; ok {
			out = append(out, u)
		}
	}
	return out
}

func (rt *Router) strategyOrder(cfg *config.Config, model string, matched []config.Upstream) []config.Upstream {
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

	strategy := cfg.RouteStrategy
	price := 0.0
	hasPrice := false
	if strategy == "cost" {
		if p, ok := cfg.Prices[model]; ok {
			price = p.Input
			hasPrice = true
		}
	}

	out := make([]config.Upstream, 0, len(matched))
	for _, p := range prios {
		g := groups[p]
		switch strategy {
		case "cost":
			out = append(out, costOrder(g, price, hasPrice)...)
		case "latency":
			out = append(out, rt.latencyOrder(g)...)
		default:
			out = append(out, weightedShuffle(g)...)
		}
	}
	return out
}

// latencyOrder reweights each upstream by inverse probe latency (unknown
// latency keeps its configured weight), then weighted-shuffles.
func (rt *Router) latencyOrder(g []config.Upstream) []config.Upstream {
	if rt.health == nil {
		return weightedShuffle(g)
	}
	rewritten := make([]config.Upstream, len(g))
	copy(rewritten, g)
	for i := range rewritten {
		lat := rt.health.LatencyMS(rewritten[i].Name)
		if lat <= 0 {
			continue
		}
		// weight_eff = weight * (1000 / max(lat,50)) clamped to [1, 1000]
		w := float64(rewritten[i].Weight) * (1000.0 / maxF(float64(lat), 50.0))
		if w < 1 {
			w = 1
		}
		if w > 1000 {
			w = 1000
		}
		rewritten[i].Weight = int(w)
	}
	return weightedShuffle(rewritten)
}

// costOrder sorts ascending by effective input price; missing price sorts
// last (assumed expensive) unless the whole group lacks prices.
func costOrder(g []config.Upstream, modelPrice float64, hasPrice bool) []config.Upstream {
	type key struct {
		u config.Upstream
		p float64
	}
	keys := make([]key, len(g))
	for i, u := range g {
		p := modelPrice
		if !hasPrice {
			// No configured price: fall back to priority as a stable cost proxy.
			p = float64(u.Priority)
		}
		keys[i] = key{u: u, p: p}
	}
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].p < keys[j].p })
	out := make([]config.Upstream, len(g))
	for i, k := range keys {
		out[i] = k.u
	}
	return out
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
