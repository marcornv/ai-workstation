package main

// dataset.go: reading input data (CSV or JSONL) and filling prompt templates.

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Dataset is a table: every row maps a column name to its text value.
type Dataset struct {
	File    string              `json:"file"`
	Columns []string            `json:"columns"`
	Rows    []map[string]string `json:"rows"`
}

const maxDatasetBytes = 50 << 20 // 50 MB is plenty for prompt experiments

func datasetPath(cfg Config, file string) (string, error) {
	// filepath.Base removes any "../" so a file name can't escape the folder.
	name := filepath.Base(file)
	if name != file || name == "." || name == "" {
		return "", fmt.Errorf(tr("nome de arquivo inválido: %q", "invalid file name: %q"), file)
	}
	return filepath.Join(cfg.Paths.Datasets, name), nil
}

func loadDataset(cfg Config, file string) (Dataset, error) {
	if file == "" {
		return Dataset{}, errors.New(tr("nenhum arquivo de dados selecionado", "no data file selected"))
	}
	path, err := datasetPath(cfg, file)
	if err != nil {
		return Dataset{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, fmt.Errorf(tr("não foi possível ler %s: %w", "could not read %s: %w"), file, err)
	}
	ds, err := parseDataset(file, data)
	if err != nil {
		return Dataset{}, err
	}
	ds.File = file
	return ds, nil
}

func parseDataset(file string, data []byte) (Dataset, error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".csv":
		return parseCSV(data)
	case ".jsonl", ".ndjson":
		return parseJSONL(data)
	}
	return Dataset{}, errors.New(tr("formato não suportado: use .csv ou .jsonl", "unsupported format: use .csv or .jsonl"))
}

func parseCSV(data []byte) (Dataset, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // Excel adds a BOM
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	// Excel in Brazil often saves with ";" — detect it from the header line.
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		reader.Comma = ';'
	}
	records, err := reader.ReadAll()
	if err != nil {
		return Dataset{}, fmt.Errorf(tr("CSV inválido: %w", "invalid CSV: %w"), err)
	}
	if len(records) < 2 {
		return Dataset{}, errors.New(tr("o CSV precisa de uma linha de cabeçalho e pelo menos uma linha de dados", "the CSV needs a header line and at least one data line"))
	}
	header := records[0]
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	ds := Dataset{Columns: header}
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, col := range header {
			if i < len(rec) {
				row[col] = rec[i]
			}
		}
		ds.Rows = append(ds.Rows, row)
	}
	return ds, nil
}

func parseJSONL(data []byte) (Dataset, error) {
	ds := Dataset{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024*1024), maxDatasetBytes)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return Dataset{}, fmt.Errorf(tr("linha %d não é um JSON válido: %w", "line %d is not valid JSON: %w"), lineNo, err)
		}
		row := map[string]string{}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			row[k] = valueToString(obj[k])
			if !seen[k] {
				seen[k] = true
				ds.Columns = append(ds.Columns, k)
			}
		}
		ds.Rows = append(ds.Rows, row)
	}
	if err := scanner.Err(); err != nil {
		return Dataset{}, err
	}
	if len(ds.Rows) == 0 {
		return Dataset{}, errors.New(tr("o arquivo não tem nenhuma linha", "the file has no lines"))
	}
	return ds, nil
}

// valueToString keeps strings as they are and turns anything else into JSON text.
func valueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// textToJSONL turns pasted text into a JSONL dataset with one column "input".
// Inputs are separated by a blank line, so a single input can span many lines.
func textToJSONL(text string) ([]byte, int) {
	var out bytes.Buffer
	count := 0
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		line, _ := json.Marshal(map[string]string{"input": block})
		out.Write(line)
		out.WriteByte('\n')
		count++
	}
	return out.Bytes(), count
}

// ---------- Templates ----------

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z0-9_\-]+)\}`)

// fillTemplate replaces {column} with the row's value. Placeholders that are not
// dataset columns are left untouched (so JSON examples in prompts keep working).
func fillTemplate(template string, row map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		col := match[1 : len(match)-1]
		if value, ok := row[col]; ok {
			return value
		}
		return match
	})
}

// templateColumns lists the {column} names used in a template.
func templateColumns(template string) []string {
	var cols []string
	for _, m := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		cols = append(cols, m[1])
	}
	return cols
}
