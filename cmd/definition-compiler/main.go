package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Dankular/GameService/internal/compiler"
)

func main() {
	path := flag.String("file", "", "definition YAML/JSON file")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "--file is required")
		os.Exit(2)
	}
	f, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	report, err := compiler.Compile(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := struct {
		Digest    string          `json:"digest"`
		Canonical json.RawMessage `json:"canonical"`
	}{report.Digest, report.Canonical}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
