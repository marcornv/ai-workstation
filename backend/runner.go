package main

// runner.go: running an experiment.
//
// A run is every (input row × enabled model × repetition) combination. Each one
// is a "job". A fixed number of goroutines (the "concurrency" setting) take jobs
// from a channel, call the model and save the answer. The UI polls the run to
// show progress.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type job struct {
	inputIndex int
	repetition int
	model      ModelConfig
	provider   ProviderConfig
	userPrompt string
	reference  string
	context    string
	images     []string // image paths/URLs from the row's image columns
}

// Runner keeps track of runs in progress so they can be canceled.
type Runner struct {
	db      *sql.DB
	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
}

func newRunner(db *sql.DB) *Runner {
	return &Runner{db: db, cancels: map[int64]context.CancelFunc{}}
}

// checkExperiment finds problems that would make a run fail, in plain Portuguese.
// It returns the dataset so callers don't need to load it twice.
func checkExperiment(cfg Config, exp Experiment) (Dataset, error) {
	enabled := enabledModels(exp)
	if len(enabled) == 0 {
		return Dataset{}, errors.New(tr("ative pelo menos um modelo", "enable at least one model"))
	}
	for _, m := range enabled {
		p, ok := cfg.Providers[m.Provider]
		if !ok {
			return Dataset{}, fmt.Errorf(tr("o modelo %s usa o provedor %q, que não existe nas configurações", "model %s uses provider %q, which does not exist in the settings"), m.Model, m.Provider)
		}
		if p.APIKeyEnv != "" && apiKey(p) == "" {
			return Dataset{}, fmt.Errorf(tr("chave não configurada para %s: adicione-a em Configurações", "no API key for %s: add it in Settings"), m.Provider)
		}
		if p.Kind == "typesafe" {
			return Dataset{}, fmt.Errorf(tr("%s é um modelo de decisão: use-o em um avaliador “Motor de decisão”, não como modelo do experimento", "%s is a decision model: use it in a “Decision engine” evaluator, not as an experiment model"), m.Model)
		}
	}
	if strings.TrimSpace(exp.Prompt.User) == "" {
		return Dataset{}, errors.New(tr("escreva a mensagem do usuário no prompt", "write the user message in the prompt"))
	}
	ds, err := loadDataset(cfg, exp.Dataset.File)
	if err != nil {
		return Dataset{}, err
	}
	known := map[string]bool{}
	for _, c := range ds.Columns {
		known[c] = true
	}
	for _, col := range append(templateColumns(exp.Prompt.System), templateColumns(exp.Prompt.User)...) {
		if !known[col] {
			return Dataset{}, fmt.Errorf(tr("o prompt usa {%s}, mas essa coluna não existe em %s (colunas: %s)", "the prompt uses {%s}, but that column does not exist in %s (columns: %s)"),
				col, ds.File, strings.Join(ds.Columns, ", "))
		}
	}
	for _, col := range exp.Dataset.ImageColumns {
		if !known[col] {
			return Dataset{}, fmt.Errorf(tr("a coluna de imagens %q não existe em %s", "image column %q does not exist in %s"), col, ds.File)
		}
	}
	// Find missing image files now, instead of failing call by call later.
	for i, row := range ds.Rows {
		for _, ref := range imageRefs(row, exp.Dataset.ImageColumns) {
			if err := checkImageRef(cfg, ref); err != nil {
				return Dataset{}, fmt.Errorf(tr("linha %d: %w", "line %d: %w"), i+1, err)
			}
		}
	}
	return ds, nil
}

func enabledModels(exp Experiment) []ModelConfig {
	var out []ModelConfig
	for _, m := range exp.Models {
		if m.Enabled {
			out = append(out, m)
		}
	}
	return out
}

// buildJobs creates one job per (row, model, repetition).
func buildJobs(cfg Config, exp Experiment, rows []map[string]string) []job {
	var jobs []job
	for i, row := range rows {
		for _, m := range enabledModels(exp) {
			for rep := 1; rep <= exp.Run.Repetitions; rep++ {
				jobs = append(jobs, job{
					inputIndex: i,
					repetition: rep,
					model:      m,
					provider:   cfg.Providers[m.Provider],
					userPrompt: fillTemplate(exp.Prompt.User, row),
					reference:  row[exp.Dataset.ReferenceColumn],
					context:    row[exp.Dataset.ContextColumn],
					images:     imageRefs(row, exp.Dataset.ImageColumns),
				})
			}
		}
	}
	return jobs
}

// start validates the experiment, records a new run and processes it in the
// background. limit > 0 runs only the first `limit` inputs ("test with 1 input").
func (r *Runner) start(cfg Config, exp Experiment, limit int) (int64, error) {
	ds, err := checkExperiment(cfg, exp)
	if err != nil {
		return 0, err
	}
	rows := ds.Rows
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	jobs := buildJobs(cfg, exp, rows)

	snapshot, _ := json.Marshal(exp)
	runID, err := insertRun(r.db, exp.ID, len(jobs), string(snapshot))
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancels[runID] = cancel
	r.mu.Unlock()

	go r.process(ctx, runID, cfg, exp, jobs)
	return runID, nil
}

func (r *Runner) process(ctx context.Context, runID int64, cfg Config, exp Experiment, jobs []job) {
	defer func() {
		r.mu.Lock()
		delete(r.cancels, runID)
		r.mu.Unlock()
	}()

	queue := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < exp.Run.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				r.runJob(ctx, runID, cfg, exp, j)
			}
		}()
	}

	for _, j := range jobs {
		select {
		case queue <- j:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(queue)
	wg.Wait()

	status := "done"
	if ctx.Err() != nil {
		status = "canceled"
	}
	if err := finishRun(r.db, runID, status); err != nil {
		log.Printf("run %d: could not mark as finished: %v", runID, err)
	}
}

func (r *Runner) runJob(ctx context.Context, runID int64, cfg Config, exp Experiment, j job) {
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(exp.Run.TimeoutS)*time.Second)
	defer cancel()

	var images []Image
	var err error
	for _, ref := range j.images {
		var img Image
		if img, err = loadImage(callCtx, cfg, ref); err != nil {
			break
		}
		images = append(images, img)
	}

	var resp LLMResponse
	if err == nil {
		resp, err = callLLM(callCtx, j.provider, LLMRequest{
			Model:       j.model.Model,
			System:      exp.Prompt.System,
			User:        j.userPrompt,
			Temperature: j.model.Temperature,
			MaxTokens:   j.model.MaxTokens,
			Images:      images,
		})
	}
	if ctx.Err() != nil {
		return // the whole run was canceled: don't save half-finished calls
	}

	result := Result{
		RunID:        runID,
		InputIndex:   j.inputIndex,
		Repetition:   j.repetition,
		Provider:     j.model.Provider,
		Model:        j.model.Model,
		UserPrompt:   j.userPrompt,
		Reference:    j.reference,
		Context:      j.context,
		Images:       j.images,
		Output:       resp.Text,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		LatencyMs:    resp.LatencyMs,
		CostUSD:      callCost(cfg, j.provider, j.model.Model, resp.InputTokens, resp.OutputTokens),
	}
	if err != nil {
		result.Error = err.Error()
	}
	if err := insertResult(r.db, result); err != nil {
		log.Printf("run %d: could not save result: %v", runID, err)
	}
}

func (r *Runner) cancel(runID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.cancels[runID]
	if ok {
		cancel()
	}
	return ok
}
