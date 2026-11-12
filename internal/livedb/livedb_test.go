package livedb

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bbrainttech/migrail/internal/ir"
)

const testDatabaseURLEnv = "MIGRAIL_TEST_DATABASE_URL"

func TestRedactRemovesSecrets(t *testing.T) {
	t.Parallel()

	const url = "postgres://app:s3cret-Value@db.internal:5432/app?sslmode=require" //nolint:gosec // fake credentials for the redaction test

	tests := map[string]string{ //nolint:gosec // fake credentials for the redaction test
		"whole url":      "failed to connect to " + url,
		"password alone": "authentication failed using s3cret-Value",
		"other url":      "dial postgres://admin:hunter2@10.0.0.1/app failed",
		"key value":      "cannot parse host=db password=hunter2 user=app",
		"sslpassword":    "cannot parse sslpassword=hunter2",
	}

	for name, message := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			redacted := Redact(message, url)

			for _, secret := range []string{"s3cret-Value", "hunter2"} {
				if strings.Contains(redacted, secret) {
					t.Errorf("Redact() = %q still contains %q", redacted, secret)
				}
			}
		})
	}
}

func TestOpenWithInvalidURLKeepsItOutOfTheError(t *testing.T) {
	t.Parallel()

	_, err := Open(context.Background(), "postgres://app:s3cret@bad host/app")
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("Open() error = %v, want ErrInvalidURL", err)
	}

	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %v", err)
	}
}

func TestParseVersionNumber(t *testing.T) {
	t.Parallel()

	tests := map[string]ir.Version{
		"160004": {Major: 16, Minor: 4},
		"120022": {Major: 12, Minor: 22},
		"190000": {Major: 19},
	}

	for text, want := range tests {
		got, err := parseVersionNumber(text)
		if err != nil || got != want {
			t.Errorf("parseVersionNumber(%q) = %v, %v; want %v", text, got, err, want)
		}
	}

	if _, err := parseVersionNumber("sixteen"); err == nil {
		t.Error("parseVersionNumber accepted text")
	}
}

func TestQualifiedReference(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"orders":         `"orders"`,
		"billing.orders": `"billing"."orders"`,
		`we"ird`:         `"we""ird"`,
	}

	for name, want := range tests {
		if got := qualifiedReference(name); got != want {
			t.Errorf("qualifiedReference(%q) = %s, want %s", name, got, want)
		}
	}
}

func openTestSession(t *testing.T) *Session {
	t.Helper()

	url := os.Getenv(testDatabaseURLEnv)
	if url == "" {
		t.Skip(testDatabaseURLEnv + " isn't set")
	}

	session, err := Open(t.Context(), url)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	t.Cleanup(func() { session.Close(context.Background()) })

	return session
}

func TestSessionReadsCatalogStatistics(t *testing.T) {
	t.Parallel()

	session := openTestSession(t)

	if session.Version.Major < 12 {
		t.Errorf("Version = %s, want at least 12", session.Version)
	}

	tables, err := session.Tables(t.Context(), []string{"pg_catalog.pg_class", "no_such_table"})
	if err != nil {
		t.Fatalf("Tables() error = %v", err)
	}

	catalog, ok := tables["pg_catalog.pg_class"]
	if !ok || catalog.Rows == 0 || catalog.TableBytes == 0 || catalog.IndexBytes == 0 {
		t.Errorf("pg_class stats = %+v, found %v", catalog, ok)
	}

	if _, ok := tables["no_such_table"]; ok {
		t.Error("a missing table has stats")
	}

	if _, err := session.LongTransactions(t.Context(), time.Hour); err != nil {
		t.Errorf("LongTransactions() error = %v", err)
	}
}

func TestSessionRejectsWrites(t *testing.T) {
	t.Parallel()

	session := openTestSession(t)

	_, err := session.tx.Exec(t.Context(), "CREATE TABLE migrail_must_not_exist (id int)")

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25006" {
		t.Fatalf("write error = %v, want read_only_sql_transaction (25006)", err)
	}
}
