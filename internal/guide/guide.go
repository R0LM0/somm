package guide

import (
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/R0LM0/somm/v2/internal/profile"
)

// guia_gentle_ai.md is kept embedded as the gentle-ai preset's reference
// doc (demoted from the live criteria source — see ExtractFromProfile,
// which is what backs the get_agent_criteria tool now).
//
//go:embed guia_gentle_ai.md
var embeddedGuide []byte

var guideContent = normalizeLineEndings(string(embeddedGuide))

// Extract returns the embedded guide section for the requested agent.
// An empty agent returns the full guide. Supported lookups are exact, or via
// the sdd-, review-, or jd- prefixes used in the heading names.
func Extract(agent string) (string, error) {
	if strings.TrimSpace(agent) == "" {
		return guideContent, nil
	}

	needle := strings.ToLower(strings.TrimSpace(agent))
	lines := strings.Split(guideContent, "\n")

	for i, line := range lines {
		if !isHeading(line) {
			continue
		}
		heading := headingText(line)
		if heading == "" {
			continue
		}
		if matchesAgent(needle, heading) {
			return extractFrom(i, lines), nil
		}
	}

	return "", errors.New(fmt.Sprintf(`Agent %q not found in guia_gentle_ai.md. Available agents: %s`, agent, strings.Join(availableAgents(lines), ", ")))
}

func isHeading(line string) bool {
	return strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "## ")
}

func headingText(line string) string {
	line = strings.TrimSpace(line)
	var body string
	if strings.HasPrefix(line, "### ") {
		body = line[4:]
	} else if strings.HasPrefix(line, "## ") {
		body = line[3:]
	} else {
		return ""
	}
	// The heading name is the first whitespace-separated token; anything after
	// a space is descriptive text (e.g. "### review-risk (R1 ...").
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func matchesAgent(needle, heading string) bool {
	h := strings.ToLower(heading)
	if h == needle {
		return true
	}
	for _, prefix := range []string{"sdd-", "review-", "jd-"} {
		if h == prefix+needle {
			return true
		}
	}
	return false
}

func extractFrom(start int, lines []string) string {
	var sb strings.Builder
	for i := start; i < len(lines); i++ {
		if i > start && (strings.HasPrefix(lines[i], "### ") || strings.HasPrefix(lines[i], "## ")) {
			break
		}
		if i > start {
			sb.WriteByte('\n')
		}
		sb.WriteString(lines[i])
	}
	return sb.String()
}

func availableAgents(lines []string) []string {
	seen := make(map[string]bool)
	var agents []string
	for _, line := range lines {
		if !isHeading(line) {
			continue
		}
		name := headingText(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		agents = append(agents, name)
	}
	sort.Strings(agents)
	return agents
}

func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// ExtractFromProfile returns the criteria text for the requested role,
// sourced from the active Profile's roles instead of the hardcoded
// guia_gentle_ai.md guide. An empty agent returns every role's criteria,
// in profile order. Lookups match the role id case-insensitively.
func ExtractFromProfile(prof *profile.Profile, agent string) (string, error) {
	if prof == nil || len(prof.Roles) == 0 {
		return "", errors.New("no active profile roles available")
	}

	if strings.TrimSpace(agent) == "" {
		var sb strings.Builder
		for i, r := range prof.Roles {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(formatRoleCriteria(r))
		}
		return sb.String(), nil
	}

	needle := strings.ToLower(strings.TrimSpace(agent))
	for _, r := range prof.Roles {
		if strings.ToLower(r.ID) == needle {
			return formatRoleCriteria(r), nil
		}
	}

	ids := make([]string, len(prof.Roles))
	for i, r := range prof.Roles {
		ids[i] = r.ID
	}
	sort.Strings(ids)
	return "", fmt.Errorf("agent %q not found in the active profile. Available agents: %s", agent, strings.Join(ids, ", "))
}

// formatRoleCriteria renders a single role's criteria as a text block.
func formatRoleCriteria(r profile.Role) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s\n\n", r.ID))
	if r.Criticidad != "" {
		sb.WriteString(fmt.Sprintf("**Criticidad: %s**\n\n", r.Criticidad))
	}
	if r.Description != "" {
		sb.WriteString(r.Description)
	}
	return sb.String()
}
