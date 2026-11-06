package main

// evaluation.go: scoring model answers with the Python evaluation service.
//
// Go starts the Python service (evaluator/ folder) as a child process when its
// virtual environment exists. For each answer and evaluator, Go sends one HTTP
// request to the service and stores the score it gets back.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------- Python service process ----------

// startEvaluatorService launches `python -m uvicorn main:app` from evaluator/.venv.
// It returns nil (and the app keeps working without evaluation) if Python isn't set up.
func startEvaluatorService(serviceURL string) *exec.Cmd {
	u, err := url.Parse(serviceURL)
	if err != nil {
		log.Printf("invalid evaluator_url %q: %v", serviceURL, err)
		return nil
	}
	// A service on another host (e.g. the "evaluator" container in Docker
	// Compose) is managed by someone else: just use it.
	if host := u.Hostname(); host != "127.0.0.1" && host != "localhost" {
		log.Printf("using evaluation service at %s", serviceURL)
		return nil
	}
	if evaluatorOnline(serviceURL) {
		log.Printf("evaluation service already running at %s", serviceURL)
		return nil
	}
	python := filepath.Join("evaluator", ".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		python = filepath.Join("evaluator", ".venv", "Scripts", "python.exe")
	}
	if _, err := os.Stat(python); err != nil {
		log.Printf("evaluation service not started: %s not found (run ./start.sh once to set it up)", python)
		return nil
	}
	port := u.Port()
	if port == "" {
		port = "8001"
	}
	absPython, _ := filepath.Abs(python)
	cmd := exec.Command(absPython, "-m", "uvicorn", "main:app", "--host", "127.0.0.1", "--port", port, "--log-level", "warning")
	cmd.Dir = "evaluator"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("could not start evaluation service: %v", err)
		return nil
	}
	log.Printf("evaluation service starting on %s (pid %d)", serviceURL, cmd.Process.Pid)
	return cmd
}

func evaluatorOnline(serviceURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var out map[string]any
	return getJSON(ctx, serviceURL+"/health", nil, &out) == nil
}

// ---------- What each evaluator needs ----------

// evaluatorProblems lists what is missing for an evaluator to work on this
// experiment (empty list = ready). Messages are shown in the UI as-is.
func evaluatorProblems(cfg Config, ev Evaluator, exp Experiment) []string {
	var problems []string
	needsJudge := ev.Kind == "llm_judge" || ev.Kind == "ragas"
	needsEmbedding := ev.Kind == "embedding" || (ev.Kind == "ragas" && ev.Metric == "answer_relevancy")
	needsReference := ev.Kind == "embedding" ||
		(ev.Kind == "ragas" && ev.Metric == "context_precision") ||
		(ev.Kind == "rule" && (ev.Metric == "exact" || (ev.Metric == "contains" && ev.Pattern == "")))
	needsContext := ev.Kind == "ragas" && ev.Metric == "context_precision"

	if needsReference && exp.Dataset.ReferenceColumn == "" {
		problems = append(problems, tr("Falta mapear a coluna de resposta esperada", "Map the expected answer column"))
	}
	if needsContext && exp.Dataset.ContextColumn == "" {
		problems = append(problems, tr("Falta mapear a coluna de contexto recuperado", "Map the retrieved context column"))
	}
	if needsJudge {
		problems = append(problems, providerProblems(cfg, ev.JudgeProvider, ev.JudgeModel, tr("modelo juiz", "judge model"))...)
	}
	if needsEmbedding {
		problems = append(problems, providerProblems(cfg, ev.EmbeddingProvider, ev.EmbeddingModel, tr("modelo de embedding", "embedding model"))...)
	}
	if ev.Kind == "decision" {
		problems = append(problems, decisionProblems(cfg, ev, exp)...)
	}
	return problems
}

func decisionProblems(cfg Config, ev Evaluator, exp Experiment) []string {
	if problems := providerProblems(cfg, ev.DecisionProvider, ev.DecisionModel, tr("modelo de decisão", "decision model")); problems != nil {
		return problems
	}
	var problems []string
	kind := cfg.Providers[ev.DecisionProvider].Kind
	if kind != "ollama" && kind != "typesafe" {
		problems = append(problems, tr("Modelos de decisão só funcionam com provedores Ollama ou TypeSafe", "Decision models only work with Ollama or TypeSafe providers"))
	}
	if ev.SendImages && kind != "ollama" {
		problems = append(problems, tr("O envio de imagens para modelos de decisão só funciona pelo Ollama", "Sending images to decision models only works through Ollama"))
	}
	if strings.Contains(stateTemplate(ev), "{reference}") && exp.Dataset.ReferenceColumn == "" {
		problems = append(problems, tr("Falta mapear a coluna de resposta esperada (o texto usa {reference})", "Map the expected answer column (the text uses {reference})"))
	}
	return problems
}

func providerProblems(cfg Config, providerName, model, label string) []string {
	if providerName == "" || model == "" {
		return []string{tr("Escolha o ", "Choose the ") + label}
	}
	p, ok := cfg.Providers[providerName]
	if !ok {
		return []string{fmt.Sprintf(tr("O provedor %q do %s não existe", "Provider %q of the %s does not exist"), providerName, label)}
	}
	if p.APIKeyEnv != "" && apiKey(p) == "" {
		return []string{fmt.Sprintf(tr("Chave não configurada para %s (%s)", "No API key for %s (%s)"), providerName, label)}
	}
	return nil
}

// checkPythonService fails if some evaluator needs the Python service and it is
// offline. Only decision evaluators run without it.
func checkPythonService(cfg Config, evaluators []Evaluator) error {
	for _, ev := range evaluators {
		if ev.Kind != "decision" {
			if !evaluatorOnline(cfg.Server.EvaluatorURL) {
				return errors.New(tr("o serviço de avaliação (Python) está offline. Veja Configurações › Serviços", "the evaluation service (Python) is offline. See Settings › Services"))
			}
			return nil
		}
	}
	return nil
}

// ---------- Running an evaluation ----------

type EvalProgress struct {
	Running bool   `json:"running"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Error   string `json:"error"`
}

type Evaluations struct {
	db       *sql.DB
	mu       sync.Mutex
	progress map[int64]*EvalProgress
}

func newEvaluations(db *sql.DB) *Evaluations {
	return &Evaluations{db: db, progress: map[int64]*EvalProgress{}}
}

func (e *Evaluations) status(runID int64) EvalProgress {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.progress[runID]; ok {
		return *p
	}
	return EvalProgress{}
}

// start scores every successful answer of a run with the given evaluators, in the background.
func (e *Evaluations) start(cfg Config, exp Experiment, runID int64, evaluators []Evaluator) error {
	if err := checkPythonService(cfg, evaluators); err != nil {
		return err
	}
	for _, ev := range evaluators {
		if problems := evaluatorProblems(cfg, ev, exp); len(problems) > 0 {
			return fmt.Errorf("%s: %s", ev.Name, problems[0])
		}
	}
	results, err := listResults(e.db, runID)
	if err != nil {
		return err
	}
	var answered []Result
	for _, r := range results {
		if r.Error == "" {
			answered = append(answered, r)
		}
	}
	if len(answered) == 0 || len(evaluators) == 0 {
		return errors.New(tr("nada para avaliar: escolha avaliadores e rode o experimento primeiro", "nothing to evaluate: choose evaluators and run the experiment first"))
	}

	e.mu.Lock()
	if p, ok := e.progress[runID]; ok && p.Running {
		e.mu.Unlock()
		return errors.New(tr("esta execução já está sendo avaliada", "this run is already being evaluated"))
	}
	progress := &EvalProgress{Running: true, Total: len(answered) * len(evaluators)}
	e.progress[runID] = progress
	e.mu.Unlock()

	go e.process(cfg, runID, answered, evaluators, progress)
	return nil
}

type evalTask struct {
	result    Result
	evaluator Evaluator
}

func (e *Evaluations) process(cfg Config, runID int64, results []Result, evaluators []Evaluator, progress *EvalProgress) {
	tasks := make(chan evalTask)
	var wg sync.WaitGroup
	const workers = 4
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				score := callEvaluator(cfg, t.evaluator, t.result)
				if err := upsertScore(e.db, score); err != nil {
					log.Printf("run %d: could not save score: %v", runID, err)
				}
				e.mu.Lock()
				progress.Done++
				e.mu.Unlock()
			}
		}()
	}
	for _, ev := range evaluators {
		for _, r := range results {
			tasks <- evalTask{result: r, evaluator: ev}
		}
	}
	close(tasks)
	wg.Wait()

	e.mu.Lock()
	progress.Running = false
	e.mu.Unlock()
}

// modelEndpoint is everything Python needs to call a judge or embedding model.
type modelEndpoint struct {
	Kind    string `json:"kind"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

func endpointFor(cfg Config, providerName, model string) *modelEndpoint {
	if providerName == "" || model == "" {
		return nil
	}
	p := cfg.Providers[providerName]
	return &modelEndpoint{Kind: p.Kind, Model: model, BaseURL: p.BaseURL, APIKey: apiKey(p)}
}

// evalItem is what an evaluator looks at: one answer plus its input.
type evalItem struct {
	Question  string   `json:"question"` // the prompt the model received
	Answer    string   `json:"answer"`
	Reference string   `json:"reference"`
	Context   string   `json:"context"`
	Images    []string `json:"images"`
}

func itemFromResult(r Result) evalItem {
	context_ := r.Context
	if context_ == "" {
		// Without a context column, the "context" is what the model was given.
		context_ = r.UserPrompt
	}
	return evalItem{Question: r.UserPrompt, Answer: r.Output, Reference: r.Reference, Context: context_, Images: r.Images}
}

// callEvaluator scores one answer. Decision evaluators run here in Go;
// all the others are sent to the Python service.
func callEvaluator(cfg Config, ev Evaluator, r Result) Score {
	score := scoreItem(cfg, ev, itemFromResult(r))
	score.ResultID = r.ID
	return score
}

// scoreItem scores one item with one evaluator. It is shared by experiment
// evaluations and by spreadsheet tests (evaluator_tests.go).
func scoreItem(cfg Config, ev Evaluator, item evalItem) Score {
	score := Score{EvaluatorID: ev.ID}
	if ev.Kind == "decision" {
		_, value, reason, err := evaluateDecision(cfg, ev, item)
		if err != nil {
			score.Error = err.Error()
		} else {
			score.Score, score.Reason = &value, reason
		}
		return score
	}
	return callPythonEvaluator(cfg, ev, item, score)
}

func stateTemplate(ev Evaluator) string {
	if strings.TrimSpace(ev.StateTemplate) == "" {
		return defaultStateTemplate
	}
	return ev.StateTemplate
}

// evaluateDecision asks the decision model the evaluator's question about one item.
// It also returns the raw answer, so the "test" button can show the probabilities.
func evaluateDecision(cfg Config, ev Evaluator, item evalItem) (DecisionAnswer, float64, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	state := fillTemplate(stateTemplate(ev), map[string]string{
		"question": item.Question, "answer": item.Answer, "reference": item.Reference, "context": item.Context,
	})
	var images []Image
	if ev.SendImages {
		for _, ref := range item.Images {
			img, err := loadImage(ctx, cfg, ref)
			if err != nil {
				return DecisionAnswer{}, 0, "", err
			}
			images = append(images, img)
		}
	}
	provider, ok := cfg.Providers[ev.DecisionProvider]
	if !ok {
		return DecisionAnswer{}, 0, "", fmt.Errorf(tr("o provedor %q não existe", "provider %q does not exist"), ev.DecisionProvider)
	}
	answer, err := callDecision(ctx, provider, ev.DecisionModel, state, ev.Question, images)
	if err != nil {
		return DecisionAnswer{}, 0, "", err
	}
	value, reason, err := decisionScore(ev.Question, answer)
	return answer, value, reason, err
}

// callPythonEvaluator sends one item to the Python service (ragas, judge, embedding, rule).
func callPythonEvaluator(cfg Config, ev Evaluator, item evalItem, score Score) Score {
	body := map[string]any{
		"evaluator": map[string]any{
			"kind": ev.Kind, "metric": ev.Metric, "rubric": ev.Rubric, "pattern": ev.Pattern,
		},
		"item": map[string]string{
			"question": item.Question, "answer": item.Answer, "reference": item.Reference, "context": item.Context,
		},
		"language":  currentLanguage(),
		"judge":     endpointFor(cfg, ev.JudgeProvider, ev.JudgeModel),
		"embedding": endpointFor(cfg, ev.EmbeddingProvider, ev.EmbeddingModel),
	}
	var out struct {
		Score  *float64 `json:"score"`
		Reason string   `json:"reason"`
		Error  string   `json:"error"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := postJSON(ctx, cfg.Server.EvaluatorURL+"/evaluate", nil, body, &out); err != nil {
		score.Error = err.Error()
		return score
	}
	score.Score, score.Reason, score.Error = out.Score, out.Reason, out.Error
	return score
}
