package main

// evaluator_tests.go: testing an evaluator on a spreadsheet, without an experiment.
//
// The person picks a spreadsheet (CSV/JSONL in the datasets folder) and says
// which column holds the answer to judge, and optionally the input, the
// expected answer, the context, the images and a HUMAN VERDICT (sim/não, 1/0…).
// Every row is scored by the evaluator. When there is a human verdict, the
// summary shows how often the evaluator agrees with the human, and which
// threshold would agree the most.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
)

// SheetColumns maps spreadsheet columns to what the evaluator reads.
// Only Answer is required.
type SheetColumns struct {
	Answer    string `json:"answer"`
	Question  string `json:"question"`
	Reference string `json:"reference"`
	Context   string `json:"context"`
	Images    string `json:"images"`
	Human     string `json:"human"` // human verdict: sim/não, 1/0, true/false, aprovado/reprovado, or a 0–1 number
}

type EvaluatorTests struct {
	db      *sql.DB
	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
}

func newEvaluatorTests(db *sql.DB) *EvaluatorTests {
	return &EvaluatorTests{db: db, cancels: map[int64]context.CancelFunc{}}
}

// start checks everything, records the test and scores the rows in the background.
func (t *EvaluatorTests) start(cfg Config, ev Evaluator, file string, cols SheetColumns) (int64, error) {
	ds, err := loadDataset(cfg, file)
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, c := range ds.Columns {
		known[c] = true
	}
	if cols.Answer == "" {
		return 0, errors.New(tr("escolha a coluna com a resposta a ser avaliada", "choose the column with the answer to evaluate"))
	}
	for _, col := range []string{cols.Answer, cols.Question, cols.Reference, cols.Context, cols.Images, cols.Human} {
		if col != "" && !known[col] {
			return 0, fmt.Errorf(tr("a coluna %q não existe em %s", "column %q does not exist in %s"), col, file)
		}
	}
	// Reuse the experiment checks: a fake experiment that "has" the mapped columns.
	exp := Experiment{Dataset: DatasetRef{ReferenceColumn: cols.Reference, ContextColumn: cols.Context}}
	if problems := evaluatorProblems(cfg, ev, exp); len(problems) > 0 {
		return 0, errors.New(problems[0])
	}
	if err := checkPythonService(cfg, []Evaluator{ev}); err != nil {
		return 0, err
	}
	if cols.Images != "" {
		for i, row := range ds.Rows {
			for _, ref := range imageRefs(row, []string{cols.Images}) {
				if err := checkImageRef(cfg, ref); err != nil {
					return 0, fmt.Errorf(tr("linha %d: %w", "line %d: %w"), i+1, err)
				}
			}
		}
	}

	columnsJSON, _ := json.Marshal(cols)
	evaluatorJSON, _ := json.Marshal(ev)
	id, err := insertEvaluatorTest(t.db, EvaluatorTest{
		EvaluatorID: ev.ID, Dataset: file, ColumnsJSON: string(columnsJSON), EvaluatorJSON: string(evaluatorJSON), Total: len(ds.Rows),
	})
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.cancels[id] = cancel
	t.mu.Unlock()
	go t.process(ctx, id, cfg, ev, ds.Rows, cols)
	return id, nil
}

func (t *EvaluatorTests) process(ctx context.Context, id int64, cfg Config, ev Evaluator, rows []map[string]string, cols SheetColumns) {
	defer func() {
		t.mu.Lock()
		delete(t.cancels, id)
		t.mu.Unlock()
	}()

	indexes := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indexes {
				t.scoreRow(id, cfg, ev, i, rows[i], cols)
			}
		}()
	}
	for i := range rows {
		select {
		case indexes <- i:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(indexes)
	wg.Wait()

	status := "done"
	if ctx.Err() != nil {
		status = "canceled"
	}
	if err := finishEvaluatorTest(t.db, id, status); err != nil {
		log.Printf("evaluator test %d: %v", id, err)
	}
}

func (t *EvaluatorTests) scoreRow(id int64, cfg Config, ev Evaluator, index int, row map[string]string, cols SheetColumns) {
	item := evalItem{
		Question:  row[cols.Question],
		Answer:    row[cols.Answer],
		Reference: row[cols.Reference],
		Context:   row[cols.Context],
	}
	if item.Context == "" {
		item.Context = item.Question // same rule as in experiments
	}
	if cols.Images != "" {
		item.Images = imageRefs(row, []string{cols.Images})
	}
	score := scoreItem(cfg, ev, item)
	err := insertTestRow(t.db, id, TestRow{
		RowIndex: index, Question: item.Question, Answer: item.Answer, Reference: item.Reference,
		Human: row[cols.Human], Score: score.Score, Reason: score.Reason, Error: score.Error,
	})
	if err != nil {
		log.Printf("evaluator test %d: could not save row %d: %v", id, index, err)
	}
}

func (t *EvaluatorTests) cancel(id int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	cancel, ok := t.cancels[id]
	if ok {
		cancel()
	}
	return ok
}

// ---------- Human verdicts and agreement ----------

// parseHumanVerdict reads what a person wrote in the verdict column.
// It returns nil when the cell is empty or not understood (the row is then
// left out of the agreement numbers).
func parseHumanVerdict(value string) *bool {
	v := strings.ToLower(strings.TrimSpace(value))
	yes, no := true, false
	switch v {
	case "1", "sim", "s", "yes", "y", "true", "verdadeiro", "v", "aprovado", "aprovada", "ok",
		"correto", "correta", "certo", "certa", "pass", "passou", "bom", "boa", "good", "correct":
		return &yes
	case "0", "não", "nao", "n", "no", "false", "falso", "f", "reprovado", "reprovada",
		"incorreto", "incorreta", "errado", "errada", "fail", "falhou", "ruim", "bad", "wrong", "incorrect":
		return &no
	}
	// A number between 0 and 1 (also "0,8"): passes from 0.5 up.
	if number, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64); err == nil && number >= 0 && number <= 1 {
		pass := number >= 0.5
		return &pass
	}
	return nil
}

// markTestRows fills Passed and Agrees on each row, so the UI doesn't need to
// repeat the verdict rules.
func markTestRows(rows []TestRow, threshold float64) {
	for i := range rows {
		if rows[i].Score == nil {
			continue
		}
		passed := *rows[i].Score >= threshold
		rows[i].Passed = &passed
		if human := parseHumanVerdict(rows[i].Human); human != nil {
			agrees := passed == *human
			rows[i].Agrees = &agrees
		}
	}
}

type TestSummary struct {
	Rows      int      `json:"rows"`
	Scored    int      `json:"scored"`
	Errors    int      `json:"errors"`
	MeanScore *float64 `json:"mean_score"`
	PassRate  *float64 `json:"pass_rate"` // share of scored rows at or above the threshold
	Threshold float64  `json:"threshold"`

	// Only when there is a human verdict column.
	Labeled   int      `json:"labeled"`   // rows with a score AND an understood human verdict
	Agreement *float64 `json:"agreement"` // share of labeled rows where evaluator and human agree
	// Confusion counts. "Positive" = approved.
	BothPass  int `json:"both_pass"`  // evaluator passes, human passes
	OnlyEval  int `json:"only_eval"`  // evaluator passes, human fails (too lenient)
	OnlyHuman int `json:"only_human"` // evaluator fails, human passes (too strict)
	BothFail  int `json:"both_fail"`  // evaluator fails, human fails
	Unlabeled int `json:"unlabeled"`  // scored rows whose verdict was empty or not understood
	// The threshold (0.05 steps) that would agree the most with the human.
	BestThreshold *float64 `json:"best_threshold"`
	BestAgreement *float64 `json:"best_agreement"`
}

func summarizeTest(rows []TestRow, threshold float64, hasHuman bool) TestSummary {
	s := TestSummary{Rows: len(rows), Threshold: threshold}
	sum, passed := 0.0, 0
	type labeled struct {
		score float64
		human bool
	}
	var pairs []labeled
	for _, r := range rows {
		if r.Score == nil {
			s.Errors++
			continue
		}
		s.Scored++
		sum += *r.Score
		evalPass := *r.Score >= threshold
		if evalPass {
			passed++
		}
		if !hasHuman {
			continue
		}
		human := parseHumanVerdict(r.Human)
		if human == nil {
			s.Unlabeled++
			continue
		}
		pairs = append(pairs, labeled{*r.Score, *human})
		switch {
		case evalPass && *human:
			s.BothPass++
		case evalPass && !*human:
			s.OnlyEval++
		case !evalPass && *human:
			s.OnlyHuman++
		default:
			s.BothFail++
		}
	}
	if s.Scored > 0 {
		mean := sum / float64(s.Scored)
		rate := float64(passed) / float64(s.Scored)
		s.MeanScore, s.PassRate = &mean, &rate
	}
	s.Labeled = len(pairs)
	if s.Labeled == 0 {
		return s
	}
	agreement := float64(s.BothPass+s.BothFail) / float64(s.Labeled)
	s.Agreement = &agreement

	// Try thresholds 0.05, 0.10 … 0.95 and keep the one that agrees most.
	bestT, bestA := 0.0, -1.0
	for step := 1; step <= 19; step++ {
		candidate := math.Round(float64(step)*5) / 100 // 0.05, 0.10 … without float noise
		agree := 0
		for _, p := range pairs {
			if (p.score >= candidate) == p.human {
				agree++
			}
		}
		if a := float64(agree) / float64(len(pairs)); a > bestA {
			bestT, bestA = candidate, a
		}
	}
	s.BestThreshold, s.BestAgreement = &bestT, &bestA
	return s
}
