package main

// experiment_data.go: each experiment's own data folder.
//
// Files uploaded from an experiment (CSV, JSONL, pasted text, images) go into
// data/<experiment id>/, with images in data/<experiment id>/images/<name>/.
// The experiment owns that folder: its files can be deleted from the UI, and
// deleting the experiment deletes the folder. Files directly in data/ are
// shared (put there by hand, the examples, evaluator spreadsheet tests) and
// are never deleted by Bancada.
//
// Dataset names keep working as paths relative to data/: "contratos.csv" is
// shared, "my-experiment/contratos.csv" belongs to my-experiment. Image paths
// in the rows are relative to data/ too ("my-experiment/images/fotos/1.png").

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// sharedImagesDir is where images were uploaded before experiments had their
// own folders. An experiment can't use this id, or deleting it would delete
// those shared images.
const sharedImagesDir = "images"

func experimentDataDir(cfg Config, id string) string {
	return filepath.Join(cfg.Paths.Datasets, id)
}

// ownsDataDir is false for ids whose folder must never be treated as an
// experiment's (invalid ids and the old shared images folder).
func ownsDataDir(id string) bool {
	return validID(id) && id != sharedImagesDir
}

// newExperimentID picks a free id whose data folder doesn't exist yet, so a
// new experiment never adopts files left behind by someone else.
func newExperimentID(cfg Config, slug string) string {
	if slug == "" {
		slug = "experimento"
	}
	id := slug
	for i := 2; ; i++ {
		_, errYAML := os.Stat(experimentPath(cfg, id))
		_, errData := os.Stat(experimentDataDir(cfg, id))
		if ownsDataDir(id) && errors.Is(errYAML, os.ErrNotExist) && errors.Is(errData, os.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", slug, i)
	}
}

// datasetOwner returns the experiment that owns a dataset ("" = shared).
func datasetOwner(file string) string {
	owner, _, found := strings.Cut(file, "/")
	if !found {
		return ""
	}
	return owner
}

// listDatasetFiles lists shared datasets plus those in experiment folders.
// With experimentID, only that experiment's files are added (listed first).
func listDatasetFiles(cfg Config, experimentID string) []string {
	globAll := func(dir, prefix string) []string {
		files := []string{}
		for _, pattern := range []string{"*.csv", "*.jsonl", "*.ndjson"} {
			matches, _ := filepath.Glob(filepath.Join(dir, pattern))
			for _, m := range matches {
				files = append(files, prefix+filepath.Base(m))
			}
		}
		sort.Strings(files)
		return files
	}
	shared := globAll(cfg.Paths.Datasets, "")
	if experimentID != "" {
		if !ownsDataDir(experimentID) {
			return shared
		}
		return append(globAll(experimentDataDir(cfg, experimentID), experimentID+"/"), shared...)
	}
	owned := []string{}
	entries, _ := os.ReadDir(cfg.Paths.Datasets)
	for _, e := range entries {
		// Only folders of experiments that exist: anything else in data/ is not ours.
		if e.IsDir() && ownsDataDir(e.Name()) {
			if _, err := os.Stat(experimentPath(cfg, e.Name())); err == nil {
				owned = append(owned, globAll(filepath.Join(cfg.Paths.Datasets, e.Name()), e.Name()+"/")...)
			}
		}
	}
	return append(shared, owned...)
}

// uploadDir returns where an upload goes (data/ or data/<experiment>/) and the
// prefix for the dataset names created there.
func uploadDir(cfg Config, experimentID string) (dir, prefix string, err error) {
	if experimentID == "" {
		return cfg.Paths.Datasets, "", nil
	}
	if !ownsDataDir(experimentID) {
		return "", "", fmt.Errorf(tr("identificador inválido: %q", "invalid id: %q"), experimentID)
	}
	if _, err := os.Stat(experimentPath(cfg, experimentID)); err != nil {
		return "", "", fmt.Errorf(tr("o experimento %q não existe", "experiment %q does not exist"), experimentID)
	}
	dir = experimentDataDir(cfg, experimentID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	return dir, experimentID + "/", nil
}

// deleteExperimentDataset deletes a dataset owned by an experiment and, for
// an image upload, its images folder (same name as the dataset).
func deleteExperimentDataset(cfg Config, file string) error {
	owner := datasetOwner(file)
	if owner == "" || !ownsDataDir(owner) {
		return errors.New(tr("só é possível excluir arquivos que pertencem a um experimento", "only files that belong to an experiment can be deleted"))
	}
	path, err := datasetPath(cfg, file)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return os.RemoveAll(filepath.Join(experimentDataDir(cfg, owner), sharedImagesDir, base))
}

// DataUsage counts what an experiment's folder holds, to warn before deleting it.
type DataUsage struct {
	Files  int   `json:"files"`  // datasets (CSV/JSONL)
	Images int   `json:"images"` // image files
	Bytes  int64 `json:"bytes"`
}

func experimentDataUsage(cfg Config, id string) DataUsage {
	var usage DataUsage
	if !ownsDataDir(id) {
		return usage
	}
	filepath.WalkDir(experimentDataDir(cfg, id), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			usage.Bytes += info.Size()
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csv", ".jsonl", ".ndjson":
			usage.Files++
		default:
			if looksLikeImage(path) {
				usage.Images++
			}
		}
		return nil
	})
	return usage
}

func deleteExperimentData(cfg Config, id string) error {
	if !ownsDataDir(id) {
		return nil
	}
	return os.RemoveAll(experimentDataDir(cfg, id))
}

// copyExperimentData copies data/<from>/ into data/<to>/ when duplicating an
// experiment, so the copy keeps working if the original is deleted. Image paths
// inside the datasets ("<from>/images/...") are rewritten to the new folder.
func copyExperimentData(cfg Config, from, to string) error {
	if !ownsDataDir(from) || !ownsDataDir(to) {
		return nil
	}
	src, dst := experimentDataDir(cfg, from), experimentDataDir(cfg, to)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jsonl", ".ndjson":
			data = rewriteJSONLPrefix(data, from+"/images/", to+"/images/")
		case ".csv":
			data = rewriteCSVPrefix(data, from+"/images/", to+"/images/")
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// rewriteJSONLPrefix replaces oldPrefix at the start of string values. Lines
// that aren't JSON objects are kept as they are.
func rewriteJSONLPrefix(data []byte, oldPrefix, newPrefix string) []byte {
	var out bytes.Buffer
	for _, line := range bytes.Split(data, []byte("\n")) {
		var obj map[string]any
		if len(bytes.TrimSpace(line)) == 0 || json.Unmarshal(line, &obj) != nil {
			out.Write(line)
		} else {
			for k, v := range obj {
				if s, ok := v.(string); ok && strings.HasPrefix(s, oldPrefix) {
					obj[k] = newPrefix + strings.TrimPrefix(s, oldPrefix)
				}
			}
			encoded, _ := json.Marshal(obj)
			out.Write(encoded)
		}
		out.WriteByte('\n')
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

// rewriteCSVPrefix replaces oldPrefix at the start of CSV cells. It works on
// the text (start of line, after a separator or an opening quote) so the
// file's separator and quoting stay exactly as they were.
func rewriteCSVPrefix(data []byte, oldPrefix, newPrefix string) []byte {
	re := regexp.MustCompile(`(^|\n|,|;|")` + regexp.QuoteMeta(oldPrefix))
	return re.ReplaceAll(data, []byte("${1}"+strings.ReplaceAll(newPrefix, "$", "$$")))
}
