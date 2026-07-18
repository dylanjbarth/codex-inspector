package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dylanjbarth/codex-inspector/internal/phase0"
)

func main() {
	names := []string{"root.jsonl", "descendant.jsonl", "unsupported.jsonl", "truncated.jsonl"}
	inputs := make([]phase0.CorpusInput, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join("fixtures", "synthetic", name))
		if err != nil {
			fail(err)
		}
		inputs = append(inputs, phase0.CorpusInput{Name: name, Data: data})
	}
	corpus, err := phase0.NormalizeCorpus(inputs)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(corpus); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
