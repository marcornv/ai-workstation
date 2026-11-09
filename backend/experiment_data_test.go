package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testWorkspace creates experiments/ and data/ in a temp folder.
func testWorkspace(t *testing.T, experiments ...string) Config {
	t.Helper()
	root := t.TempDir()
	cfg := Config{Paths: Paths{Experiments: filepath.Join(root, "experiments"), Datasets: filepath.Join(root, "data")}}
	os.MkdirAll(cfg.Paths.Experiments, 0o755)
	os.MkdirAll(cfg.Paths.Datasets, 0o755)
	for _, id := range experiments {
		os.WriteFile(experimentPath(cfg, id), []byte("name: x\n"), 0o644)
	}
	return cfg
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDatasetPath(t *testing.T) {
	cfg := testWorkspace(t)
	ok := map[string]string{
		"a.csv":         filepath.Join(cfg.Paths.Datasets, "a.csv"),
		"exp-1/a.csv":   filepath.Join(cfg.Paths.Datasets, "exp-1", "a.csv"),
		"exp-1/b.jsonl": filepath.Join(cfg.Paths.Datasets, "exp-1", "b.jsonl"),
	}
	for in, want := range ok {
		if got, err := datasetPath(cfg, in); err != nil || got != want {
			t.Errorf("datasetPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "..", "../a.csv", "exp/../a.csv", "exp/sub/a.csv", "images/a.csv", "Exp/a.csv", "/a.csv", "exp/", "exp\\..\\a.csv"} {
		if _, err := datasetPath(cfg, bad); err == nil {
			t.Errorf("datasetPath(%q) should fail", bad)
		}
	}
}

func TestNewExperimentIDAvoidsTakenNames(t *testing.T) {
	cfg := testWorkspace(t, "taken")
	os.MkdirAll(experimentDataDir(cfg, "leftover"), 0o755) // data folder without experiment
	cases := map[string]string{"new": "new", "taken": "taken-2", "leftover": "leftover-2", "images": "images-2", "": "experimento"}
	for slug, want := range cases {
		if got := newExperimentID(cfg, slug); got != want {
			t.Errorf("newExperimentID(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestListDatasetFiles(t *testing.T) {
	cfg := testWorkspace(t, "exp-a", "exp-b")
	d := cfg.Paths.Datasets
	writeTestFile(t, filepath.Join(d, "shared.csv"), "x\n1\n")
	writeTestFile(t, filepath.Join(d, "exp-a", "mine.jsonl"), "{}\n")
	writeTestFile(t, filepath.Join(d, "exp-b", "other.csv"), "x\n1\n")
	writeTestFile(t, filepath.Join(d, "gone", "orphan.csv"), "x\n1\n")  // no experiment
	writeTestFile(t, filepath.Join(d, "images", "old", "1.png"), "png") // old shared images

	if got, want := listDatasetFiles(cfg, "exp-a"), []string{"exp-a/mine.jsonl", "shared.csv"}; !reflect.DeepEqual(got, want) {
		t.Errorf("for exp-a got %v, want %v", got, want)
	}
	if got, want := listDatasetFiles(cfg, ""), []string{"shared.csv", "exp-a/mine.jsonl", "exp-b/other.csv"}; !reflect.DeepEqual(got, want) {
		t.Errorf("for all got %v, want %v", got, want)
	}
}

func TestDeleteExperimentDataset(t *testing.T) {
	cfg := testWorkspace(t, "exp")
	d := cfg.Paths.Datasets
	writeTestFile(t, filepath.Join(d, "exp", "fotos.jsonl"), "{}\n")
	writeTestFile(t, filepath.Join(d, "exp", "images", "fotos", "1.png"), "png")
	writeTestFile(t, filepath.Join(d, "exp", "images", "outras", "1.png"), "png")
	writeTestFile(t, filepath.Join(d, "shared.csv"), "x\n1\n")

	if err := deleteExperimentDataset(cfg, "shared.csv"); err == nil {
		t.Fatal("shared files must not be deletable")
	}
	if err := deleteExperimentDataset(cfg, "exp/fotos.jsonl"); err != nil {
		t.Fatal(err)
	}
	for path, wantExists := range map[string]bool{
		filepath.Join(d, "exp", "fotos.jsonl"):      false,
		filepath.Join(d, "exp", "images", "fotos"):  false,
		filepath.Join(d, "exp", "images", "outras"): true,
		filepath.Join(d, "shared.csv"):              true,
	} {
		if _, err := os.Stat(path); (err == nil) != wantExists {
			t.Errorf("%s: exists=%v, want %v", path, err == nil, wantExists)
		}
	}
}

func TestDeleteExperimentDataKeepsSharedImages(t *testing.T) {
	cfg := testWorkspace(t)
	writeTestFile(t, filepath.Join(cfg.Paths.Datasets, "images", "old", "1.png"), "png")
	writeTestFile(t, filepath.Join(cfg.Paths.Datasets, "exp", "a.csv"), "x\n1\n")
	deleteExperimentData(cfg, "images") // an old experiment with this id must not wipe shared images
	deleteExperimentData(cfg, "exp")
	if _, err := os.Stat(filepath.Join(cfg.Paths.Datasets, "images", "old", "1.png")); err != nil {
		t.Fatal("shared images were deleted")
	}
	if _, err := os.Stat(filepath.Join(cfg.Paths.Datasets, "exp")); err == nil {
		t.Fatal("experiment folder was not deleted")
	}
}

func TestCopyExperimentDataRewritesImagePaths(t *testing.T) {
	cfg := testWorkspace(t)
	d := cfg.Paths.Datasets
	writeTestFile(t, filepath.Join(d, "src", "fotos.jsonl"), `{"imagem":"src/images/fotos/1.png","rotulo":"src/images is just text"}`+"\n")
	writeTestFile(t, filepath.Join(d, "src", "lista.csv"), "imagem;nota\nsrc/images/fotos/1.png;ok\n\"src/images/fotos/2.png\";images/shared.png\n")
	writeTestFile(t, filepath.Join(d, "src", "images", "fotos", "1.png"), "png")

	if err := copyExperimentData(cfg, "src", "dst"); err != nil {
		t.Fatal(err)
	}
	jsonl, _ := os.ReadFile(filepath.Join(d, "dst", "fotos.jsonl"))
	if !strings.Contains(string(jsonl), `"imagem":"dst/images/fotos/1.png"`) || !strings.Contains(string(jsonl), `"rotulo":"src/images is just text"`) {
		t.Errorf("jsonl not rewritten as expected: %s", jsonl)
	}
	csvData, _ := os.ReadFile(filepath.Join(d, "dst", "lista.csv"))
	if want := "imagem;nota\ndst/images/fotos/1.png;ok\n\"dst/images/fotos/2.png\";images/shared.png\n"; string(csvData) != want {
		t.Errorf("csv = %q, want %q", csvData, want)
	}
	if _, err := os.Stat(filepath.Join(d, "dst", "images", "fotos", "1.png")); err != nil {
		t.Error("image was not copied")
	}
	original, _ := os.ReadFile(filepath.Join(d, "src", "fotos.jsonl"))
	if !strings.Contains(string(original), "src/images/fotos/1.png") {
		t.Error("the original was changed")
	}
	if err := copyExperimentData(cfg, "missing", "dst2"); err != nil {
		t.Errorf("copying an experiment without data should do nothing, got %v", err)
	}
}
