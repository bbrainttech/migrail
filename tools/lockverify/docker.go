package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	startupTimeout = 90 * time.Second
	password       = "lockverify"
)

type postgresContainer struct {
	id  string
	url string
}

func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // docker is a fixed binary and the arguments come from this tool

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func startPostgres(ctx context.Context, version string) (*postgresContainer, error) {
	id, err := docker(ctx, "run", "-d", "--rm",
		"-e", "POSTGRES_PASSWORD="+password,
		"-p", "127.0.0.1::5432",
		"--tmpfs", "/var/lib/postgresql/data",
		"postgres:"+version+"-alpine",
		"-c", "fsync=off")
	if err != nil {
		return nil, err
	}

	container := &postgresContainer{id: id}

	address, err := docker(ctx, "port", id, "5432/tcp")
	if err != nil {
		container.stop()

		return nil, err
	}

	container.url = fmt.Sprintf("postgres://postgres:%s@%s/postgres?sslmode=disable", password, strings.Split(address, "\n")[0])

	if err := waitReady(ctx, container.url); err != nil {
		container.stop()

		return nil, err
	}

	return container, nil
}

func waitReady(ctx context.Context, url string) error {
	deadline := time.Now().Add(startupTimeout)

	for time.Now().Before(deadline) {
		conn, err := pgx.Connect(ctx, url)
		if err == nil {
			_, err = conn.Exec(ctx, "SELECT 1")
			_ = conn.Close(ctx)

			if err == nil {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	return fmt.Errorf("postgres didn't accept connections within %s", startupTimeout)
}

func (c *postgresContainer) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = docker(ctx, "rm", "-f", c.id)
}
