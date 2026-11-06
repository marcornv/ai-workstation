package main

import "testing"

func TestEnglishMessages(t *testing.T) {
	setLanguage("en")
	defer setLanguage("pt-BR")

	_, reason, err := decisionScore(DecisionQuestion{Type: "noul"}, DecisionAnswer{Noul: ptr(0.87)})
	if err != nil || reason != "Yes · probability of yes 0.87" {
		t.Fatalf("got %q %v", reason, err)
	}
	if _, err := parseDataset("x.txt", nil); err == nil || err.Error() != "unsupported format: use .csv or .jsonl" {
		t.Fatalf("got %v", err)
	}
	cfg, err := parseConfig([]byte("language: en\n"))
	if err != nil || cfg.Language != "en" {
		t.Fatalf("got %+v %v", cfg.Language, err)
	}
	if _, err := parseConfig([]byte("language: fr\n")); err == nil {
		t.Fatal("unknown language should fail")
	}
}
