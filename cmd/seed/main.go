package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Dankular/GameService/internal/compiler"
	"github.com/Dankular/GameService/internal/definitions"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	file := flag.String("file", "definitions/examples/arena.yaml", "definition YAML/JSON file")
	environment := flag.String("environment", envOr("SEED_ENVIRONMENT", "dev"), "environment to activate")
	actor := flag.String("actor", envOr("SEED_ACTOR", "seed"), "audit actor ID")
	databaseURL := flag.String("database-url", envOr("DATABASE_URL", ""), "PostgreSQL connection URL")
	flag.Parse()
	if *databaseURL == "" {
		fatal("DATABASE_URL is required")
	}

	source, err := os.ReadFile(*file)
	if err != nil {
		fatal(err.Error())
	}
	report, err := compiler.Compile(bytes.NewReader(source))
	if err != nil {
		fatal(err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		fatal(err.Error())
	}
	defer pool.Close()
	store := definitions.Store{Pool: pool}
	if err := store.Publish(ctx, report, string(source), *actor, "seed definition", true); err != nil {
		fatal(fmt.Sprintf("publish definition: %v", err))
	}
	if err := store.Activate(ctx, report.Definition.Metadata.GameID, *environment, report.Definition.Metadata.Revision, *actor, "seed definition", true); err != nil {
		fatal(fmt.Sprintf("activate definition: %v", err))
	}
	fmt.Printf("seeded game=%s environment=%s revision=%d digest=%s\n", report.Definition.Metadata.GameID, *environment, report.Definition.Metadata.Revision, report.Digest)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
