package markdown

import (
	"fmt"
	"io"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

const (
	fenceRune  = '`'
	minFence   = 3
	dotSpacing = " · "
)

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

type Input struct {
	ToolVersion string
	Dialect     ir.Dialect
	DBVersion   ir.Version
	Migrations  []*ir.Migration
	Result      analyze.Result
	FailOn      string
}

type counts struct {
	errors   int
	warnings int
	notices  int
	ignored  int
	failing  int
}

func Write(w io.Writer, in Input) error {
	if _, err := io.WriteString(w, Render(in)); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}

	return nil
}

func Render(in Input) string {
	var out strings.Builder

	c := count(in.Result.Findings, ir.Severity(in.FailOn))

	out.WriteString("## migrail\n\n")
	out.WriteString(headline(in, c))

	if c.errors+c.warnings+c.notices > 0 {
		out.WriteString("\n")
		writeTable(&out, in.Result.Findings)

		for _, finding := range in.Result.Findings {
			if finding.Suppressed == nil {
				out.WriteString("\n")
				writeFinding(&out, finding)
			}
		}
	}

	for _, e := range in.Result.RuleErrors {
		fmt.Fprintf(&out, "\n> [!CAUTION]\n> Rule %s failed on %s, so findings may be incomplete: %s\n", e.RuleID, code(e.MigrationID), e.Message)
	}

	out.WriteString("\n<sub>" + footer(in, c) + "</sub>\n")

	return out.String()
}

func count(findings []ir.Finding, failOn ir.Severity) counts {
	c := counts{}

	for _, finding := range findings {
		if finding.Suppressed != nil {
			c.ignored++

			continue
		}

		if finding.Severity.AtLeast(failOn) {
			c.failing++
		}

		switch finding.Severity {
		case ir.SeverityError:
			c.errors++
		case ir.SeverityWarning:
			c.warnings++
		case ir.SeverityNotice:
			c.notices++
		case ir.SeverityOff:
		}
	}

	return c
}

func headline(in Input, c counts) string {
	migrations := quantity(len(in.Migrations), "migration", "migrations")

	if c.errors+c.warnings+c.notices == 0 {
		return fmt.Sprintf("No issues found in %s.\n", migrations)
	}

	line := fmt.Sprintf("**%s, %s, %s** in %s.",
		quantity(c.errors, "error", "errors"),
		quantity(c.warnings, "warning", "warnings"),
		quantity(c.notices, "notice", "notices"),
		migrations,
	)

	if c.failing > 0 {
		line += fmt.Sprintf(" %s at or above %s %s this check.",
			quantity(c.failing, "finding", "findings"), code(in.FailOn), components.Plural(c.failing, "fails", "fail"))
	}

	return line + "\n"
}

func writeTable(out *strings.Builder, findings []ir.Finding) {
	out.WriteString("| Severity | Rule | Location | Finding |\n")
	out.WriteString("| --- | --- | --- | --- |\n")

	for _, finding := range findings {
		if finding.Suppressed != nil {
			continue
		}

		fmt.Fprintf(out, "| %s | %s | %s | %s |\n",
			finding.Severity,
			finding.RuleID,
			strings.ReplaceAll(code(location(finding)), "|", `\|`),
			cell(finding.Title),
		)
	}
}

func writeFinding(out *strings.Builder, finding ir.Finding) {
	fmt.Fprintf(out, "<details>\n<summary><b>%s</b> %s%s%s</summary>\n\n",
		finding.Severity,
		finding.RuleID,
		dotSpacing,
		htmlEscaper.Replace(finding.Title),
	)

	fmt.Fprintf(out, "%s\n\n", code(location(finding)))

	if finding.Statement != nil && strings.TrimSpace(finding.Statement.SQL) != "" {
		out.WriteString(codeBlock("sql", strings.TrimSpace(finding.Statement.SQL)))
		out.WriteString("\n")
	}

	if finding.Lock != nil {
		fmt.Fprintf(out, "**Lock:** %s\n\n", lockText(finding.Lock))
	}

	if finding.Why != "" {
		fmt.Fprintf(out, "**Why:** %s\n\n", finding.Why)
	}

	if finding.Fix != nil {
		writeFix(out, finding.Fix)
	}

	fmt.Fprintf(out, "Run `migrail explain %s` for details.\n\n</details>\n", finding.RuleID)
}

func writeFix(out *strings.Builder, fix *ir.Fix) {
	if fix.Summary != "" {
		fmt.Fprintf(out, "**Fix:** %s\n\n", fix.Summary)
	}

	numbered := len(fix.Steps) > 1

	for i, step := range fix.Steps {
		if step.Title != "" {
			if numbered {
				fmt.Fprintf(out, "%d. %s\n\n", i+1, step.Title)
			} else {
				fmt.Fprintf(out, "%s\n\n", step.Title)
			}
		}

		if step.Code != "" {
			out.WriteString(codeBlock(step.Lang, step.Code))
			out.WriteString("\n")
		}
	}
}

func lockText(lock *ir.LockImpact) string {
	text := fmt.Sprintf("%s on %s%s%s", lock.Mode, strings.Join(lock.Tables, ", "), dotSpacing, lock.BlockedText())

	if lock.Rewrite {
		text += dotSpacing + "rewrites the table"
	}

	return text
}

func location(finding ir.Finding) string {
	return fmt.Sprintf("%s:%d", finding.Location.Path, finding.Location.Span.Start.Line)
}

func footer(in Input, c counts) string {
	parts := []string{
		"migrail " + in.ToolVersion,
		fmt.Sprintf("%s %s", dialectName(in.Dialect), in.DBVersion),
		quantity(in.Result.Statements, "statement", "statements"),
	}

	if c.ignored > 0 {
		parts = append(parts, fmt.Sprintf("%d ignored", c.ignored))
	}

	return strings.Join(parts, dotSpacing)
}

func dialectName(dialect ir.Dialect) string {
	if dialect == ir.DialectPostgres {
		return "PostgreSQL"
	}

	return string(dialect)
}

func codeBlock(lang, body string) string {
	marker := strings.Repeat(string(fenceRune), max(minFence, longestRun(body, fenceRune)+1))

	return marker + lang + "\n" + body + "\n" + marker + "\n"
}

func longestRun(text string, r rune) int {
	longest, current := 0, 0

	for _, c := range text {
		if c != r {
			current = 0

			continue
		}

		current++
		longest = max(longest, current)
	}

	return longest
}

func code(text string) string {
	marker := strings.Repeat(string(fenceRune), longestRun(text, fenceRune)+1)
	if strings.HasPrefix(text, string(fenceRune)) || strings.HasSuffix(text, string(fenceRune)) {
		return marker + " " + text + " " + marker
	}

	return marker + text + marker
}

func cell(text string) string {
	return strings.ReplaceAll(htmlEscaper.Replace(text), "|", `\|`)
}

func quantity(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, components.Plural(n, one, many))
}
