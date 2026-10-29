package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultUsageURL      = "https://opencode.ai/zen/go/v1/usage"
	defaultUsageCacheTTL = 60 * time.Second
	usageTimeout         = 5 * time.Second
	maxKeyNameLen        = 40
)

// parseKeyNames splits OPENCODE_GO_KEY_NAMES. Positions matter (name i labels
// key i), so empty entries are kept. Names lose line breaks and are cut at 40
// characters.
func parseKeyNames(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	strip := strings.NewReplacer("\r", "", "\n", "")
	parts := strings.Split(v, ",")
	for i, p := range parts {
		r := []rune(strings.TrimSpace(strip.Replace(p)))
		if len(r) > maxKeyNameLen {
			r = r[:maxKeyNameLen]
		}
		parts[i] = strings.TrimSpace(string(r))
	}
	return parts
}

func (c Config) keyName(i int) string {
	if i < len(c.KeyNames) {
		return c.KeyNames[i]
	}
	return ""
}

type UsageWindow struct {
	Percent  float64         `json:"percent"`
	ResetsAt json.RawMessage `json:"resets_at,omitempty"`
}

type UsageWindows struct {
	Rolling *UsageWindow `json:"rolling,omitempty"`
	Weekly  *UsageWindow `json:"weekly,omitempty"`
	Monthly *UsageWindow `json:"monthly,omitempty"`
}

type KeyReport struct {
	Index             int           `json:"index"`
	Name              string        `json:"name"`
	State             string        `json:"state"`
	Current           bool          `json:"current"`
	Eligible          bool          `json:"eligible"`
	Last429Time       string        `json:"last_429_time,omitempty"`
	RetryAfterSeconds int           `json:"retry_after_seconds,omitempty"`
	Usage             *UsageWindows `json:"usage,omitempty"`
	UsageError        string        `json:"usage_error,omitempty"`
}

type KeysReportResponse struct {
	GeneratedAt        string      `json:"generated_at"`
	UsageCachedSeconds int         `json:"usage_cached_seconds"`
	Keys               []KeyReport `json:"keys"`
}

type usageEntry struct {
	at    time.Time
	usage *UsageWindows
	err   string
}

// usageCache holds the last usage lookup per key index. The mutex is held while
// refreshing so concurrent dashboards trigger a single upstream round.
type usageCache struct {
	mu      sync.Mutex
	entries map[int]usageEntry
	now     func() time.Time
	client  *http.Client
}

func newUsageCache() *usageCache {
	return &usageCache{
		entries: map[int]usageEntry{},
		now:     time.Now,
		// Never follow redirects: Go would resend the Authorization header.
		client: &http.Client{Timeout: usageTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// usageFor returns usage for every key, querying only stale entries, in parallel.
func (a *App) usageFor(ctx context.Context) []usageEntry {
	c := a.usage
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := a.config.UpstreamAPIKeys
	now := c.now()
	var wg sync.WaitGroup
	fresh := make([]*usageEntry, len(keys)) // one slot per goroutine: no shared writes
	for i, key := range keys {
		if e, ok := c.entries[i]; ok && now.Sub(e.at) < a.config.UsageCacheTTL {
			continue
		}
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			u, errCode := a.fetchUsage(ctx, key)
			if errCode != "" {
				log.Printf("usage query failed key_index=%d key_name=%q error=%s", i, a.config.keyName(i), errCode)
			}
			fresh[i] = &usageEntry{at: now, usage: u, err: errCode}
		}(i, key)
	}
	wg.Wait()
	// A cancelled request yields cancel errors: show them once, never cache them for the TTL.
	if ctx.Err() == nil {
		for i, e := range fresh {
			if e != nil {
				c.entries[i] = *e
			}
		}
	}
	out := make([]usageEntry, len(keys))
	for i := range keys {
		out[i] = c.entries[i]
		if fresh[i] != nil {
			out[i] = *fresh[i]
		}
	}
	return out
}

// fetchUsage returns windows or a short error code (HTTP status or error type).
// It never includes the key or the underlying error text.
func (a *App) fetchUsage(ctx context.Context, key string) (*UsageWindows, string) {
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultString(a.config.UsageURL, defaultUsageURL), nil)
	if err != nil {
		return nil, "invalid_request"
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "switchboard-go")
	resp, err := a.usage.client.Do(req)
	if err != nil {
		var ue *url.Error
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return nil, "timeout"
		case errors.As(err, &ue):
			return nil, fmt.Sprintf("%T", ue.Err)
		}
		return nil, fmt.Sprintf("%T", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	// Upstream shape (same as the Python reference): {"usage": {"rolling": {"percent", "resetsAt"}, ...}}.
	var raw struct {
		Usage map[string]struct {
			Percent  *float64        `json:"percent"`
			ResetsAt json.RawMessage `json:"resetsAt"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, "invalid_response"
	}
	var u UsageWindows
	for name, dst := range map[string]**UsageWindow{"rolling": &u.Rolling, "weekly": &u.Weekly, "monthly": &u.Monthly} {
		if w, ok := raw.Usage[name]; ok && w.Percent != nil {
			*dst = &UsageWindow{Percent: *w.Percent, ResetsAt: w.ResetsAt}
		}
	}
	if u == (UsageWindows{}) {
		return nil, "no_windows"
	}
	return &u, ""
}

func (a *App) buildKeysReport(ctx context.Context) KeysReportResponse {
	st := a.keys.Status()
	usage := a.usageFor(ctx)
	out := KeysReportResponse{GeneratedAt: time.Now().UTC().Format(time.RFC3339), UsageCachedSeconds: int(a.config.UsageCacheTTL / time.Second), Keys: make([]KeyReport, len(st.Keys))}
	for i, k := range st.Keys {
		out.Keys[i] = KeyReport{Index: k.Index, Name: a.config.keyName(i), State: k.State, Current: k.Current, Eligible: k.Eligible, Last429Time: k.Last429Time, RetryAfterSeconds: k.RetryAfterSeconds, Usage: usage[i].usage, UsageError: usage[i].err}
	}
	return out
}

func (a *App) handleKeysReport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.buildKeysReport(r.Context()))
}

// statusTmpl renders /status. html/template escapes names and usage errors.
var statusTmpl = template.Must(template.New("status").Funcs(template.FuncMap{
	"label": func(k KeyReport) string {
		if k.Name != "" {
			return k.Name
		}
		return fmt.Sprintf("#%d", k.Index+1)
	},
	"pct": func(w *UsageWindow) string {
		if w == nil {
			return "—"
		}
		return fmt.Sprintf("%.1f%%", w.Percent)
	},
}).Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Status das chaves</title>
<style>body{font:14px system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}th,td{padding:.35rem .8rem;border-bottom:1px solid #ddd;text-align:left}</style></head>
<body><h1>Status das chaves</h1><p>Gerado em {{.GeneratedAt}}; uso em cache por {{.UsageCachedSeconds}}s.</p>
<table><tr><th>Chave</th><th>Estado</th><th>Cooldown</th><th>Rolling</th><th>Weekly</th><th>Monthly</th></tr>
{{range .Keys}}<tr><td>{{label .}}{{if .Current}} (atual){{end}}</td><td>{{.State}}</td><td>{{if .RetryAfterSeconds}}{{.RetryAfterSeconds}}s{{else}}—{{end}}</td>
{{if .UsageError}}<td colspan="3">erro de uso: {{.UsageError}}</td>{{else}}<td>{{pct .Usage.Rolling}}</td><td>{{pct .Usage.Weekly}}</td><td>{{pct .Usage.Monthly}}</td>{{end}}</tr>
{{end}}</table></body></html>
`))

func (a *App) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := statusTmpl.Execute(&buf, a.buildKeysReport(r.Context())); err != nil {
		log.Printf("status page render failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}
