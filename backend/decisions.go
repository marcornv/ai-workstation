package main

// decisions.go: decision models (Laya, AutoTrust JEV on Ollama; TypeSafe's Jev).
//
// A decision model doesn't write text: it reads a text (the "state") and answers
// a question with probabilities. All of them speak the same API:
//
//	POST {base_url}/v1/systemone
//	{"model": "laya", "state": "...", "questions": {"q": {"type": "noul", ...}}}
//
// Three question types exist:
//
//	noul    yes/no            -> answer: probability of "yes" (0–1)
//	choice  pick one label    -> answer: label + probability of every label
//	score   ordered levels    -> answer: expected level (0..N-1) + probabilities
//
// A decision evaluator turns that answer into a 0–1 score (decisionScore).

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DecisionQuestion is stored in the evaluator's YAML file.
type DecisionQuestion struct {
	Type         string `yaml:"type" json:"type"` // noul | choice | score
	Instructions string `yaml:"instructions" json:"instructions"`
	// noul: what "yes" and "no" mean (optional but recommended).
	TrueMeans  string `yaml:"true_means,omitempty" json:"true_means"`
	FalseMeans string `yaml:"false_means,omitempty" json:"false_means"`
	// choice: the labels. score: the levels, lowest first (only Description is used).
	Options []DecisionOption `yaml:"options,omitempty" json:"options"`
}

type DecisionOption struct {
	Label       string `yaml:"label,omitempty" json:"label"`
	Description string `yaml:"description,omitempty" json:"description"`
	Pass        bool   `yaml:"pass,omitempty" json:"pass"` // choice: this label counts as a good answer
}

// DecisionAnswer is the model's answer to one question.
type DecisionAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Decision models like Laya only understand English, so the default text is in English.
const defaultStateTemplate = "Question:\n{question}\n\nModel answer:\n{answer}\n\nExpected answer:\n{reference}"

func validateDecisionQuestion(q DecisionQuestion) error {
	if strings.TrimSpace(q.Instructions) == "" {
		return errors.New(tr("escreva a pergunta que o modelo de decisão deve responder", "write the question the decision model should answer"))
	}
	switch q.Type {
	case "noul":
		return nil
	case "choice":
		if len(q.Options) < 2 || len(q.Options) > 26 {
			return errors.New(tr("uma escolha precisa de 2 a 26 opções", "a choice needs 2 to 26 options"))
		}
		seen := map[string]bool{}
		anyPass := false
		for _, o := range q.Options {
			label := strings.TrimSpace(o.Label)
			if label == "" || seen[label] {
				return errors.New(tr("cada opção precisa de um nome, sem repetir", "each option needs a unique name"))
			}
			seen[label] = true
			anyPass = anyPass || o.Pass
		}
		if !anyPass {
			return errors.New(tr("marque pelo menos uma opção como aprovada", "mark at least one option as passing"))
		}
		return nil
	case "score":
		if len(q.Options) < 2 || len(q.Options) > 10 {
			return errors.New(tr("uma nota precisa de 2 a 10 níveis", "a rating needs 2 to 10 levels"))
		}
		for _, o := range q.Options {
			if strings.TrimSpace(o.Description) == "" {
				return errors.New(tr("descreva cada nível da nota", "describe every rating level"))
			}
		}
		return nil
	}
	return fmt.Errorf(tr("tipo de pergunta desconhecido: %q (use noul, choice ou score)", "unknown question type: %q (use noul, choice or score)"), q.Type)
}

// apiQuestion converts our question into the API's JSON shape.
func apiQuestion(q DecisionQuestion) map[string]any {
	out := map[string]any{"type": q.Type, "instructions": q.Instructions}
	switch q.Type {
	case "noul":
		if q.TrueMeans != "" || q.FalseMeans != "" {
			out["criteria"] = map[string]string{"true": q.TrueMeans, "false": q.FalseMeans}
		}
	case "choice":
		criteria := map[string]any{}
		for _, o := range q.Options {
			if o.Description == "" {
				criteria[o.Label] = nil // the label describes itself
			} else {
				criteria[o.Label] = o.Description
			}
		}
		out["criteria"] = criteria
	case "score":
		var levels []string
		for _, o := range q.Options {
			levels = append(levels, o.Description)
		}
		out["criteria"] = levels
	}
	return out
}

// callDecision asks a decision model one question about `state`.
func callDecision(ctx context.Context, p ProviderConfig, model, state string, q DecisionQuestion, images []Image) (DecisionAnswer, error) {
	if p.Kind != "ollama" && p.Kind != "typesafe" {
		return DecisionAnswer{}, fmt.Errorf(tr("o provedor %q não tem modelos de decisão (use Ollama ou TypeSafe)", "provider %q has no decision models (use Ollama or TypeSafe)"), p.Kind)
	}
	body := map[string]any{
		"model":     model,
		"state":     state,
		"questions": map[string]any{"q": apiQuestion(q)},
	}
	if len(images) > 0 {
		var encoded []string
		for _, img := range images {
			encoded = append(encoded, base64.StdEncoding.EncodeToString(img.Data))
		}
		body["images"] = encoded
	}
	headers := map[string]string{}
	if key := apiKey(p); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	fallback := "http://localhost:11434"
	if p.Kind == "typesafe" {
		fallback = "https://api.typesafe.ai"
	}

	var out struct {
		Answers map[string]DecisionAnswer `json:"answers"`
	}
	if err := postJSON(ctx, baseURL(p, fallback)+"/v1/systemone", headers, body, &out); err != nil {
		if p.Kind == "ollama" && strings.Contains(err.Error(), "not found") {
			return DecisionAnswer{}, fmt.Errorf(tr("o modelo %s não está baixado no Ollama: baixe em Configurações › Baixar modelo", "model %s is not downloaded in Ollama: download it in Settings › Download model"), model)
		}
		return DecisionAnswer{}, err
	}
	answer, ok := out.Answers["q"]
	if !ok {
		return DecisionAnswer{}, errors.New(tr("o modelo de decisão não respondeu à pergunta", "the decision model did not answer the question"))
	}
	return answer, nil
}

// decisionScore turns an answer into a 0–1 score and a short explanation.
//
//	noul:   probability of "yes"
//	choice: sum of the probabilities of the labels marked as passing
//	score:  expected level / highest level
func decisionScore(q DecisionQuestion, a DecisionAnswer) (float64, string, error) {
	var score float64
	var reason string
	switch q.Type {
	case "noul":
		if a.Noul == nil {
			return 0, "", errors.New(tr("resposta sem a probabilidade de sim", "answer without the probability of yes"))
		}
		score = *a.Noul
		verdict := tr("Não", "No")
		if score >= 0.5 {
			verdict = tr("Sim", "Yes")
		}
		reason = fmt.Sprintf(tr("%s · probabilidade de sim %s", "%s · probability of yes %s"), verdict, fmtProb(score))
	case "choice":
		passing := map[string]bool{}
		for _, o := range q.Options {
			passing[o.Label] = o.Pass
		}
		if len(a.Probabilities) > 0 {
			for label, p := range a.Probabilities {
				if passing[label] {
					score += p
				}
			}
		} else if passing[a.Choice] {
			score = 1
		}
		reason = fmt.Sprintf(tr("Escolheu %q · %s", "Chose %q · %s"), a.Choice, fmtProbabilities(a.Probabilities))
	case "score":
		if a.Score == nil || len(q.Options) < 2 {
			return 0, "", errors.New(tr("resposta sem nota", "answer without a rating"))
		}
		top := float64(len(q.Options) - 1)
		score = *a.Score / top
		level := int(*a.Score + 0.5)
		if level >= 0 && level < len(q.Options) {
			reason = fmt.Sprintf(tr("Nível %s de %d (mais perto de %q)", "Level %s of %d (closest to %q)"), fmtProb(*a.Score), len(q.Options)-1, q.Options[level].Description)
		}
	default:
		return 0, "", fmt.Errorf(tr("tipo de pergunta desconhecido: %q", "unknown question type: %q"), q.Type)
	}
	if a.Confidence != nil {
		reason += tr(" · confiança ", " · confidence ") + fmtProb(*a.Confidence)
		if *a.Confidence < 0.3 {
			reason += tr(" (baixa: o modelo está em dúvida)", " (low: the model is unsure)")
		}
	}
	return min(max(score, 0), 1), reason, nil
}

// fmtProb formats 0.8666 as "0,87" in Portuguese and "0.87" in English.
func fmtProb(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 2, 64), ".", tr(",", "."), 1)
}

// fmtProbabilities lists labels from most to least likely: "billing 0,94 · other 0,01".
func fmtProbabilities(probs map[string]float64) string {
	labels := make([]string, 0, len(probs))
	for label := range probs {
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool { return probs[labels[i]] > probs[labels[j]] })
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		parts = append(parts, label+" "+fmtProb(probs[label]))
	}
	return strings.Join(parts, " · ")
}
