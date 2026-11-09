package main

// config.go: everything that lives in YAML files.
//
//   bancada.yaml              -> Config (global settings)
//   experiments/<id>.yaml     -> Experiment
//   evaluators/<id>.yaml      -> Evaluator
//
// The files on disk are always the source of truth: every API request reads
// them fresh, and every save from the UI writes them back. That way a person
// can edit a file by hand and the UI shows the change on the next load.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------- Global config (bancada.yaml) ----------

type Config struct {
	Language  string                    `yaml:"language" json:"language"` // "pt-BR" or "en" (see i18n.go)
	Server    ServerConfig              `yaml:"server" json:"server"`
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Defaults  Defaults                  `yaml:"defaults" json:"defaults"`
	Paths     Paths                     `yaml:"paths" json:"paths"`
	Pricing   map[string]Price          `yaml:"pricing" json:"pricing"`
}

type ServerConfig struct {
	Port         int    `yaml:"port" json:"port"`
	EvaluatorURL string `yaml:"evaluator_url" json:"evaluator_url"`
	OpenBrowser  bool   `yaml:"open_browser" json:"open_browser"`
}

// ProviderConfig describes how to reach one LLM provider.
// Kind is one of: openai, anthropic, gemini, ollama, openai_compatible, typesafe.
// "typesafe" is TypeSafe's hosted decision models (Jev). It only answers
// decision questions (see decisions.go), it doesn't generate text.
// API keys are never stored here, only the NAME of the environment variable
// that holds the key.
type ProviderConfig struct {
	Kind      string `yaml:"kind" json:"kind"`
	APIKeyEnv string `yaml:"api_key_env,omitempty" json:"api_key_env,omitempty"`
	BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
}

type Defaults struct {
	Run           RunSettings `yaml:"run" json:"run"`
	JudgeProvider string      `yaml:"judge_provider" json:"judge_provider"`
	JudgeModel    string      `yaml:"judge_model" json:"judge_model"`
}

type Paths struct {
	Experiments string `yaml:"experiments" json:"experiments"`
	Evaluators  string `yaml:"evaluators" json:"evaluators"`
	Datasets    string `yaml:"datasets" json:"datasets"`
	Database    string `yaml:"database" json:"database"`
}

// Price is in US dollars per 1 million tokens.
type Price struct {
	Input  float64 `yaml:"input" json:"input"`
	Output float64 `yaml:"output" json:"output"`
}

const configFile = "bancada.yaml"

// defaultConfig is used when bancada.yaml doesn't exist yet. Two addresses can
// be changed with environment variables, so Docker Compose can point them at
// the other containers (see docker-compose.yml):
//
//	BANCADA_EVALUATOR_URL  (default http://127.0.0.1:8001)
//	BANCADA_OLLAMA_URL     (default http://localhost:11434)
func defaultConfig() Config {
	return Config{
		Language: "pt-BR",
		Server:   ServerConfig{Port: 8080, EvaluatorURL: envOr("BANCADA_EVALUATOR_URL", "http://127.0.0.1:8001"), OpenBrowser: true},
		Providers: map[string]ProviderConfig{
			"openai":    {Kind: "openai", APIKeyEnv: "OPENAI_API_KEY", BaseURL: "https://api.openai.com"},
			"anthropic": {Kind: "anthropic", APIKeyEnv: "ANTHROPIC_API_KEY", BaseURL: "https://api.anthropic.com"},
			"gemini":    {Kind: "gemini", APIKeyEnv: "GEMINI_API_KEY", BaseURL: "https://generativelanguage.googleapis.com"},
			"ollama":    {Kind: "ollama", BaseURL: envOr("BANCADA_OLLAMA_URL", "http://localhost:11434")},
			"typesafe":  {Kind: "typesafe", APIKeyEnv: "TYPESAFE_API_KEY", BaseURL: "https://api.typesafe.ai"},
		},
		Defaults: Defaults{
			Run:           RunSettings{Repetitions: 1, Concurrency: 4, TimeoutS: 120},
			JudgeProvider: "anthropic",
			JudgeModel:    "claude-sonnet-5-5",
		},
		Paths: Paths{Experiments: "experiments", Evaluators: "evaluators", Datasets: "data", Database: "data/bancada.db"},
		// Prices change often. Check the provider's pricing page and edit here.
		Pricing: map[string]Price{
			"claude-opus-5-5":   {Input: 4, Output: 20},
			"claude-sonnet-5-5": {Input: 2, Output: 10},
			"claude-haiku-5-5":  {Input: 0.10, Output: 0.50},
			"gpt-4o":            {Input: 2.5, Output: 10},
			"gpt-4o-mini":       {Input: 0.15, Output: 0.60},
			"gemini-2.5-pro":    {Input: 1.25, Output: 10},
			"gemini-2.5-flash":  {Input: 0.30, Output: 2.50},
		},
	}
}

// loadConfig reads bancada.yaml. If it does not exist, a default one is written.
func loadConfig() (Config, error) {
	data, err := os.ReadFile(configFile)
	if errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		return cfg, saveConfig(cfg)
	}
	if err != nil {
		return Config{}, err
	}
	return parseConfig(data)
}

// parseConfig parses YAML on top of the defaults, so missing keys get sane values.
func parseConfig(data []byte) (Config, error) {
	cfg := defaultConfig()
	// Start with empty maps so the file fully decides which providers/prices exist.
	cfg.Providers = nil
	cfg.Pricing = nil
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf(tr("YAML inválido: %w", "invalid YAML: %w"), err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Pricing == nil {
		cfg.Pricing = map[string]Price{}
	}
	if !validLanguage(cfg.Language) {
		return Config{}, fmt.Errorf(tr("language %q: use pt-BR ou en", "language %q: use pt-BR or en"), cfg.Language)
	}
	setLanguage(cfg.Language)
	for name, p := range cfg.Providers {
		if !validProviderKind(p.Kind) {
			return Config{}, fmt.Errorf(tr("provedor %q: tipo %q desconhecido (use openai, anthropic, gemini, ollama, openai_compatible ou typesafe)", "provider %q: unknown kind %q (use openai, anthropic, gemini, ollama, openai_compatible or typesafe)"), name, p.Kind)
		}
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	return writeYAML(configFile, cfg)
}

func validProviderKind(kind string) bool {
	switch kind {
	case "openai", "anthropic", "gemini", "ollama", "openai_compatible", "typesafe":
		return true
	}
	return false
}

// ---------- Experiments (experiments/<id>.yaml) ----------

type Experiment struct {
	ID          string        `yaml:"-" json:"id"`
	Name        string        `yaml:"name" json:"name"`
	Description string        `yaml:"description,omitempty" json:"description"`
	UseCase     string        `yaml:"use_case,omitempty" json:"use_case"`
	Models      []ModelConfig `yaml:"models" json:"models"`
	Prompt      Prompt        `yaml:"prompt" json:"prompt"`
	Dataset     DatasetRef    `yaml:"dataset" json:"dataset"`
	Run         RunSettings   `yaml:"run" json:"run"`
	Evaluators  []string      `yaml:"evaluators" json:"evaluators"` // evaluator ids enabled for this experiment
}

type ModelConfig struct {
	Model       string   `yaml:"model" json:"model"`       // e.g. "gpt-4o-mini", "llama3.1:8b"
	Provider    string   `yaml:"provider" json:"provider"` // key in Config.Providers
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	Temperature *float64 `yaml:"temperature,omitempty" json:"temperature"` // nil = provider default
	MaxTokens   int      `yaml:"max_tokens,omitempty" json:"max_tokens"`
}

// Prompt templates. "{column}" is replaced by that column's value in each dataset row.
type Prompt struct {
	System string `yaml:"system" json:"system"`
	User   string `yaml:"user" json:"user"`
}

type DatasetRef struct {
	File            string `yaml:"file" json:"file"`                                   // file name inside the datasets folder
	ReferenceColumn string `yaml:"reference_column,omitempty" json:"reference_column"` // expected answer
	ContextColumn   string `yaml:"context_column,omitempty" json:"context_column"`     // retrieved context (RAG)
	// Columns whose cells are images (a path inside the datasets folder or a URL).
	// Their images are attached to the message sent to the model. See images.go.
	ImageColumns []string `yaml:"image_columns,omitempty" json:"image_columns"`
}

type RunSettings struct {
	Repetitions int `yaml:"repetitions" json:"repetitions"`
	Concurrency int `yaml:"concurrency" json:"concurrency"`
	TimeoutS    int `yaml:"timeout_s" json:"timeout_s"`
}

func experimentPath(cfg Config, id string) string {
	return filepath.Join(cfg.Paths.Experiments, id+".yaml")
}

func listExperiments(cfg Config) ([]Experiment, error) {
	files, err := filepath.Glob(filepath.Join(cfg.Paths.Experiments, "*.yaml"))
	if err != nil {
		return nil, err
	}
	experiments := []Experiment{}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".yaml")
		exp, err := loadExperiment(cfg, id)
		if err != nil {
			// A broken file should not hide all the others. Show it with its error.
			exp = Experiment{ID: id, Name: id, Description: "Erro ao ler o arquivo: " + err.Error()}
		}
		experiments = append(experiments, exp)
	}
	sort.Slice(experiments, func(i, j int) bool { return experiments[i].Name < experiments[j].Name })
	return experiments, nil
}

func loadExperiment(cfg Config, id string) (Experiment, error) {
	if !validID(id) {
		return Experiment{}, fmt.Errorf(tr("identificador inválido: %q", "invalid id: %q"), id)
	}
	data, err := os.ReadFile(experimentPath(cfg, id))
	if err != nil {
		return Experiment{}, err
	}
	return parseExperiment(cfg, id, data)
}

func parseExperiment(cfg Config, id string, data []byte) (Experiment, error) {
	var exp Experiment
	if err := yaml.Unmarshal(data, &exp); err != nil {
		return Experiment{}, fmt.Errorf(tr("YAML inválido: %w", "invalid YAML: %w"), err)
	}
	exp.ID = id
	fillExperimentDefaults(cfg, &exp)
	return exp, nil
}

func fillExperimentDefaults(cfg Config, exp *Experiment) {
	if exp.Run.Repetitions < 1 {
		exp.Run.Repetitions = max(cfg.Defaults.Run.Repetitions, 1)
	}
	if exp.Run.Concurrency < 1 {
		exp.Run.Concurrency = max(cfg.Defaults.Run.Concurrency, 1)
	}
	if exp.Run.TimeoutS < 1 {
		exp.Run.TimeoutS = max(cfg.Defaults.Run.TimeoutS, 10)
	}
	if exp.Models == nil {
		exp.Models = []ModelConfig{}
	}
	if exp.Evaluators == nil {
		exp.Evaluators = []string{}
	}
	if exp.Dataset.ImageColumns == nil {
		exp.Dataset.ImageColumns = []string{}
	}
}

func saveExperiment(cfg Config, exp Experiment) error {
	if !validID(exp.ID) {
		return fmt.Errorf(tr("identificador inválido: %q", "invalid id: %q"), exp.ID)
	}
	if strings.TrimSpace(exp.Name) == "" {
		return errors.New(tr("o experimento precisa de um nome", "the experiment needs a name"))
	}
	return writeYAML(experimentPath(cfg, exp.ID), exp)
}

// ---------- Evaluators (evaluators/<id>.yaml) ----------

// Evaluator scores one model answer between 0 and 1.
//
//	Kind "ragas":     Metric = faithfulness | answer_relevancy | context_precision
//	Kind "llm_judge": a judge model follows Rubric
//	Kind "embedding": cosine similarity between answer and reference
//	Kind "rule":      Metric = exact | contains | regex, using Pattern
//	Kind "decision":  a decision model (Laya, JEV…) answers Question about the answer
type Evaluator struct {
	ID          string  `yaml:"-" json:"id"`
	Name        string  `yaml:"name" json:"name"`
	Description string  `yaml:"description,omitempty" json:"description"`
	Kind        string  `yaml:"kind" json:"kind"`
	Metric      string  `yaml:"metric,omitempty" json:"metric"`
	Threshold   float64 `yaml:"threshold" json:"threshold"`

	JudgeProvider string `yaml:"judge_provider,omitempty" json:"judge_provider"`
	JudgeModel    string `yaml:"judge_model,omitempty" json:"judge_model"`
	// Only for answer_relevancy and embedding.
	EmbeddingProvider string `yaml:"embedding_provider,omitempty" json:"embedding_provider"`
	EmbeddingModel    string `yaml:"embedding_model,omitempty" json:"embedding_model"`

	Rubric  string `yaml:"rubric,omitempty" json:"rubric"`
	Pattern string `yaml:"pattern,omitempty" json:"pattern"`

	// Only for kind "decision" (see decisions.go).
	DecisionProvider string           `yaml:"decision_provider,omitempty" json:"decision_provider"`
	DecisionModel    string           `yaml:"decision_model,omitempty" json:"decision_model"`
	Question         DecisionQuestion `yaml:"question,omitempty" json:"question"`
	// Text the decision model reads. Placeholders: {question} {answer} {reference} {context}.
	// Empty = defaultStateTemplate.
	StateTemplate string `yaml:"state_template,omitempty" json:"state_template"`
	// Also send the input's images: for decision models (vision models on Ollama)
	// and LLM judges (any provider, as long as the judge model has vision).
	SendImages bool `yaml:"send_images,omitempty" json:"send_images"`
}

func evaluatorPath(cfg Config, id string) string {
	return filepath.Join(cfg.Paths.Evaluators, id+".yaml")
}

func listEvaluators(cfg Config) ([]Evaluator, error) {
	files, err := filepath.Glob(filepath.Join(cfg.Paths.Evaluators, "*.yaml"))
	if err != nil {
		return nil, err
	}
	evaluators := []Evaluator{}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".yaml")
		ev, err := loadEvaluator(cfg, id)
		if err != nil {
			ev = Evaluator{ID: id, Name: id, Description: "Erro ao ler o arquivo: " + err.Error()}
		}
		evaluators = append(evaluators, ev)
	}
	sort.Slice(evaluators, func(i, j int) bool { return evaluators[i].Name < evaluators[j].Name })
	return evaluators, nil
}

func loadEvaluator(cfg Config, id string) (Evaluator, error) {
	if !validID(id) {
		return Evaluator{}, fmt.Errorf(tr("identificador inválido: %q", "invalid id: %q"), id)
	}
	data, err := os.ReadFile(evaluatorPath(cfg, id))
	if err != nil {
		return Evaluator{}, err
	}
	var ev Evaluator
	if err := yaml.Unmarshal(data, &ev); err != nil {
		return Evaluator{}, fmt.Errorf(tr("YAML inválido: %w", "invalid YAML: %w"), err)
	}
	ev.ID = id
	return ev, nil
}

func saveEvaluator(cfg Config, ev Evaluator) error {
	if !validID(ev.ID) {
		return fmt.Errorf(tr("identificador inválido: %q", "invalid id: %q"), ev.ID)
	}
	if err := validateEvaluator(ev); err != nil {
		return err
	}
	return writeYAML(evaluatorPath(cfg, ev.ID), ev)
}

func validateEvaluator(ev Evaluator) error {
	if strings.TrimSpace(ev.Name) == "" {
		return errors.New(tr("o avaliador precisa de um nome", "the evaluator needs a name"))
	}
	switch ev.Kind {
	case "ragas":
		switch ev.Metric {
		case "faithfulness", "answer_relevancy", "context_precision":
		default:
			return fmt.Errorf(tr("métrica Ragas desconhecida: %q", "unknown Ragas metric: %q"), ev.Metric)
		}
	case "rule":
		switch ev.Metric {
		case "exact", "contains", "regex":
		default:
			return fmt.Errorf(tr("tipo de regra desconhecido: %q", "unknown rule type: %q"), ev.Metric)
		}
		if ev.Metric == "regex" {
			if _, err := regexp.Compile(ev.Pattern); err != nil {
				return fmt.Errorf(tr("expressão regular inválida: %w", "invalid regular expression: %w"), err)
			}
		}
	case "llm_judge":
		if strings.TrimSpace(ev.Rubric) == "" {
			return errors.New(tr("o juiz LLM precisa de uma rubrica", "the LLM judge needs a rubric"))
		}
	case "embedding":
	case "decision":
		return validateDecisionQuestion(ev.Question)
	default:
		return fmt.Errorf(tr("tipo de avaliador desconhecido: %q", "unknown evaluator type: %q"), ev.Kind)
	}
	return nil
}

// ---------- Helpers ----------

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// validID protects against path traversal: ids become file names.
func validID(id string) bool { return idPattern.MatchString(id) }

// slugify turns "Resumo de contratos PT-BR" into "resumo-de-contratos-pt-br".
func slugify(name string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "í", "i", "ì", "i",
		"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ú", "u", "ü", "u", "ç", "c",
	)
	s := replacer.Replace(strings.ToLower(name))
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}
	if slug == "" {
		slug = "experimento"
	}
	return slug
}

// uniqueID returns slug, or slug-2, slug-3... if a file with that name already exists.
func uniqueID(dir, slug string) string {
	id := slug
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, id+".yaml")); errors.Is(err, os.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", slug, i)
	}
}

// writeYAML writes atomically (temp file + rename), so a crash never leaves half a file.
func writeYAML(path string, value any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes(), 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
