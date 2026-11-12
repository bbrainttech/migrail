package cli

import (
	"context"
	"errors"
	"fmt"

	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/livedb"
	"github.com/bbrainttech/migrail/internal/ui/progress"
)

const databaseURLEnv = "MIGRAIL_DATABASE_URL"

func databaseURL(opts checkOptions) string {
	if opts.databaseURL != "" {
		return opts.databaseURL
	}

	if url := opts.ui.env[databaseURLEnv]; url != "" {
		return url
	}

	if opts.cfg.DB.URLEnv != "" {
		return opts.ui.env[opts.cfg.DB.URLEnv]
	}

	return ""
}

func openLiveSession(ctx context.Context, opts checkOptions, phases *progress.Progress, stderr display, migrations int) *livedb.Session {
	url := databaseURL(opts)
	if url == "" || migrations == 0 {
		return nil
	}

	finish := phases.Start("Querying database", "read-only")

	session, err := livedb.Open(ctx, url)
	if err != nil {
		finish.Abort()
		writeNotice(stderr, fmt.Sprintf("couldn't use the database, so this run has no size estimates: %s.", livedb.Redact(err.Error(), url)))

		return nil
	}

	finish.Done("Queried database", fmt.Sprintf("PostgreSQL %s %s read-only", session.Version, stderr.theme.Symbols.Dot))

	return session
}

func liveVersion(session *livedb.Session, configured ir.Version, opts checkOptions, stderr display) ir.Version {
	if opts.dbVersion != "" {
		return configured
	}

	lowest, highest := pg.New().SupportedVersions()
	if session.Version.Major < lowest.Major || session.Version.Major > highest.Major {
		writeNotice(stderr, fmt.Sprintf(
			"the database runs PostgreSQL %s, outside the supported %s to %s, so migrail assumes PostgreSQL %s.",
			session.Version, lowest, highest, configured,
		))

		return configured
	}

	return session.Version
}

func annotateImpact(ctx context.Context, session *livedb.Session, findings []ir.Finding, opts checkOptions, stderr display) error {
	if session == nil {
		return nil
	}

	tables, err := session.Tables(ctx, livedb.LockedTables(findings))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}

		writeNotice(stderr, fmt.Sprintf("couldn't read table sizes, so this run has no size estimates: %s.", livedb.Redact(err.Error(), databaseURL(opts))))

		return nil
	}

	livedb.Annotate(findings, tables, livedb.Thresholds{
		SmallRows: opts.cfg.Thresholds.SmallTableRows,
		LargeRows: opts.cfg.Thresholds.LargeTableRows,
	})

	return nil
}
