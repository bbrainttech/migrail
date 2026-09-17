package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
)

type versionResult struct {
	Version    string            `json:"version"`
	Matrix     []matrixResult    `json:"matrix"`
	Operations []operationResult `json:"operations"`
}

func main() {
	versions := flag.String("versions", "12,13,14,15,16,17,18", "comma-separated PostgreSQL major versions")
	output := flag.String("output", "lockmodel.verified.json", "where to write the verification report")

	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, strings.Split(*versions, ","), *output)

	stop()
	os.Exit(code)
}

func run(ctx context.Context, versions []string, output string) int {
	results := []versionResult{}
	failures := 0

	for _, version := range versions {
		version = strings.TrimSpace(version)
		fmt.Fprintf(os.Stderr, "PostgreSQL %s: starting container\n", version)

		result, err := verifyVersion(ctx, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PostgreSQL %s: %v\n", version, err)

			return 2
		}

		failures += report(result)
		results = append(results, result)
	}

	if err := writeReport(output, results); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 2
	}

	if failures > 0 {
		fmt.Fprintf(os.Stderr, "%d lock model claims don't match PostgreSQL\n", failures)

		return 1
	}

	fmt.Fprintln(os.Stderr, "All lock model claims match PostgreSQL")

	return 0
}

func verifyVersion(ctx context.Context, version string) (versionResult, error) {
	container, err := startPostgres(ctx, version)
	if err != nil {
		return versionResult{}, err
	}
	defer container.stop()

	result := versionResult{Version: version}

	result.Matrix, err = verifyMatrix(ctx, container.url)
	if err != nil {
		return versionResult{}, fmt.Errorf("verify lock conflicts: %w", err)
	}

	result.Operations, err = verifyOperations(ctx, container.url)
	if err != nil {
		return versionResult{}, fmt.Errorf("verify operations: %w", err)
	}

	return result, nil
}

func report(result versionResult) int {
	failures := 0

	for _, entry := range result.Matrix {
		if !entry.OK {
			failures++

			fmt.Fprintf(os.Stderr, "  x PG %s %s blocks %v, lock model says %v\n", result.Version, entry.Mode, entry.Observed, entry.Expected)
		}
	}

	for _, entry := range result.Operations {
		if !entry.OK {
			failures++

			fmt.Fprintf(os.Stderr, "  x PG %s %s: observed %s rewrite=%v, lock model says %s rewrite=%v (%s)\n",
				result.Version, entry.Operation, entry.ObservedModes, entry.ObservedRewrite, entry.ExpectedMode, entry.ExpectedRewrite, entry.Note)
		}
	}

	fmt.Fprintf(os.Stderr, "PostgreSQL %s: %d lock modes and %d operations checked, %d mismatches\n",
		result.Version, len(result.Matrix), len(result.Operations), failures)

	return failures
}

func writeReport(path string, results []versionResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return errors.Join(fmt.Errorf("write report %s", path), err)
	}

	return nil
}
