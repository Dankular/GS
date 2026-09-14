package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/Dankular/GameService/internal/commandstore"
	"github.com/Dankular/GameService/internal/reconciliation"
)

func main() {
	ctx := context.Background()
	repo, err := commandstore.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("reconciliation database initialization failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	interval := time.Duration(envInt("RECONCILIATION_INTERVAL_SECONDS", 300)) * time.Second
	once := os.Getenv("RECONCILIATION_ONCE") == "true"
	for {
		allocationTimeout := time.Duration(envInt("MATCH_ALLOCATION_TIMEOUT_SECONDS", 120)) * time.Second
		if recovered, recoveryErr := reconciliation.RecoverStaleAllocations(ctx, repo.Pool(), time.Now().UTC().Add(-allocationTimeout)); recoveryErr != nil {
			slog.Error("stale allocation recovery failed", "error", recoveryErr)
		} else if recovered > 0 {
			slog.Warn("stale allocations failed", "count", recovered)
		}
		runningTimeout := time.Duration(envInt("MATCH_RUNNING_TIMEOUT_SECONDS", 180)) * time.Second
		if abandoned, recoveryErr := reconciliation.RecoverStaleRunningMatches(ctx, repo.Pool(), time.Now().UTC().Add(-runningTimeout)); recoveryErr != nil {
			slog.Error("stale running-match recovery failed", "error", recoveryErr)
		} else if abandoned > 0 {
			slog.Warn("stale running matches abandoned", "count", abandoned)
		}
		findings, scanErr := reconciliation.Scan(ctx, repo.Pool())
		if scanErr != nil {
			slog.Error("reconciliation scan failed", "error", scanErr)
		} else if len(findings) > 0 {
			slog.Error("economic projection mismatch detected", "findingCount", len(findings), "findings", findings)
		} else {
			slog.Info("economic reconciliation passed")
		}
		if once {
			if scanErr != nil || len(findings) > 0 {
				os.Exit(1)
			}
			return
		}
		time.Sleep(interval)
	}
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
