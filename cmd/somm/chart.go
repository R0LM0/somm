package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/R0LM0/somm/v2/internal/api"
)

// runChart prints a console quality/price view of the OpenRouter catalog:
// by default, only the Pareto-optimal models (no cheaper model beats their
// chosen quality metric) are shown, marked with ★ — the same "green
// quadrant" idea as a cost-vs-intelligence scatter chart, rendered as a
// ranked list instead of a 2D plot.
func runChart(argv []string) {
	fs := flag.NewFlagSet("chart", flag.ContinueOnError)
	metric := fs.String("metric", "intelligence", "Métrica a graficar: intelligence, coding o agentic")
	filter := fs.String("provider", "", "Filtrar por proveedor o nombre de modelo (substring)")
	all := fs.Bool("all", false, "Mostrar todos los modelos, no solo los óptimos en calidad/precio")
	top := fs.Int("top", 40, "Máximo de modelos a listar con --all")
	if err := fs.Parse(argv); err != nil {
		return
	}

	label, ok := metricLabel(*metric)
	if !ok {
		fmt.Fprintf(os.Stderr, "❌ Métrica inválida: %s (usá intelligence, coding o agentic)\n", *metric)
		os.Exit(1)
	}

	// OpenRouter's model list is public; auth is only sent when a key is
	// set, so this works even without OPENCODE_API_KEY configured.
	client := api.NewClient(nil, "", os.Getenv("OPENROUTER_API_KEY"))
	models, err := client.ListORModels(context.Background(), *filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error consultando OpenRouter: %v\n", err)
		os.Exit(1)
	}

	rows := buildChartRows(models, *metric)
	if len(rows) == 0 {
		fmt.Println("No encontré modelos con precio y benchmarks disponibles para graficar.")
		return
	}
	markParetoFrontier(rows)

	toShow := rows
	if *all {
		if len(toShow) > *top {
			toShow = toShow[:*top]
		}
	} else {
		toShow = frontierOnly(rows)
	}

	fmt.Println(renderChart(toShow, label, *all, len(rows)))
}

type chartRow struct {
	name     string
	provider string
	value    float64
	price    float64
	frontier bool
}

func metricLabel(metric string) (string, bool) {
	switch metric {
	case "intelligence":
		return "Intelligence", true
	case "coding":
		return "Coding", true
	case "agentic":
		return "Agentic", true
	default:
		return "", false
	}
}

// buildChartRows extracts (name, provider, metric, price) for every model
// that has both a usable input price and a value for the requested metric,
// sorted by price ascending.
func buildChartRows(models []api.ORModel, metric string) []chartRow {
	var rows []chartRow
	for _, m := range models {
		if m.Pricing == nil {
			continue
		}
		price := api.ParseMoney(m.Pricing.Prompt) * 1_000_000
		if price <= 0 {
			continue
		}
		if m.Benchmarks == nil || m.Benchmarks.ArtificialAnalysis == nil {
			continue
		}

		aa := m.Benchmarks.ArtificialAnalysis
		var value *float64
		switch metric {
		case "intelligence":
			value = aa.IntelligenceIndex
		case "coding":
			value = aa.CodingIndex
		case "agentic":
			value = aa.AgenticIndex
		}
		if value == nil {
			continue
		}

		rows = append(rows, chartRow{
			name:     m.Name,
			provider: providerFromORID(m.ID),
			value:    *value,
			price:    price,
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].price < rows[j].price })
	return rows
}

// providerFromORID extracts the provider slug from an OpenRouter model ID
// (e.g. "anthropic/claude-opus-5" -> "Anthropic").
func providerFromORID(id string) string {
	provider := id
	if i := strings.Index(provider, "/"); i >= 0 {
		provider = provider[:i]
	}
	if provider == "" {
		return provider
	}
	r := []rune(provider)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// markParetoFrontier flags every row that improves on the best metric value
// seen so far while walking price-ascending — the classic skyline/Pareto
// frontier for "maximize quality, minimize price".
func markParetoFrontier(rows []chartRow) {
	runningMax := -1.0
	for i := range rows {
		if rows[i].value > runningMax {
			rows[i].frontier = true
			runningMax = rows[i].value
		}
	}
}

func frontierOnly(rows []chartRow) []chartRow {
	var out []chartRow
	for _, r := range rows {
		if r.frontier {
			out = append(out, r)
		}
	}
	return out
}

func renderChart(rows []chartRow, metric string, showedAll bool, totalConsidered int) string {
	var b strings.Builder

	if showedAll {
		b.WriteString(fmt.Sprintf("Mostrando %d de %d modelos con %s y precio disponibles.\n", len(rows), totalConsidered, metric))
	} else {
		b.WriteString(fmt.Sprintf("%d modelo(s) en la frontera de Pareto (mejor %s al menor precio), de %d considerados.\n", len(rows), metric, totalConsidered))
	}
	b.WriteString(successStyle.Render("★") + " = ningún modelo más barato tiene mejor " + strings.ToLower(metric) + "\n\n")

	nameW, provW := len("Modelo"), len("Proveedor")
	for _, r := range rows {
		if len(r.name) > nameW {
			nameW = len(r.name)
		}
		if len(r.provider) > provW {
			provW = len(r.provider)
		}
	}

	header := fmt.Sprintf("   %-*s  %-*s  %11s  %12s", nameW, "Modelo", provW, "Proveedor", metric, "$/1M input")
	b.WriteString(titleStyle.Render(header) + "\n")

	for _, r := range rows {
		marker := " "
		if r.frontier {
			marker = successStyle.Render("★")
		}
		b.WriteString(fmt.Sprintf("%s  %-*s  %-*s  %11.1f  $%11.3f\n", marker, nameW, r.name, provW, r.provider, r.value, r.price))
	}

	return boxStyle.Render(strings.TrimRight(b.String(), "\n"))
}
