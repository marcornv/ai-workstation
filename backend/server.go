package main

// server.go: the HTTP API used by the frontend. All routes are registered in
// routes(); each handler is a plain function that reads files / the database
// and writes JSON.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type App struct {
	db     *sql.DB
	runner *Runner
	evals  *Evaluations
	// spreadsheet tests of evaluators (evaluator_tests.go)
	evalTests *EvaluatorTests
	web       fs.FS // the built frontend (may be empty during development)
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.handleHealth)

	mux.HandleFunc("GET /api/settings", a.handleGetSettings)
	mux.HandleFunc("PUT /api/settings/file", a.handlePutSettingsFile)
	mux.HandleFunc("PUT /api/settings/language", a.handlePutLanguage)
	mux.HandleFunc("PUT /api/providers/{name}", a.handlePutProvider)
	mux.HandleFunc("DELETE /api/providers/{name}", a.handleDeleteProvider)
	mux.HandleFunc("PUT /api/providers/{name}/key", a.handlePutProviderKey)
	mux.HandleFunc("POST /api/providers/{name}/test", a.handleTestProvider)
	mux.HandleFunc("GET /api/providers/{name}/models", a.handleProviderModels)
	mux.HandleFunc("POST /api/providers/{name}/pull", a.handlePullModel)

	mux.HandleFunc("GET /api/experiments", a.handleListExperiments)
	mux.HandleFunc("POST /api/experiments", a.handleCreateExperiment)
	mux.HandleFunc("GET /api/experiments/{id}", a.handleGetExperiment)
	mux.HandleFunc("PUT /api/experiments/{id}", a.handlePutExperiment)
	mux.HandleFunc("DELETE /api/experiments/{id}", a.handleDeleteExperiment)
	mux.HandleFunc("GET /api/experiments/{id}/file", a.handleGetExperimentFile)
	mux.HandleFunc("PUT /api/experiments/{id}/file", a.handlePutExperimentFile)
	mux.HandleFunc("POST /api/experiments/{id}/estimate", a.handleEstimate)
	mux.HandleFunc("GET /api/experiments/{id}/runs", a.handleListRuns)
	mux.HandleFunc("POST /api/experiments/{id}/runs", a.handleStartRun)

	mux.HandleFunc("GET /api/runs/{id}", a.handleGetRun)
	mux.HandleFunc("POST /api/runs/{id}/cancel", a.handleCancelRun)
	mux.HandleFunc("GET /api/runs/{id}/results", a.handleRunResults)
	mux.HandleFunc("GET /api/runs/{id}/export.csv", a.handleExportCSV)
	mux.HandleFunc("POST /api/runs/{id}/evaluate", a.handleEvaluateRun)
	mux.HandleFunc("GET /api/runs/{id}/evaluation", a.handleRunEvaluation)

	mux.HandleFunc("GET /api/datasets", a.handleListDatasets)
	mux.HandleFunc("POST /api/datasets", a.handleUploadDataset)
	mux.HandleFunc("POST /api/datasets/images", a.handleUploadImages)
	mux.HandleFunc("GET /api/datasets/{file}", a.handlePreviewDataset)
	mux.HandleFunc("GET /api/files/{path...}", a.handleDataFile)

	mux.HandleFunc("GET /api/evaluators", a.handleListEvaluators)
	mux.HandleFunc("POST /api/evaluators", a.handleCreateEvaluator)
	mux.HandleFunc("POST /api/evaluators/test", a.handleTestEvaluator)
	mux.HandleFunc("PUT /api/evaluators/{id}", a.handlePutEvaluator)
	mux.HandleFunc("DELETE /api/evaluators/{id}", a.handleDeleteEvaluator)
	mux.HandleFunc("GET /api/evaluators/{id}/tests", a.handleListEvaluatorTests)
	mux.HandleFunc("POST /api/evaluators/{id}/tests", a.handleStartEvaluatorTest)
	mux.HandleFunc("GET /api/evaluator-tests/{id}", a.handleGetEvaluatorTest)
	mux.HandleFunc("POST /api/evaluator-tests/{id}/cancel", a.handleCancelEvaluatorTest)
	mux.HandleFunc("DELETE /api/evaluator-tests/{id}", a.handleDeleteEvaluatorTest)
	mux.HandleFunc("GET /api/evaluator-tests/{id}/export.csv", a.handleExportEvaluatorTest)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, tr("rota não encontrada", "route not found"))
	})
	mux.HandleFunc("/", a.handleFrontend)
	return mux
}

// ---------- Small helpers ----------

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// writeError sends {"error": "..."}; the UI shows the message to the user.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, 10<<20)
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf(tr("dados inválidos: %w", "invalid data: %w"), err)
	}
	return nil
}

// mustConfig loads bancada.yaml or writes an error response.
func mustConfig(w http.ResponseWriter) (Config, bool) {
	cfg, err := loadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, tr("erro em bancada.yaml: ", "error in bancada.yaml: ")+err.Error())
		return Config{}, false
	}
	return cfg, true
}

func (a *App) mustExperiment(w http.ResponseWriter, r *http.Request, cfg Config) (Experiment, bool) {
	exp, err := loadExperiment(cfg, r.PathValue("id"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, tr("experimento não encontrado", "experiment not found"))
		return Experiment{}, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return Experiment{}, false
	}
	return exp, true
}

func (a *App) mustRun(w http.ResponseWriter, r *http.Request) (Run, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, tr("execução inválida", "invalid run"))
		return Run{}, false
	}
	run, err := getRun(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, tr("execução não encontrada", "run not found"))
		return Run{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return Run{}, false
	}
	return run, true
}

// ---------- Health ----------

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	storageOK := a.db.Ping() == nil
	writeJSON(w, http.StatusOK, map[string]any{
		"api":           true,
		"api_port":      cfg.Server.Port,
		"evaluator":     evaluatorOnline(cfg.Server.EvaluatorURL),
		"evaluator_url": cfg.Server.EvaluatorURL,
		"storage":       storageOK,
		"storage_path":  cfg.Paths.Database,
	})
}

// ---------- Settings & providers ----------

type providerView struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	APIKeyEnv string `json:"api_key_env"`
	BaseURL   string `json:"base_url"`
	KeySet    bool   `json:"key_set"`
	KeyMasked string `json:"key_masked"`
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	raw, _ := os.ReadFile(configFile)
	providers := []providerView{}
	for name, p := range cfg.Providers {
		key := apiKey(p)
		providers = append(providers, providerView{
			Name: name, Kind: p.Kind, APIKeyEnv: p.APIKeyEnv, BaseURL: p.BaseURL,
			KeySet: key != "", KeyMasked: maskKey(key),
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{
		"config":    cfg,
		"providers": providers,
		"file":      string(raw),
		"file_path": configFile,
	})
}

// handlePutSettingsFile saves bancada.yaml exactly as typed (comments kept),
// but only after checking it parses.
func (a *App) handlePutSettingsFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := parseConfig([]byte(body.YAML)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := writeFileAtomic(configFile, []byte(body.YAML), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.handleGetSettings(w, r)
}

// handlePutLanguage switches the interface language (pt-BR or en) in bancada.yaml.
func (a *App) handlePutLanguage(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if err := readJSON(r, &body); err != nil || !validLanguage(body.Language) {
		writeError(w, http.StatusBadRequest, tr("idioma inválido: use pt-BR ou en", "invalid language: use pt-BR or en"))
		return
	}
	cfg.Language = body.Language
	if err := saveConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	setLanguage(cfg.Language)
	a.handleGetSettings(w, r)
}

func (a *App) handlePutProvider(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !validID(name) {
		writeError(w, http.StatusBadRequest, tr("nome de provedor inválido: use letras minúsculas, números e hífen", "invalid provider name: use lowercase letters, numbers and hyphens"))
		return
	}
	var p ProviderConfig
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validProviderKind(p.Kind) {
		writeError(w, http.StatusBadRequest, tr("tipo de provedor desconhecido", "unknown provider kind"))
		return
	}
	if p.APIKeyEnv != "" && !validEnvName(p.APIKeyEnv) {
		writeError(w, http.StatusBadRequest, tr("nome de variável de ambiente inválido", "invalid environment variable name"))
		return
	}
	cfg.Providers[name] = p
	if err := saveConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.handleGetSettings(w, r)
}

func (a *App) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	delete(cfg.Providers, r.PathValue("name"))
	if err := saveConfig(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.handleGetSettings(w, r)
}

func (a *App) handlePutProviderKey(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	p, exists := cfg.Providers[r.PathValue("name")]
	if !exists {
		writeError(w, http.StatusNotFound, tr("provedor não encontrado", "provider not found"))
		return
	}
	if p.APIKeyEnv == "" {
		writeError(w, http.StatusBadRequest, tr("este provedor não usa chave (defina api_key_env primeiro)", "this provider does not use a key (set api_key_env first)"))
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := saveKey(p.APIKeyEnv, strings.TrimSpace(body.Key)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.handleGetSettings(w, r)
}

func (a *App) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	p, exists := cfg.Providers[r.PathValue("name")]
	if !exists {
		writeError(w, http.StatusNotFound, tr("provedor não encontrado", "provider not found"))
		return
	}
	if p.APIKeyEnv != "" && apiKey(p) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": tr("chave não configurada", "no API key set")})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	start := time.Now()
	models, err := listModels(ctx, p)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "ms": ms})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": ms, "models": len(models)})
}

func (a *App) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	p, exists := cfg.Providers[r.PathValue("name")]
	if !exists {
		writeError(w, http.StatusNotFound, tr("provedor não encontrado", "provider not found"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	models, err := listModels(ctx, p)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sort.Strings(models)
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// handlePullModel downloads a model into Ollama. Useful in Docker, where
// people can't run "ollama pull" themselves.
func (a *App) handlePullModel(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	p, exists := cfg.Providers[r.PathValue("name")]
	if !exists {
		writeError(w, http.StatusNotFound, tr("provedor não encontrado", "provider not found"))
		return
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Model) == "" {
		writeError(w, http.StatusBadRequest, tr("informe o nome do modelo", "enter the model name"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Minute) // big models take a while
	defer cancel()
	if err := pullOllamaModel(ctx, p, strings.TrimSpace(body.Model)); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- Experiments ----------

type experimentListItem struct {
	Experiment
	Inputs        int      `json:"inputs"`
	EnabledModels int      `json:"enabled_models"`
	LastRun       *Run     `json:"last_run"`
	MeanScore     *float64 `json:"mean_score"`
	Changed       bool     `json:"changed"` // file edited after the last run started
}

func (a *App) handleListExperiments(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	experiments, err := listExperiments(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := []experimentListItem{}
	for _, exp := range experiments {
		item := experimentListItem{Experiment: exp, EnabledModels: len(enabledModels(exp))}
		if ds, err := loadDataset(cfg, exp.Dataset.File); err == nil {
			item.Inputs = len(ds.Rows)
		}
		if runs, err := listRuns(a.db, exp.ID); err == nil && len(runs) > 0 {
			item.LastRun = &runs[0]
			item.Changed = a.changedSinceRun(cfg, exp.ID, runs[0])
			if results, err := listResults(a.db, runs[0].ID); err == nil {
				scores, _ := listScores(a.db, runs[0].ID)
				item.MeanScore = overallMean(summarize(results, scores))
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func overallMean(s RunSummary) *float64 {
	sum, n := 0.0, 0
	for _, m := range s.Models {
		if m.MeanScore != nil {
			sum += *m.MeanScore
			n++
		}
	}
	if n == 0 {
		return nil
	}
	mean := sum / float64(n)
	return &mean
}

func (a *App) changedSinceRun(cfg Config, id string, run Run) bool {
	info, err := os.Stat(experimentPath(cfg, id))
	if err != nil {
		return false
	}
	started, err := time.Parse(time.RFC3339, run.StartedAt)
	if err != nil {
		return false
	}
	return info.ModTime().After(started.Add(time.Second))
}

func (a *App) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var body struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		UseCase       string `json:"use_case"`
		DuplicateFrom string `json:"duplicate_from"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, tr("dê um nome ao experimento", "give the experiment a name"))
		return
	}

	var exp Experiment
	if body.DuplicateFrom != "" {
		source, err := loadExperiment(cfg, body.DuplicateFrom)
		if err != nil {
			writeError(w, http.StatusBadRequest, tr("não foi possível duplicar: ", "could not duplicate: ")+err.Error())
			return
		}
		exp = source
	} else {
		exp = Experiment{Prompt: Prompt{User: "{input}"}, Run: cfg.Defaults.Run}
	}
	exp.ID = uniqueID(cfg.Paths.Experiments, slugify(body.Name))
	exp.Name = strings.TrimSpace(body.Name)
	exp.Description = body.Description
	if body.UseCase != "" {
		exp.UseCase = body.UseCase
	}
	fillExperimentDefaults(cfg, &exp)
	if err := saveExperiment(cfg, exp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, exp)
}

func (a *App) handleGetExperiment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	changed := false
	if runs, err := listRuns(a.db, exp.ID); err == nil && len(runs) > 0 {
		changed = a.changedSinceRun(cfg, exp.ID, runs[0])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"experiment": exp,
		"file_path":  experimentPath(cfg, exp.ID),
		"changed":    changed,
	})
}

func (a *App) handlePutExperiment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	if _, ok := a.mustExperiment(w, r, cfg); !ok {
		return
	}
	var exp Experiment
	if err := readJSON(r, &exp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	exp.ID = r.PathValue("id") // the id is the file name and never changes
	fillExperimentDefaults(cfg, &exp)
	if err := saveExperiment(cfg, exp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.handleGetExperiment(w, r)
}

func (a *App) handleDeleteExperiment(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	if err := os.Remove(experimentPath(cfg, exp.ID)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	deleteRunsOfExperiment(a.db, exp.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleGetExperimentFile(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	raw, err := os.ReadFile(experimentPath(cfg, exp.ID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": string(raw), "path": experimentPath(cfg, exp.ID)})
}

func (a *App) handlePutExperimentFile(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	parsed, err := parseExperiment(cfg, exp.ID, []byte(body.YAML))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(parsed.Name) == "" {
		writeError(w, http.StatusBadRequest, tr("o campo name é obrigatório", "the name field is required"))
		return
	}
	if err := writeFileAtomic(experimentPath(cfg, exp.ID), []byte(body.YAML), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.handleGetExperiment(w, r)
}

func (a *App) handleEstimate(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	var body struct {
		Limit int `json:"limit"`
	}
	readJSON(r, &body) // an empty body is fine
	ds, err := checkExperiment(cfg, exp)
	if err != nil {
		// Not an HTTP error: the UI shows "what's missing" next to the estimate.
		writeJSON(w, http.StatusOK, map[string]any{"ready": false, "problem": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true, "estimate": estimateRun(cfg, exp, ds, body.Limit)})
}

// ---------- Runs ----------

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := listRuns(a.db, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (a *App) handleStartRun(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	exp, ok := a.mustExperiment(w, r, cfg)
	if !ok {
		return
	}
	var body struct {
		Limit int `json:"limit"`
	}
	readJSON(r, &body)
	runID, err := a.runner.start(cfg, exp, body.Limit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"run_id": runID})
}

func (a *App) handleGetRun(w http.ResponseWriter, r *http.Request) {
	if run, ok := a.mustRun(w, r); ok {
		writeJSON(w, http.StatusOK, run)
	}
}

func (a *App) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	run, ok := a.mustRun(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"canceled": a.runner.cancel(run.ID)})
}

func (a *App) handleRunResults(w http.ResponseWriter, r *http.Request) {
	run, ok := a.mustRun(w, r)
	if !ok {
		return
	}
	results, err := listResults(a.db, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scores, err := listScores(a.db, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run":     run,
		"summary": summarize(results, scores),
		"results": results,
		"scores":  scores,
	})
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	run, ok := a.mustRun(w, r)
	if !ok {
		return
	}
	results, err := listResults(a.db, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scores, _ := listScores(a.db, run.ID)

	// One column per evaluator, filled with that evaluator's score.
	evaluatorIDs := []string{}
	seen := map[string]bool{}
	byResult := map[int64]map[string]*float64{}
	for _, s := range scores {
		if !seen[s.EvaluatorID] {
			seen[s.EvaluatorID] = true
			evaluatorIDs = append(evaluatorIDs, s.EvaluatorID)
		}
		if byResult[s.ResultID] == nil {
			byResult[s.ResultID] = map[string]*float64{}
		}
		byResult[s.ResultID][s.EvaluatorID] = s.Score
	}
	sort.Strings(evaluatorIDs)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-execucao-%d.csv"`, run.Experiment, run.ID))
	w.Write([]byte("\xef\xbb\xbf")) // BOM so Excel opens accents correctly
	out := csv.NewWriter(w)
	header := []string{"input", "repeticao", "provedor", "modelo", "prompt", "imagens", "referencia", "resposta",
		"tokens_entrada", "tokens_saida", "latencia_ms", "custo_usd", "erro"}
	out.Write(append(header, evaluatorIDs...))
	for _, res := range results {
		cost := ""
		if res.CostUSD != nil {
			cost = strconv.FormatFloat(*res.CostUSD, 'f', 6, 64)
		}
		row := []string{
			strconv.Itoa(res.InputIndex + 1), strconv.Itoa(res.Repetition), res.Provider, res.Model,
			res.UserPrompt, strings.Join(res.Images, "; "), res.Reference, res.Output,
			strconv.Itoa(res.InputTokens), strconv.Itoa(res.OutputTokens), strconv.FormatInt(res.LatencyMs, 10),
			cost, res.Error,
		}
		for _, id := range evaluatorIDs {
			value := ""
			if s := byResult[res.ID][id]; s != nil {
				value = strconv.FormatFloat(*s, 'f', 4, 64)
			}
			row = append(row, value)
		}
		out.Write(row)
	}
	out.Flush()
}

// ---------- Evaluation ----------

func (a *App) handleEvaluateRun(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	run, ok := a.mustRun(w, r)
	if !ok {
		return
	}
	exp, err := loadExperiment(cfg, run.Experiment)
	if err != nil {
		writeError(w, http.StatusBadRequest, tr("experimento não encontrado", "experiment not found"))
		return
	}
	var body struct {
		Evaluators []string `json:"evaluators"` // empty = the experiment's evaluators
	}
	readJSON(r, &body)
	ids := body.Evaluators
	if len(ids) == 0 {
		ids = exp.Evaluators
	}
	var evaluators []Evaluator
	for _, id := range ids {
		ev, err := loadEvaluator(cfg, id)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf(tr("avaliador %q não encontrado", "evaluator %q not found"), id))
			return
		}
		evaluators = append(evaluators, ev)
	}
	if err := a.evals.start(cfg, exp, run.ID, evaluators); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, a.evals.status(run.ID))
}

// handleRunEvaluation returns progress plus the model × evaluator score matrix.
func (a *App) handleRunEvaluation(w http.ResponseWriter, r *http.Request) {
	run, ok := a.mustRun(w, r)
	if !ok {
		return
	}
	results, err := listResults(a.db, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	scores, err := listScores(a.db, run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	modelOf := map[int64]string{}
	var models []string
	seenModel := map[string]bool{}
	for _, res := range results {
		modelOf[res.ID] = res.Model
		if !seenModel[res.Model] {
			seenModel[res.Model] = true
			models = append(models, res.Model)
		}
	}
	type cell struct {
		Sum    float64  `json:"-"`
		N      int      `json:"n"`
		Errors int      `json:"errors"`
		Mean   *float64 `json:"mean"`
	}
	matrix := map[string]map[string]*cell{} // model -> evaluator -> cell
	for _, s := range scores {
		model := modelOf[s.ResultID]
		if matrix[model] == nil {
			matrix[model] = map[string]*cell{}
		}
		c := matrix[model][s.EvaluatorID]
		if c == nil {
			c = &cell{}
			matrix[model][s.EvaluatorID] = c
		}
		if s.Score != nil {
			c.Sum += *s.Score
			c.N++
		} else {
			c.Errors++
		}
	}
	for _, row := range matrix {
		for _, c := range row {
			if c.N > 0 {
				mean := c.Sum / float64(c.N)
				c.Mean = &mean
			}
		}
	}
	if models == nil {
		models = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"progress": a.evals.status(run.ID),
		"models":   models,
		"matrix":   matrix,
		"answers":  len(results),
	})
}

// ---------- Datasets ----------

func (a *App) handleListDatasets(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	files := []string{}
	for _, pattern := range []string{"*.csv", "*.jsonl", "*.ndjson"} {
		matches, _ := filepath.Glob(filepath.Join(cfg.Paths.Datasets, pattern))
		for _, m := range matches {
			files = append(files, filepath.Base(m))
		}
	}
	sort.Strings(files)
	writeJSON(w, http.StatusOK, files)
}

// handleUploadDataset accepts either a multipart file ("file") or JSON
// {"name": "...", "text": "..."} with pasted text (one input per paragraph).
func (a *App) handleUploadDataset(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var name string
	var data []byte

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxDatasetBytes)
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, tr("arquivo não recebido (máximo 50 MB)", "file not received (50 MB max)"))
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		name = header.Filename
	} else {
		var body struct {
			Name string `json:"name"`
			Text string `json:"text"`
		}
		if err := readJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var count int
		data, count = textToJSONL(body.Text)
		if count == 0 {
			writeError(w, http.StatusBadRequest, tr("cole pelo menos um texto", "paste at least one text"))
			return
		}
		name = slugify(body.Name) + ".jsonl"
	}

	ext := strings.ToLower(filepath.Ext(name))
	base := slugify(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if _, err := parseDataset(name, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Never overwrite an existing dataset: old runs may depend on it.
	file := base + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(cfg.Paths.Datasets, file)); errors.Is(err, os.ErrNotExist) {
			break
		}
		file = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	if err := writeFileAtomic(filepath.Join(cfg.Paths.Datasets, file), data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"file": file})
}

func (a *App) handlePreviewDataset(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	ds, err := loadDataset(cfg, r.PathValue("file"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 5
	}
	rows := ds.Rows
	if len(rows) > limit {
		rows = rows[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file": ds.File, "columns": ds.Columns, "rows": rows, "count": len(ds.Rows),
		"image_columns_guess": guessImageColumns(cfg, ds),
	})
}

// guessImageColumns returns the columns whose values (in the first rows) are all
// images that exist (or URLs), so the UI can pre-select them. A column with
// just file names, like "foto.jpg", doesn't count unless that file is in data/.
func guessImageColumns(cfg Config, ds Dataset) []string {
	guess := []string{}
	for _, col := range ds.Columns {
		seen, allImages := 0, true
		for i, row := range ds.Rows {
			if i >= 20 {
				break
			}
			if value := strings.TrimSpace(row[col]); value != "" {
				seen++
				allImages = allImages && looksLikeImage(value) && checkImageRef(cfg, value) == nil
			}
		}
		if seen > 0 && allImages {
			guess = append(guess, col)
		}
	}
	return guess
}

// handleUploadImages saves several images into data/images/<name>/ and creates
// a dataset <name>.jsonl with one row per image (columns "imagem" and "arquivo").
func (a *App) handleUploadImages(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, tr("não foi possível receber as imagens (máximo 500 MB no total)", "could not receive the images (500 MB max in total)"))
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, tr("escolha pelo menos uma imagem", "choose at least one image"))
		return
	}

	// The dataset file and the image folder share a unique name.
	base := slugify(r.FormValue("name"))
	name := base
	for i := 2; ; i++ {
		_, errFile := os.Stat(filepath.Join(cfg.Paths.Datasets, name+".jsonl"))
		_, errDir := os.Stat(filepath.Join(cfg.Paths.Datasets, "images", name))
		if errors.Is(errFile, os.ErrNotExist) && errors.Is(errDir, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	folder := filepath.Join(cfg.Paths.Datasets, "images", name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var lines []byte
	used := map[string]bool{}
	for _, header := range files {
		f, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
		f.Close()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(data) > maxImageBytes {
			writeError(w, http.StatusBadRequest, header.Filename+tr(": imagem maior que 20 MB", ": image larger than 20 MB"))
			return
		}
		mediaType, err := imageMediaType(data)
		if err != nil {
			writeError(w, http.StatusBadRequest, header.Filename+": "+err.Error())
			return
		}
		// Safe file name: slug of the original name + extension from the real type.
		ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[mediaType]
		stem := slugify(strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)))
		fileName := stem + ext
		for i := 2; used[fileName]; i++ {
			fileName = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		used[fileName] = true
		if err := os.WriteFile(filepath.Join(folder, fileName), data, 0o644); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		line, _ := json.Marshal(map[string]string{
			"imagem":  "images/" + name + "/" + fileName, // relative to the datasets folder
			"arquivo": header.Filename,
		})
		lines = append(append(lines, line...), '\n')
	}
	file := name + ".jsonl"
	if err := writeFileAtomic(filepath.Join(cfg.Paths.Datasets, file), lines, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"file": file, "image_columns": []string{"imagem"}, "count": len(files)})
}

// handleDataFile serves an image from the datasets folder, so the UI can show
// thumbnails. Only image files are served.
func (a *App) handleDataFile(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	rel := r.PathValue("path")
	if !looksLikeImage(rel) {
		writeError(w, http.StatusNotFound, tr("arquivo não encontrado", "file not found"))
		return
	}
	path, err := safeDataPath(cfg, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	http.ServeFile(w, r, path)
}

// ---------- Evaluators ----------

func (a *App) handleListEvaluators(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	evaluators, err := listEvaluators(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type item struct {
		Evaluator
		Problems []string `json:"problems"`
	}
	// With ?experiment=id, also report what each evaluator is missing for it.
	var exp *Experiment
	if id := r.URL.Query().Get("experiment"); id != "" {
		if e, err := loadExperiment(cfg, id); err == nil {
			exp = &e
		}
	}
	items := []item{}
	for _, ev := range evaluators {
		problems := []string{}
		if exp != nil {
			problems = append(problems, evaluatorProblems(cfg, ev, *exp)...)
		}
		items = append(items, item{Evaluator: ev, Problems: problems})
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) handleCreateEvaluator(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var ev Evaluator
	if err := readJSON(r, &ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev.ID = uniqueID(cfg.Paths.Evaluators, slugify(ev.Name))
	if err := saveEvaluator(cfg, ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}

func (a *App) handlePutEvaluator(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var ev Evaluator
	if err := readJSON(r, &ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev.ID = r.PathValue("id")
	if _, err := os.Stat(evaluatorPath(cfg, ev.ID)); err != nil {
		writeError(w, http.StatusNotFound, tr("avaliador não encontrado", "evaluator not found"))
		return
	}
	if err := saveEvaluator(cfg, ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleTestEvaluator runs a (possibly unsaved) decision evaluator on one example
// typed in the UI, and returns the raw answer so people can tune the question
// and the threshold before running it on a whole experiment.
func (a *App) handleTestEvaluator(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	var body struct {
		Evaluator Evaluator `json:"evaluator"`
		Item      evalItem  `json:"item"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev := body.Evaluator
	if ev.Kind != "decision" {
		writeError(w, http.StatusBadRequest, tr("o teste rápido está disponível para avaliadores do tipo Motor de decisão", "the quick test is available for Decision engine evaluators"))
		return
	}
	if err := validateEvaluator(ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if problems := decisionProblems(cfg, ev, Experiment{Dataset: DatasetRef{ReferenceColumn: "-"}}); len(problems) > 0 {
		writeError(w, http.StatusBadRequest, problems[0])
		return
	}
	if body.Item.Context == "" {
		body.Item.Context = body.Item.Question
	}
	answer, score, reason, err := evaluateDecision(cfg, ev, body.Item)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"score":  score,
		"reason": reason,
		"passed": score >= ev.Threshold,
		"answer": answer,
		"state": fillTemplate(stateTemplate(ev), map[string]string{
			"question": body.Item.Question, "answer": body.Item.Answer,
			"reference": body.Item.Reference, "context": body.Item.Context,
		}),
	})
}

func (a *App) handleDeleteEvaluator(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, tr("identificador inválido", "invalid id"))
		return
	}
	if err := os.Remove(evaluatorPath(cfg, id)); err != nil {
		writeError(w, http.StatusNotFound, tr("avaliador não encontrado", "evaluator not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- Evaluator spreadsheet tests ----------

func (a *App) handleStartEvaluatorTest(w http.ResponseWriter, r *http.Request) {
	cfg, ok := mustConfig(w)
	if !ok {
		return
	}
	ev, err := loadEvaluator(cfg, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, tr("avaliador não encontrado (salve-o antes de testar)", "evaluator not found (save it before testing)"))
		return
	}
	var body struct {
		File    string       `json:"file"`
		Columns SheetColumns `json:"columns"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := a.evalTests.start(cfg, ev, body.File, body.Columns)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"test_id": id})
}

func (a *App) handleListEvaluatorTests(w http.ResponseWriter, r *http.Request) {
	tests, err := listEvaluatorTests(a.db, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tests)
}

func (a *App) mustEvaluatorTest(w http.ResponseWriter, r *http.Request) (EvaluatorTest, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, tr("teste inválido", "invalid test"))
		return EvaluatorTest{}, false
	}
	t, err := getEvaluatorTest(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, tr("teste não encontrado", "test not found"))
		return EvaluatorTest{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return EvaluatorTest{}, false
	}
	return t, true
}

// handleGetEvaluatorTest returns the test, its rows and the summary. The
// summary uses the evaluator as it was when the test started (its threshold).
func (a *App) handleGetEvaluatorTest(w http.ResponseWriter, r *http.Request) {
	t, ok := a.mustEvaluatorTest(w, r)
	if !ok {
		return
	}
	rows, err := listTestRows(a.db, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ev Evaluator
	var cols SheetColumns
	json.Unmarshal([]byte(t.EvaluatorJSON), &ev)
	json.Unmarshal([]byte(t.ColumnsJSON), &cols)
	markTestRows(rows, ev.Threshold)
	writeJSON(w, http.StatusOK, map[string]any{
		"test":      t,
		"columns":   cols,
		"evaluator": ev,
		"summary":   summarizeTest(rows, ev.Threshold, cols.Human != ""),
		"rows":      rows,
	})
}

func (a *App) handleCancelEvaluatorTest(w http.ResponseWriter, r *http.Request) {
	if t, ok := a.mustEvaluatorTest(w, r); ok {
		writeJSON(w, http.StatusOK, map[string]bool{"canceled": a.evalTests.cancel(t.ID)})
	}
}

func (a *App) handleDeleteEvaluatorTest(w http.ResponseWriter, r *http.Request) {
	t, ok := a.mustEvaluatorTest(w, r)
	if !ok {
		return
	}
	a.evalTests.cancel(t.ID)
	if err := deleteEvaluatorTest(a.db, t.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleExportEvaluatorTest(w http.ResponseWriter, r *http.Request) {
	t, ok := a.mustEvaluatorTest(w, r)
	if !ok {
		return
	}
	rows, err := listTestRows(a.db, t.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ev Evaluator
	json.Unmarshal([]byte(t.EvaluatorJSON), &ev)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="teste-%s-%d.csv"`, t.EvaluatorID, t.ID))
	w.Write([]byte("\xef\xbb\xbf")) // BOM so Excel opens accents correctly
	out := csv.NewWriter(w)
	out.Write([]string{"linha", "input", "resposta", "referencia", "veredito_humano", "nota", "aprovado", "concorda", "motivo", "erro"})
	for _, row := range rows {
		score, approved, agrees := "", "", ""
		if row.Score != nil {
			score = strconv.FormatFloat(*row.Score, 'f', 4, 64)
			pass := *row.Score >= ev.Threshold
			approved = strconv.FormatBool(pass)
			if human := parseHumanVerdict(row.Human); human != nil {
				agrees = strconv.FormatBool(pass == *human)
			}
		}
		out.Write([]string{strconv.Itoa(row.RowIndex + 1), row.Question, row.Answer, row.Reference, row.Human,
			score, approved, agrees, row.Reason, row.Error})
	}
	out.Flush()
}

// ---------- Frontend ----------

// handleFrontend serves the built React app. Unknown paths return index.html so
// that client-side routes like /experimentos/x/resultados work on reload.
func (a *App) handleFrontend(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" {
		if f, err := a.web.Open(path); err == nil {
			f.Close()
			http.ServeFileFS(w, r, a.web, path)
			return
		}
	}
	index, err := fs.ReadFile(a.web, "index.html")
	if err == nil {
		// Tell the interface which language to use (frontend/src/i18n.ts reads <html lang>).
		if cfg, cfgErr := loadConfig(); cfgErr == nil {
			index = bytes.Replace(index, []byte(`<html lang="pt-BR">`), []byte(`<html lang="`+cfg.Language+`">`), 1)
		}
	}
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(tr("A interface ainda não foi compilada. Rode \"make build\" (ou, em desenvolvimento, \"make dev\").\n", "The interface has not been built yet. Run \"make build\" (or, in development, \"make dev\").\n")))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(index)
}
