package api

import (
	"regexp"
	"strings"
	"unicode"
)

// ProviderAlias maps an OpenCode model ID prefix to the corresponding
// OpenRouter provider prefix. The slice is ordered and intentionally not a
// map so iteration is deterministic.
var PROVIDER_ALIASES = []struct {
	Prefix      string
	Replacement string
}{
	{Prefix: "kimi-", Replacement: "moonshotai/"},
	{Prefix: "gpt-", Replacement: "openai/gpt-"},
	{Prefix: "gemini-", Replacement: "google/gemini-"},
	{Prefix: "grok-", Replacement: "x-ai/grok-"},
	{Prefix: "deepseek-", Replacement: "deepseek/"},
	{Prefix: "qwen", Replacement: "qwen/"},
	{Prefix: "glm-", Replacement: "z-ai/glm-"},
	{Prefix: "mimo-", Replacement: "xiaomi/mimo-"},
	{Prefix: "minimax-", Replacement: "minimax/"},
	{Prefix: "hy3", Replacement: "tencent/hy3"},
	{Prefix: "nemotron-", Replacement: "nvidia/nemotron-"},
	{Prefix: "north-mini-", Replacement: "cohere/north-mini-"},
}

// providerDisplay keeps the ordered provider-name overrides used by
// deriveProvider. Order matters because the first matching prefix wins.
var providerDisplay = []struct {
	prefix  string
	display string
}{
	{prefix: "deepseek", display: "DeepSeek"},
	{prefix: "minimax", display: "MiniMax"},
	{prefix: "kimi", display: "Kimi"},
	{prefix: "qwen", display: "Qwen"},
	{prefix: "glm", display: "GLM"},
	{prefix: "grok", display: "Grok"},
	{prefix: "mimo", display: "MiMo"},
	{prefix: "hy3", display: "Hy3"},
	{prefix: "claude", display: "Anthropic"},
	{prefix: "gpt", display: "OpenAI"},
	{prefix: "llama", display: "Meta"},
	{prefix: "gemini", display: "Google"},
	{prefix: "mistral", display: "Mistral"},
	{prefix: "opencode", display: "OpenCode"},
	{prefix: "big", display: "Big"},
}

// deriveProvider returns a human-readable provider name for an OpenCode model ID.
// It matches the ordered prefix overrides and falls back to title-casing the
// first dash-separated token.
func deriveProvider(ocID string) string {
	lower := strings.ToLower(ocID)
	for _, pd := range providerDisplay {
		if strings.HasPrefix(lower, pd.prefix) {
			return pd.display
		}
	}
	first := ocID
	if i := strings.Index(first, "-"); i >= 0 {
		first = first[:i]
	}
	if first == "" {
		return ocID
	}
	return titleCaseFirst(first)
}

// deriveDisplayName returns a human-readable model name from an OpenCode ID.
func deriveDisplayName(ocID string) string {
	provider := deriveProvider(ocID)
	rest := reLeadingToken.ReplaceAllString(ocID, "")
	rest = strings.ReplaceAll(rest, "-", " ")
	rest = titleCaseWords(rest)
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return provider
	}
	return provider + " " + rest
}

var reLeadingToken = regexp.MustCompile(`^[a-zA-Z0-9]+-?`)
var reWordBoundary = regexp.MustCompile(`\b\w`)

func titleCaseFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	for i := 1; i < len(r); i++ {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}

func titleCaseWords(s string) string {
	return reWordBoundary.ReplaceAllStringFunc(s, func(token string) string {
		if token == "" {
			return token
		}
		r := []rune(token)
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	})
}
