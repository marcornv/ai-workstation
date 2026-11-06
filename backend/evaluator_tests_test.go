package main

import "testing"

func TestParseHumanVerdict(t *testing.T) {
	cases := map[string]*bool{"Sim": ptrBool(true), " NÃO ": ptrBool(false), "1": ptrBool(true), "0,2": ptrBool(false),
		"aprovado": ptrBool(true), "errada": ptrBool(false), "": nil, "talvez": nil, "7": nil}
	for in, want := range cases {
		got := parseHumanVerdict(in)
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
	}
}

func ptrBool(b bool) *bool { return &b }

func TestSummarizeTest(t *testing.T) {
	rows := []TestRow{
		{Score: ptr(0.9), Human: "sim"},    // both pass
		{Score: ptr(0.6), Human: "não"},    // evaluator too lenient at 0.5
		{Score: ptr(0.2), Human: "não"},    // both fail
		{Score: ptr(0.4), Human: "sim"},    // evaluator too strict at 0.5
		{Score: ptr(0.3), Human: "talvez"}, // not understood
		{Error: "falhou"},                  // no score
	}
	s := summarizeTest(rows, 0.5, true)
	if s.Rows != 6 || s.Scored != 5 || s.Errors != 1 || s.Labeled != 4 || s.Unlabeled != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if s.BothPass != 1 || s.OnlyEval != 1 || s.BothFail != 1 || s.OnlyHuman != 1 || !near(*s.Agreement, 0.5) {
		t.Fatalf("confusion: %+v", s)
	}
	// Scores 0.9 (sim), 0.6 (não), 0.4 (sim), 0.2 (não) can't be fully separated:
	// the best threshold agrees on 3 of 4.
	if s.BestAgreement == nil || !near(*s.BestAgreement, 0.75) {
		t.Fatalf("best: %v %v", s.BestThreshold, s.BestAgreement)
	}
	// Without a human column there are no agreement numbers.
	if s := summarizeTest(rows, 0.5, false); s.Agreement != nil || s.Labeled != 0 {
		t.Fatalf("no human column: %+v", s)
	}
}
