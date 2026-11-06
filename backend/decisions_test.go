package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ptr(v float64) *float64 { return &v }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDecisionScore(t *testing.T) {
	// yes/no: the score is the probability of yes.
	s, reason, err := decisionScore(DecisionQuestion{Type: "noul"}, DecisionAnswer{Noul: ptr(0.87)})
	if err != nil || !near(s, 0.87) || !strings.HasPrefix(reason, "Sim") {
		t.Fatalf("noul: %v %q %v", s, reason, err)
	}

	// choice: sum of the probabilities of the passing labels.
	choice := DecisionQuestion{Type: "choice", Options: []DecisionOption{
		{Label: "correct", Pass: true}, {Label: "partial", Pass: true}, {Label: "wrong"},
	}}
	s, _, err = decisionScore(choice, DecisionAnswer{Choice: "correct", Probabilities: map[string]float64{
		"correct": 0.6, "partial": 0.3, "wrong": 0.1}, Confidence: ptr(0.2)})
	if err != nil || !near(s, 0.9) {
		t.Fatalf("choice: %v %v", s, err)
	}

	// score: expected level / highest level.
	levels := DecisionQuestion{Type: "score", Options: []DecisionOption{{Description: "Ruim"}, {Description: "Ok"}, {Description: "Ótimo"}}}
	s, reason, err = decisionScore(levels, DecisionAnswer{Score: ptr(1.5)})
	if err != nil || !near(s, 0.75) || !strings.Contains(reason, "Ótimo") {
		t.Fatalf("score: %v %q %v", s, reason, err)
	}
}

func TestValidateDecisionQuestion(t *testing.T) {
	bad := []DecisionQuestion{
		{Type: "noul"}, // no instructions
		{Type: "choice", Instructions: "x", Options: []DecisionOption{{Label: "a", Pass: true}}},                 // 1 option
		{Type: "choice", Instructions: "x", Options: []DecisionOption{{Label: "a"}, {Label: "b"}}},               // nothing passes
		{Type: "choice", Instructions: "x", Options: []DecisionOption{{Label: "a", Pass: true}, {Label: "a"}}},   // repeated
		{Type: "score", Instructions: "x", Options: []DecisionOption{{Description: "baixo"}, {Description: ""}}}, // empty level
		{Type: "talvez", Instructions: "x"},
	}
	for i, q := range bad {
		if validateDecisionQuestion(q) == nil {
			t.Fatalf("case %d should be invalid", i)
		}
	}
	if err := validateDecisionQuestion(DecisionQuestion{Type: "noul", Instructions: "Está correto?"}); err != nil {
		t.Fatal(err)
	}
}

// The request must follow the System One format, with the key only for TypeSafe.
func TestCallDecision(t *testing.T) {
	var body map[string]any
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		io.WriteString(w, `{"model":"jev","answers":{"q":{"type":"choice","choice":"sim","probabilities":{"sim":0.8,"nao":0.2},"confidence":0.4}},"usage":{"input_tokens":10}}`)
	}))
	defer server.Close()

	t.Setenv("TEST_TYPESAFE_KEY", "segredo")
	q := DecisionQuestion{Type: "choice", Instructions: "Concorda?", Options: []DecisionOption{
		{Label: "sim", Description: "Concorda", Pass: true}, {Label: "nao"},
	}}
	answer, err := callDecision(context.Background(), ProviderConfig{Kind: "typesafe", BaseURL: server.URL, APIKeyEnv: "TEST_TYPESAFE_KEY"},
		"jev-latest", "texto", q, nil)
	if err != nil || answer.Choice != "sim" || answer.Probabilities["sim"] != 0.8 {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	if auth != "Bearer segredo" {
		t.Fatalf("auth header = %q", auth)
	}
	question := body["questions"].(map[string]any)["q"].(map[string]any)
	criteria := question["criteria"].(map[string]any)
	if body["state"] != "texto" || question["type"] != "choice" || criteria["sim"] != "Concorda" || criteria["nao"] != nil {
		t.Fatalf("unexpected body: %v", body)
	}

	// Local Ollama: no Authorization header.
	if _, err := callDecision(context.Background(), ProviderConfig{Kind: "ollama", BaseURL: server.URL}, "laya", "t", q, nil); err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Fatalf("ollama should not send a key, got %q", auth)
	}

	// Text providers can't answer decisions.
	if _, err := callDecision(context.Background(), ProviderConfig{Kind: "openai"}, "x", "t", q, nil); err == nil {
		t.Fatal("openai should be refused")
	}
}
