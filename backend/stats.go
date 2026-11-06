package main

// stats.go: cost, estimates and per-model summaries.

import (
	"math"
	"sort"
)

// callCost returns the cost in US dollars of one call, or nil if the model has
// no price in bancada.yaml. Local models (Ollama) are free.
func callCost(cfg Config, provider ProviderConfig, model string, inTokens, outTokens int) *float64 {
	if provider.Kind == "ollama" {
		zero := 0.0
		return &zero
	}
	price, ok := cfg.Pricing[model]
	if !ok {
		return nil
	}
	cost := (float64(inTokens)*price.Input + float64(outTokens)*price.Output) / 1_000_000
	return &cost
}

// approxTokens is a rough rule of thumb: ~4 characters per token.
func approxTokens(text string) int {
	return (len([]rune(text)) + 3) / 4
}

// ---------- Estimate before running ----------

type Estimate struct {
	Inputs             int      `json:"inputs"`
	Calls              int      `json:"calls"`
	InputTokens        int      `json:"input_tokens"`
	OutputTokens       int      `json:"output_tokens"`
	CostUSD            float64  `json:"cost_usd"`
	PaidCalls          int      `json:"paid_calls"`           // calls that go to a paid API
	ModelsWithoutPrice []string `json:"models_without_price"` // cost unknown for these
}

// Output length is unknown before running, so we assume a typical answer size
// (capped by the model's max_tokens).
const assumedOutputTokens = 400

func estimateRun(cfg Config, exp Experiment, ds Dataset, limit int) Estimate {
	rows := ds.Rows
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	promptTokens := 0
	for _, row := range rows {
		promptTokens += approxTokens(exp.Prompt.System) + approxTokens(fillTemplate(exp.Prompt.User, row))
		promptTokens += approxImageTokens * len(imageRefs(row, exp.Dataset.ImageColumns))
	}

	est := Estimate{Inputs: len(rows), ModelsWithoutPrice: []string{}}
	for _, m := range enabledModels(exp) {
		reps := exp.Run.Repetitions
		calls := len(rows) * reps
		outPerCall := assumedOutputTokens
		if m.MaxTokens > 0 && m.MaxTokens < outPerCall {
			outPerCall = m.MaxTokens
		}
		in := promptTokens * reps
		out := outPerCall * calls

		est.Calls += calls
		est.InputTokens += in
		est.OutputTokens += out

		provider := cfg.Providers[m.Provider]
		if provider.Kind == "ollama" {
			continue
		}
		est.PaidCalls += calls
		if cost := callCost(cfg, provider, m.Model, in, out); cost != nil {
			est.CostUSD += *cost
		} else {
			est.ModelsWithoutPrice = append(est.ModelsWithoutPrice, m.Model)
		}
	}
	return est
}

// ---------- Summary after running ----------

type ModelSummary struct {
	Model        string   `json:"model"`
	Provider     string   `json:"provider"`
	Calls        int      `json:"calls"`
	Errors       int      `json:"errors"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	LatencyP50   int64    `json:"latency_p50_ms"`
	LatencyP95   int64    `json:"latency_p95_ms"`
	CostUSD      *float64 `json:"cost_usd"` // nil if any call had an unknown price
	MeanScore    *float64 `json:"mean_score"`
}

type RunSummary struct {
	Calls        int            `json:"calls"`
	Errors       int            `json:"errors"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	LatencyP50   int64          `json:"latency_p50_ms"`
	LatencyP95   int64          `json:"latency_p95_ms"`
	CostUSD      float64        `json:"cost_usd"`
	CostComplete bool           `json:"cost_complete"` // false if some model has no price
	Models       []ModelSummary `json:"models"`
}

func summarize(results []Result, scores []Score) RunSummary {
	// Mean score per result (average of all evaluators that produced a score).
	scoreSum := map[int64]float64{}
	scoreCount := map[int64]int{}
	for _, s := range scores {
		if s.Score != nil {
			scoreSum[s.ResultID] += *s.Score
			scoreCount[s.ResultID]++
		}
	}

	type acc struct {
		summary   ModelSummary
		latencies []int64
		costKnown bool
		cost      float64
		scoreSum  float64
		scoreN    int
	}
	byModel := map[string]*acc{}
	var order []string
	var allLatencies []int64
	total := RunSummary{CostComplete: true}

	for _, r := range results {
		key := r.Provider + "/" + r.Model
		a, ok := byModel[key]
		if !ok {
			a = &acc{summary: ModelSummary{Model: r.Model, Provider: r.Provider}, costKnown: true}
			byModel[key] = a
			order = append(order, key)
		}
		a.summary.Calls++
		a.summary.InputTokens += r.InputTokens
		a.summary.OutputTokens += r.OutputTokens
		total.Calls++
		total.InputTokens += r.InputTokens
		total.OutputTokens += r.OutputTokens
		if r.Error != "" {
			a.summary.Errors++
			total.Errors++
		} else {
			a.latencies = append(a.latencies, r.LatencyMs)
			allLatencies = append(allLatencies, r.LatencyMs)
		}
		if r.CostUSD == nil {
			a.costKnown = false
			total.CostComplete = false
		} else {
			a.cost += *r.CostUSD
			total.CostUSD += *r.CostUSD
		}
		if n := scoreCount[r.ID]; n > 0 {
			a.scoreSum += scoreSum[r.ID] / float64(n)
			a.scoreN++
		}
	}

	for _, key := range order {
		a := byModel[key]
		a.summary.LatencyP50 = percentile(a.latencies, 50)
		a.summary.LatencyP95 = percentile(a.latencies, 95)
		if a.costKnown {
			cost := a.cost
			a.summary.CostUSD = &cost
		}
		if a.scoreN > 0 {
			mean := a.scoreSum / float64(a.scoreN)
			a.summary.MeanScore = &mean
		}
		total.Models = append(total.Models, a.summary)
	}
	if total.Models == nil {
		total.Models = []ModelSummary{}
	}
	total.LatencyP50 = percentile(allLatencies, 50)
	total.LatencyP95 = percentile(allLatencies, 95)
	return total
}

// percentile uses the "nearest rank" method. Returns 0 for an empty list.
func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	rank = max(0, min(rank, len(sorted)-1))
	return sorted[rank]
}
