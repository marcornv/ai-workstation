package main

// store.go: the SQLite database that keeps runs, model answers and scores.
// Configuration lives in YAML files; only results live here.

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go SQLite driver, no C compiler needed
)

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	experiment  TEXT NOT NULL,
	status      TEXT NOT NULL,          -- running | done | canceled | failed
	total       INTEGER NOT NULL,       -- number of calls planned
	started_at  TEXT NOT NULL,
	finished_at TEXT,
	config_json TEXT NOT NULL           -- copy of the experiment as it was when the run started
);

CREATE TABLE IF NOT EXISTS results (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id        INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
	input_index   INTEGER NOT NULL,
	repetition    INTEGER NOT NULL,
	provider      TEXT NOT NULL,
	model         TEXT NOT NULL,
	user_prompt   TEXT NOT NULL,        -- the prompt after filling {columns}
	reference     TEXT NOT NULL DEFAULT '',
	context       TEXT NOT NULL DEFAULT '',
	images        TEXT NOT NULL DEFAULT '[]', -- JSON list of image paths/URLs sent with the prompt
	output        TEXT NOT NULL DEFAULT '',
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	latency_ms    INTEGER NOT NULL DEFAULT 0,
	cost_usd      REAL,                 -- NULL when the model has no price configured
	error         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS results_run ON results(run_id, input_index);

CREATE TABLE IF NOT EXISTS scores (
	result_id    INTEGER NOT NULL REFERENCES results(id) ON DELETE CASCADE,
	evaluator_id TEXT NOT NULL,
	score        REAL,                  -- NULL when evaluation failed
	reason       TEXT NOT NULL DEFAULT '',
	error        TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (result_id, evaluator_id)
);

-- Spreadsheet tests of an evaluator (evaluator_tests.go): run one evaluator on
-- every row of a spreadsheet, optionally against a human verdict column.
CREATE TABLE IF NOT EXISTS evaluator_tests (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	evaluator_id   TEXT NOT NULL,
	dataset        TEXT NOT NULL,
	columns_json   TEXT NOT NULL,      -- which column is the answer, the input, the human verdict…
	evaluator_json TEXT NOT NULL,      -- copy of the evaluator as it was when the test started
	status         TEXT NOT NULL,      -- running | done | canceled
	total          INTEGER NOT NULL,
	started_at     TEXT NOT NULL,
	finished_at    TEXT
);

CREATE TABLE IF NOT EXISTS evaluator_test_rows (
	test_id   INTEGER NOT NULL REFERENCES evaluator_tests(id) ON DELETE CASCADE,
	row_index INTEGER NOT NULL,
	question  TEXT NOT NULL DEFAULT '',
	answer    TEXT NOT NULL DEFAULT '',
	reference TEXT NOT NULL DEFAULT '',
	human     TEXT NOT NULL DEFAULT '',  -- the human verdict as written in the spreadsheet
	score     REAL,
	reason    TEXT NOT NULL DEFAULT '',
	error     TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (test_id, row_index)
);
`

type Run struct {
	ID         int64   `json:"id"`
	Experiment string  `json:"experiment"`
	Status     string  `json:"status"`
	Total      int     `json:"total"`
	Done       int     `json:"done"`
	Errors     int     `json:"errors"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	ConfigJSON string  `json:"-"`
}

type Result struct {
	ID           int64    `json:"id"`
	RunID        int64    `json:"run_id"`
	InputIndex   int      `json:"input_index"`
	Repetition   int      `json:"repetition"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	UserPrompt   string   `json:"user_prompt"`
	Reference    string   `json:"reference"`
	Context      string   `json:"context"`
	Images       []string `json:"images"`
	Output       string   `json:"output"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	LatencyMs    int64    `json:"latency_ms"`
	CostUSD      *float64 `json:"cost_usd"`
	Error        string   `json:"error"`
}

type Score struct {
	ResultID    int64    `json:"result_id"`
	EvaluatorID string   `json:"evaluator_id"`
	Score       *float64 `json:"score"`
	Reason      string   `json:"reason"`
	Error       string   `json:"error"`
}

func openDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// One connection keeps SQLite simple: no "database is locked" surprises.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	// Databases created before image support don't have this column yet.
	if _, err := db.Exec(`ALTER TABLE results ADD COLUMN images TEXT NOT NULL DEFAULT '[]'`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return nil, err
	}
	// Runs and tests left "running" by a crash or a closed app can never finish.
	if _, err := db.Exec(`UPDATE runs SET status = 'canceled', finished_at = ? WHERE status = 'running'`, now()); err != nil {
		return nil, err
	}
	_, err = db.Exec(`UPDATE evaluator_tests SET status = 'canceled', finished_at = ? WHERE status = 'running'`, now())
	return db, err
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------- Runs ----------

func insertRun(db *sql.DB, experiment string, total int, configJSON string) (int64, error) {
	res, err := db.Exec(`INSERT INTO runs (experiment, status, total, started_at, config_json) VALUES (?, 'running', ?, ?, ?)`,
		experiment, total, now(), configJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func finishRun(db *sql.DB, runID int64, status string) error {
	_, err := db.Exec(`UPDATE runs SET status = ?, finished_at = ? WHERE id = ?`, status, now(), runID)
	return err
}

const runColumns = `r.id, r.experiment, r.status, r.total, r.started_at, r.finished_at, r.config_json,
	(SELECT COUNT(*) FROM results WHERE run_id = r.id),
	(SELECT COUNT(*) FROM results WHERE run_id = r.id AND error != '')`

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.Experiment, &r.Status, &r.Total, &r.StartedAt, &r.FinishedAt, &r.ConfigJSON, &r.Done, &r.Errors)
	return r, err
}

func getRun(db *sql.DB, runID int64) (Run, error) {
	return scanRun(db.QueryRow(`SELECT `+runColumns+` FROM runs r WHERE r.id = ?`, runID))
}

// listRuns returns the runs of one experiment, newest first.
func listRuns(db *sql.DB, experiment string) ([]Run, error) {
	rows, err := db.Query(`SELECT `+runColumns+` FROM runs r WHERE r.experiment = ? ORDER BY r.id DESC`, experiment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func deleteRunsOfExperiment(db *sql.DB, experiment string) error {
	_, err := db.Exec(`DELETE FROM runs WHERE experiment = ?`, experiment)
	return err
}

// ---------- Results ----------

func insertResult(db *sql.DB, r Result) error {
	if r.Images == nil {
		r.Images = []string{}
	}
	images, _ := json.Marshal(r.Images)
	_, err := db.Exec(`INSERT INTO results
		(run_id, input_index, repetition, provider, model, user_prompt, reference, context, images,
		 output, input_tokens, output_tokens, latency_ms, cost_usd, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RunID, r.InputIndex, r.Repetition, r.Provider, r.Model, r.UserPrompt, r.Reference, r.Context, string(images),
		r.Output, r.InputTokens, r.OutputTokens, r.LatencyMs, r.CostUSD, r.Error)
	return err
}

func listResults(db *sql.DB, runID int64) ([]Result, error) {
	rows, err := db.Query(`SELECT id, run_id, input_index, repetition, provider, model, user_prompt, reference, context, images,
		output, input_tokens, output_tokens, latency_ms, cost_usd, error
		FROM results WHERE run_id = ? ORDER BY input_index, model, repetition`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []Result{}
	for rows.Next() {
		var r Result
		var images string
		if err := rows.Scan(&r.ID, &r.RunID, &r.InputIndex, &r.Repetition, &r.Provider, &r.Model, &r.UserPrompt,
			&r.Reference, &r.Context, &images, &r.Output, &r.InputTokens, &r.OutputTokens, &r.LatencyMs, &r.CostUSD, &r.Error); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(images), &r.Images) != nil || r.Images == nil {
			r.Images = []string{}
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ---------- Scores ----------

func upsertScore(db *sql.DB, s Score) error {
	_, err := db.Exec(`INSERT INTO scores (result_id, evaluator_id, score, reason, error) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (result_id, evaluator_id) DO UPDATE SET score = excluded.score, reason = excluded.reason, error = excluded.error`,
		s.ResultID, s.EvaluatorID, s.Score, s.Reason, s.Error)
	return err
}

func listScores(db *sql.DB, runID int64) ([]Score, error) {
	rows, err := db.Query(`SELECT s.result_id, s.evaluator_id, s.score, s.reason, s.error
		FROM scores s JOIN results r ON r.id = s.result_id WHERE r.run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scores := []Score{}
	for rows.Next() {
		var s Score
		if err := rows.Scan(&s.ResultID, &s.EvaluatorID, &s.Score, &s.Reason, &s.Error); err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	return scores, rows.Err()
}

// ---------- Evaluator spreadsheet tests ----------

type EvaluatorTest struct {
	ID            int64   `json:"id"`
	EvaluatorID   string  `json:"evaluator_id"`
	Dataset       string  `json:"dataset"`
	ColumnsJSON   string  `json:"-"`
	EvaluatorJSON string  `json:"-"`
	Status        string  `json:"status"`
	Total         int     `json:"total"`
	Done          int     `json:"done"`
	StartedAt     string  `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
}

type TestRow struct {
	RowIndex  int      `json:"row_index"`
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	Reference string   `json:"reference"`
	Human     string   `json:"human"`
	Score     *float64 `json:"score"`
	Reason    string   `json:"reason"`
	Error     string   `json:"error"`
	// Filled when showing a test, not stored (see markTestRows).
	Passed *bool `json:"passed"` // score >= threshold
	Agrees *bool `json:"agrees"` // evaluator and human verdict agree
}

func insertEvaluatorTest(db *sql.DB, t EvaluatorTest) (int64, error) {
	res, err := db.Exec(`INSERT INTO evaluator_tests (evaluator_id, dataset, columns_json, evaluator_json, status, total, started_at)
		VALUES (?, ?, ?, ?, 'running', ?, ?)`, t.EvaluatorID, t.Dataset, t.ColumnsJSON, t.EvaluatorJSON, t.Total, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func finishEvaluatorTest(db *sql.DB, id int64, status string) error {
	_, err := db.Exec(`UPDATE evaluator_tests SET status = ?, finished_at = ? WHERE id = ?`, status, now(), id)
	return err
}

const testColumns = `t.id, t.evaluator_id, t.dataset, t.columns_json, t.evaluator_json, t.status, t.total,
	(SELECT COUNT(*) FROM evaluator_test_rows WHERE test_id = t.id), t.started_at, t.finished_at`

func scanEvaluatorTest(row interface{ Scan(...any) error }) (EvaluatorTest, error) {
	var t EvaluatorTest
	err := row.Scan(&t.ID, &t.EvaluatorID, &t.Dataset, &t.ColumnsJSON, &t.EvaluatorJSON, &t.Status, &t.Total, &t.Done, &t.StartedAt, &t.FinishedAt)
	return t, err
}

func getEvaluatorTest(db *sql.DB, id int64) (EvaluatorTest, error) {
	return scanEvaluatorTest(db.QueryRow(`SELECT `+testColumns+` FROM evaluator_tests t WHERE t.id = ?`, id))
}

// listEvaluatorTests returns the tests of one evaluator, newest first.
func listEvaluatorTests(db *sql.DB, evaluatorID string) ([]EvaluatorTest, error) {
	rows, err := db.Query(`SELECT `+testColumns+` FROM evaluator_tests t WHERE t.evaluator_id = ? ORDER BY t.id DESC`, evaluatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tests := []EvaluatorTest{}
	for rows.Next() {
		t, err := scanEvaluatorTest(rows)
		if err != nil {
			return nil, err
		}
		tests = append(tests, t)
	}
	return tests, rows.Err()
}

func deleteEvaluatorTest(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM evaluator_tests WHERE id = ?`, id)
	return err
}

func insertTestRow(db *sql.DB, testID int64, r TestRow) error {
	_, err := db.Exec(`INSERT INTO evaluator_test_rows (test_id, row_index, question, answer, reference, human, score, reason, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		testID, r.RowIndex, r.Question, r.Answer, r.Reference, r.Human, r.Score, r.Reason, r.Error)
	return err
}

func listTestRows(db *sql.DB, testID int64) ([]TestRow, error) {
	rows, err := db.Query(`SELECT row_index, question, answer, reference, human, score, reason, error
		FROM evaluator_test_rows WHERE test_id = ? ORDER BY row_index`, testID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TestRow{}
	for rows.Next() {
		var r TestRow
		if err := rows.Scan(&r.RowIndex, &r.Question, &r.Answer, &r.Reference, &r.Human, &r.Score, &r.Reason, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
