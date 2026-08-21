package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// DiscoveredModel is one model reported by the local opencode CLI. Prices are
// USD per 1M tokens (design D7), matching internal/profile/plans.Price's
// *PerM convention — not api.Money's per-token convention.
type DiscoveredModel struct {
	ProviderID    string
	ID            string
	Name          string
	InputPerM     float64
	OutputPerM    float64
	CacheReadPerM *float64
	ContextLength *int64
}

// ProviderDiscoverer reports the models served by providers configured on
// this host (default implementation: the local opencode CLI, design D1/D2).
// Every error is non-fatal: callers warn and continue with the existing
// catalog (design D4).
type ProviderDiscoverer interface {
	Discover(ctx context.Context) ([]DiscoveredModel, error)
}

// discoverCostCache is the "cost.cache" object reported per model:
// cache-read/write prices, USD per 1M tokens.
type discoverCostCache struct {
	Read  float64 `json:"read"`
	Write float64 `json:"write"`
}

// discoverCost is the "cost" object reported per model. A null cost is a
// per-entry drop signal (design D4) — usage-based pricing is unavailable for
// that model.
type discoverCost struct {
	Input  float64            `json:"input"`
	Output float64            `json:"output"`
	Cache  *discoverCostCache `json:"cache,omitempty"`
}

// discoverLimit is the "limit" object reported per model; only Context is
// consumed today.
type discoverLimit struct {
	Context *int64 `json:"context,omitempty"`
}

// discoverRecord is the raw per-model JSON shape emitted by
// `opencode models --verbose` (design D1). Unknown extra fields decode and
// are silently ignored — this is encoding/json's default behavior; no
// DisallowUnknownFields call is made (threat matrix: PATH hijack).
type discoverRecord struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"providerID"`
	Name       string         `json:"name"`
	Cost       *discoverCost  `json:"cost"`
	Limit      *discoverLimit `json:"limit,omitempty"`
}

// parseDiscoverOutput decodes the raw stdout of `opencode models --verbose`
// into []DiscoveredModel.
//
// The real output is NOT a JSON array or an NDJSON document (verified
// against a live `opencode models --verbose` capture): it is a
// `providerID/id` label line followed by a pretty-printed JSON object,
// repeated per model, with no enclosing envelope at all. splitDiscoverBlocks
// extracts each top-level object, discarding the label lines between them.
//
// An input that ends mid-object (truncated stdout — depth never returns to
// zero) returns an error; the caller (execDiscoverer, Phase 3) treats it as
// non-fatal and continues with the existing catalog (design D4). Otherwise,
// a per-block malformed record — invalid JSON body, missing id/providerID,
// or a null cost — is dropped individually; it never fails the whole parse
// (design D4's one exception to the uniform failure taxonomy). Input with no
// recognizable object at all (e.g. an unexpected CLI output format) yields
// an empty result, not an error — there is no envelope to validate against.
func parseDiscoverOutput(raw []byte) ([]DiscoveredModel, error) {
	blocks, truncated := splitDiscoverBlocks(raw)
	if truncated {
		return nil, fmt.Errorf("parsing discover output: truncated JSON object (stdout cut off mid-record)")
	}

	models := make([]DiscoveredModel, 0, len(blocks))
	for _, block := range blocks {
		var rec discoverRecord
		if err := json.Unmarshal(block, &rec); err != nil {
			continue // per-entry drop: malformed object body (design D4)
		}
		if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.ProviderID) == "" {
			continue
		}
		if rec.Cost == nil {
			continue
		}

		dm := DiscoveredModel{
			ProviderID: rec.ProviderID,
			ID:         rec.ID,
			Name:       rec.Name,
			InputPerM:  rec.Cost.Input,
			OutputPerM: rec.Cost.Output,
		}
		if rec.Cost.Cache != nil {
			read := rec.Cost.Cache.Read
			dm.CacheReadPerM = &read
		}
		if rec.Limit != nil && rec.Limit.Context != nil {
			ctxLen := *rec.Limit.Context
			dm.ContextLength = &ctxLen
		}
		models = append(models, dm)
	}
	return models, nil
}

// splitDiscoverBlocks extracts each top-level JSON object from raw stdout
// that interleaves non-JSON label lines with JSON objects (single-line or
// pretty-printed). A line is treated as the start of an object once it
// begins (after trimming) with `{`; brace-depth counting per line — not a
// full JSON tokenizer — determines where that object ends. This is
// sufficient because ids/names/providerIDs in this schema never contain
// literal `{`/`}` characters (a hostile field containing one would only
// mis-split that one block, which then fails to unmarshal and is dropped by
// the per-entry gate above, not silently misread).
//
// truncated is true when the input ends with an object that never returned
// to brace-depth zero (stdout cut off mid-record) — the one case that must
// surface as an error rather than degrade silently (design D4).
func splitDiscoverBlocks(raw []byte) (blocks [][]byte, truncated bool) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20) // 8 MiB guard (design threat matrix)

	var buf bytes.Buffer
	depth := 0
	inObject := false
	for scanner.Scan() {
		line := scanner.Text()
		if !inObject {
			if !strings.HasPrefix(strings.TrimSpace(line), "{") {
				continue // label line or blank — not the start of an object
			}
			inObject = true
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth <= 0 {
			blocks = append(blocks, append([]byte(nil), buf.Bytes()...))
			buf.Reset()
			inObject = false
			depth = 0
		}
	}
	return blocks, inObject
}

// Discovery constants (design D1, D3; threat matrix).
const (
	// discoverTimeout bounds one `opencode models --verbose` invocation.
	// exec.CommandContext kills and reaps the process on deadline (threat
	// matrix: hang/no exit).
	discoverTimeout = 5 * time.Second
	// discoverMaxStdout caps stdout read from the subprocess. Over-limit
	// output is treated as malformed, never buffered in full (threat
	// matrix: unbounded stdout).
	discoverMaxStdout = 8 << 20 // 8 MiB
	// discoverMaxStderr bounds how much stderr is retained for logging on a
	// non-zero exit (design task 3.4) — never logged in full.
	discoverMaxStderr = 512 // bytes
	// discoverSuccessTTL/discoverFailureTTL bound the in-process cache
	// (design D3): a success is trusted longer than a failure, so a
	// provider authenticated mid-session is picked up within a minute.
	discoverSuccessTTL = 5 * time.Minute
	discoverFailureTTL = 60 * time.Second
)

// discoverDisabledEnv/discoverRefreshEnv are the escape hatches documented
// in design D1 and the Migration/Rollout section: SOMM_DISCOVERY=off is the
// rollback path (never invokes the subprocess), SOMM_OC_DISCOVERY_REFRESH=1
// appends --refresh to the invocation.
const (
	discoverDisabledEnv = "SOMM_DISCOVERY"
	discoverRefreshEnv  = "SOMM_OC_DISCOVERY_REFRESH"
)

// discoverCall represents one in-flight `opencode models --verbose`
// invocation shared by every concurrent Discover caller within the same
// window (threat matrix: concurrent tool calls — at most one subprocess).
type discoverCall struct {
	done   chan struct{}
	models []DiscoveredModel
	err    error
}

// execDiscoverer is the default ProviderDiscoverer (design D1/D2): it shells
// out to the local `opencode` CLI. Results are cached in-process with a TTL
// (design D3) and single-flighted so concurrent Discover calls share one
// subprocess invocation instead of spawning N.
type execDiscoverer struct {
	// runner executes `opencode models --verbose` (or an injected fake in
	// tests) and returns raw stdout bytes. It receives a context already
	// bounded by timeout — the design's Testing Strategy test seam
	// (Unit(subprocess)).
	runner func(ctx context.Context) ([]byte, error)
	// timeout bounds one invocation; defaults to discoverTimeout in
	// newExecDiscoverer, overridable in tests so timeout/hang tests don't
	// wait out the real 5s (threat matrix: hang/no exit).
	timeout time.Duration

	mu        sync.Mutex
	cachedAt  time.Time
	cached    []DiscoveredModel
	cachedErr error
	inFlight  *discoverCall
}

// newExecDiscoverer builds the default ProviderDiscoverer, wired to the real
// opencode CLI via runOpencodeDiscover.
func newExecDiscoverer() *execDiscoverer {
	return &execDiscoverer{
		runner:  runOpencodeDiscover,
		timeout: discoverTimeout,
	}
}

// Discover reports the models served by providers configured on this host.
// SOMM_DISCOVERY=off is the rollback path: it short-circuits to a no-op
// without ever invoking the subprocess (design Migration/Rollout). Every
// failure — binary absent, non-zero exit, malformed/oversize output, timeout
// — collapses to one slog.Warn and a non-fatal (nil, error) return (design
// D4); the caller decides how to degrade.
func (d *execDiscoverer) Discover(ctx context.Context) ([]DiscoveredModel, error) {
	if os.Getenv(discoverDisabledEnv) == "off" {
		return nil, nil
	}

	d.mu.Lock()
	if !d.cachedAt.IsZero() {
		ttl := discoverSuccessTTL
		if d.cachedErr != nil {
			ttl = discoverFailureTTL
		}
		if time.Since(d.cachedAt) < ttl {
			models, err := d.cached, d.cachedErr
			d.mu.Unlock()
			return models, err
		}
	}
	if call := d.inFlight; call != nil {
		d.mu.Unlock()
		<-call.done
		return call.models, call.err
	}

	call := &discoverCall{done: make(chan struct{})}
	d.inFlight = call
	d.mu.Unlock()

	models, err := d.invoke(ctx)

	d.mu.Lock()
	d.cached, d.cachedErr, d.cachedAt = models, err, time.Now()
	d.inFlight = nil
	d.mu.Unlock()

	call.models, call.err = models, err
	close(call.done)
	return models, err
}

// invoke runs the subprocess (real or injected), applies the stdout size
// guard, and parses the result. Every error path warns and returns non-fatal
// (nil, error) — never a panic, never a fatal error (design D4).
func (d *execDiscoverer) invoke(ctx context.Context) ([]DiscoveredModel, error) {
	timeout := d.timeout
	if timeout <= 0 {
		timeout = discoverTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := d.runner(runCtx)
	if err != nil {
		slog.Warn("opencode discovery failed, continuing with existing catalog", "error", err)
		return nil, err
	}
	if len(raw) > discoverMaxStdout {
		err := fmt.Errorf("opencode discovery: stdout exceeded %d byte limit, treating as malformed", discoverMaxStdout)
		slog.Warn("opencode discovery failed, continuing with existing catalog", "error", err)
		return nil, err
	}

	models, err := parseDiscoverOutput(raw)
	if err != nil {
		slog.Warn("opencode discovery failed, continuing with existing catalog", "error", err)
		return nil, err
	}
	return models, nil
}

// buildDiscoverArgv returns the fixed argv for the opencode invocation
// (threat matrix: binary resolution — no user input is ever interpolated
// into it). The only variation is the SOMM_OC_DISCOVERY_REFRESH=1 escape
// hatch (design D1), itself a fixed literal flag, not an interpolated value.
func buildDiscoverArgv() []string {
	argv := []string{"models", "--verbose"}
	if os.Getenv(discoverRefreshEnv) == "1" {
		argv = append(argv, "--refresh")
	}
	return argv
}

// truncateStderr bounds captured stderr to limit bytes before it is ever
// logged (design task 3.4) — stderr from an arbitrary subprocess must never
// be logged or propagated in full.
func truncateStderr(b []byte, limit int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

// runOpencodeDiscover is the real ProviderDiscoverer runner: it resolves
// `opencode` via exec.LookPath only (never a shell, never a user-supplied
// path) and runs it with a fixed argv via exec.CommandContext, whose
// deadline kills and reaps the process (threat matrix: binary resolution,
// hang/no exit).
func runOpencodeDiscover(ctx context.Context) ([]byte, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return nil, fmt.Errorf("opencode binary not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, path, buildDiscoverArgv()...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opencode discovery: creating stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("opencode discovery: starting process: %w", err)
	}

	// Read at most discoverMaxStdout+1 bytes: reading one byte past the cap
	// lets the caller detect truncation (len > discoverMaxStdout) without
	// ever buffering unbounded subprocess output (threat matrix).
	stdout, readErr := io.ReadAll(io.LimitReader(stdoutPipe, discoverMaxStdout+1))
	waitErr := cmd.Wait()

	if waitErr != nil {
		return nil, fmt.Errorf("opencode discovery: process exited with error: %w (stderr: %s)", waitErr, truncateStderr(stderrBuf.Bytes(), discoverMaxStderr))
	}
	if readErr != nil {
		return nil, fmt.Errorf("opencode discovery: reading stdout: %w", readErr)
	}
	return stdout, nil
}

// defaultDiscoverer is the package-level execDiscoverer singleton used by
// Client.discoverer() when Client.Discoverer is nil. Sharing one instance
// (rather than constructing one per Client/call) lets the in-process TTL
// cache and single-flight (design D3) apply across every ListModels call
// made by this process, not just within one Client.
var defaultDiscoverer ProviderDiscoverer = newExecDiscoverer()

// noopDiscoverer reports zero providers and never errors. It is the resolved
// default under `go test` (see Client.discoverer): the design's own comment
// on Client.Discoverer says "tests inject a fake or a no-op", but this repo
// has many existing Client{} literals across several test files (validate,
// cost, recommend, export) built before discovery existed and that never set
// Discoverer explicitly. Falling back to noopDiscoverer under testing.Testing
// keeps every one of those pre-existing tests hermetic and deterministic —
// never spawning a real subprocess or depending on this host's opencode
// install/auth state — without having to touch every one of those files in
// this atomic slice; tests that specifically exercise discovery (this file's
// TestListModels_*, TestMergeDiscovered_*) inject an explicit fakeDiscoverer
// and take precedence over this fallback.
type noopDiscoverer struct{}

func (noopDiscoverer) Discover(ctx context.Context) ([]DiscoveredModel, error) {
	return nil, nil
}

// discoverer resolves the ProviderDiscoverer for a ListModels call: an
// explicitly-set Client.Discoverer always wins; otherwise it is the real
// default execDiscoverer in production, or the safe noopDiscoverer under
// `go test` (see noopDiscoverer's doc comment).
func (c *Client) discoverer() ProviderDiscoverer {
	if c.Discoverer != nil {
		return c.Discoverer
	}
	if testing.Testing() {
		return noopDiscoverer{}
	}
	return defaultDiscoverer
}
