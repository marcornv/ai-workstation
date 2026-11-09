package main

import (
	"testing"
)

func TestFillTemplate(t *testing.T) {
	row := map[string]string{"contrato": "ABC"}
	got := fillTemplate(`Contrato: {contrato} JSON: {"nota": 1} {desconhecida}`, row)
	want := `Contrato: ABC JSON: {"nota": 1} {desconhecida}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseCSVSemicolon(t *testing.T) {
	ds, err := parseDataset("x.csv", []byte("\xef\xbb\xbfpergunta;resposta\nOi?;Olá\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 1 || ds.Rows[0]["resposta"] != "Olá" {
		t.Fatalf("unexpected dataset: %+v", ds)
	}
}

func TestParseJSONL(t *testing.T) {
	ds, err := parseDataset("x.jsonl", []byte("{\"a\":\"1\",\"n\":2}\n\n{\"a\":\"3\"}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["n"] != "2" || len(ds.Columns) != 2 {
		t.Fatalf("unexpected dataset: %+v", ds)
	}
}

func TestTextToJSONL(t *testing.T) {
	_, n := textToJSONL("primeiro\nainda primeiro\n\nsegundo\n\n\n")
	if n != 2 {
		t.Fatalf("got %d inputs, want 2", n)
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if p := percentile(values, 50); p != 50 {
		t.Fatalf("p50 = %d", p)
	}
	if p := percentile(values, 95); p != 100 {
		t.Fatalf("p95 = %d", p)
	}
	if p := percentile(nil, 50); p != 0 {
		t.Fatalf("empty p50 = %d", p)
	}
}

func TestSlugifyAndValidID(t *testing.T) {
	if s := slugify("Resumo de Contratos — PT-BR!"); s != "resumo-de-contratos-pt-br" {
		t.Fatalf("slug = %q", s)
	}
	for _, bad := range []string{"../etc", "A", "", "a/b"} {
		if validID(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cfg, err := parseConfig([]byte("server:\n  port: 9000\nproviders:\n  local:\n    kind: ollama\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9000 || cfg.Providers["local"].Kind != "ollama" || cfg.Paths.Experiments != "experiments" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := parseConfig([]byte("providers:\n  x:\n    kind: banana\n")); err == nil {
		t.Fatal("unknown provider kind should fail")
	}
}

func TestSummarize(t *testing.T) {
	cost := 0.01
	score := 0.8
	results := []Result{
		{ID: 1, Model: "m", Provider: "p", InputTokens: 10, OutputTokens: 5, LatencyMs: 100, CostUSD: &cost},
		{ID: 2, Model: "m", Provider: "p", Error: "falhou", CostUSD: &cost},
	}
	s := summarize(results, []Score{{ResultID: 1, EvaluatorID: "e", Score: &score}})
	if s.Calls != 2 || s.Errors != 1 || len(s.Models) != 1 || *s.Models[0].MeanScore != 0.8 || s.Models[0].LatencyP50 != 100 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestSetJSONLColumn(t *testing.T) {
	in := []byte("{\"imagem\":\"images/a/1.png\",\"n\":1}\n\n{\"imagem\":\"images/a/2.png\",\"rotulo\":\"old\"}\n")
	out, err := setJSONLColumn(in, "rotulo", []string{"cat", "dog"})
	if err != nil {
		t.Fatal(err)
	}
	ds, err := parseJSONL(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["rotulo"] != "cat" || ds.Rows[1]["rotulo"] != "dog" {
		t.Fatalf("labels not set: %+v", ds.Rows)
	}
	if ds.Rows[0]["n"] != "1" || ds.Rows[1]["imagem"] != "images/a/2.png" {
		t.Fatalf("other fields changed: %+v", ds.Rows)
	}
	if _, err := setJSONLColumn(in, "rotulo", []string{"only one"}); err == nil {
		t.Fatal("expected an error when values and rows don't match")
	}
	if _, err := setJSONLColumn(in, "rotulo", []string{"a", "b", "c"}); err == nil {
		t.Fatal("expected an error with more values than rows")
	}
}
