package cli

import (
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const commandColumn = 12

var flagUsageLine = regexp.MustCompile(`^(\s*)(-\S.*?)(\s{2,})(.*)$`)

func installHelp(root *cobra.Command, ui *uiFlags) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		d := newDisplay(cmd.OutOrStdout(), ui.settings(ciEnabled(false)))
		banner := showBanner(d) && !cmd.HasParent()
		_, _ = io.WriteString(d.out, renderHelp(cmd, d.theme, d.caps.ContentWidth(), d.caps.Width, banner))
	})
}

func renderHelp(cmd *cobra.Command, t theme.Theme, width, terminalWidth int, banner bool) string {
	sections := []string{}

	if banner {
		sections = append(sections, strings.TrimSuffix(components.Banner(t, terminalWidth), "\n"))
	}

	description := cmd.Long
	if description == "" {
		description = cmd.Short
	}

	sections = append(sections, indentLines(t.Fg, components.Wrap(description, width-2), "  "))
	sections = append(sections, helpSection(t, "Usage", "    "+t.Fg.Render(usageLine(cmd))))

	if commands := commandLines(cmd, t); len(commands) > 0 {
		sections = append(sections, helpSection(t, "Commands", strings.Join(commands, "\n")))
	}

	if cmd.HasAvailableLocalFlags() {
		sections = append(sections, helpSection(t, "Flags", flagLines(t, cmd.LocalFlags(), width)))
	}

	if cmd.HasAvailableInheritedFlags() {
		sections = append(sections, helpSection(t, "Global flags", flagLines(t, cmd.InheritedFlags(), width)))
	}

	if cmd.HasAvailableSubCommands() {
		sections = append(sections, "  "+t.Muted.Render("Run ")+t.Fg.Render(cmd.CommandPath()+" <command> --help")+
			t.Muted.Render(" for details on a command."))
	}

	return strings.Join(sections, "\n\n") + "\n"
}

func usageLine(cmd *cobra.Command) string {
	if cmd.HasAvailableSubCommands() {
		return cmd.CommandPath() + " <command> [flags]"
	}

	return cmd.UseLine()
}

func helpSection(t theme.Theme, title, body string) string {
	return "  " + t.Strong(t.Accent).Render(title) + "\n" + body
}

func commandLines(cmd *cobra.Command, t theme.Theme) []string {
	lines := []string{}

	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}

		lines = append(lines, "    "+t.Strong(t.Fg).Render(components.PadRight(sub.Name(), commandColumn))+t.Fg.Render(sub.Short))
	}

	return lines
}

func flagLines(t theme.Theme, flags *pflag.FlagSet, width int) string {
	usages := strings.TrimRight(flags.FlagUsagesWrapped(width-4), "\n")
	lines := strings.Split(usages, "\n")

	for i, line := range lines {
		match := flagUsageLine.FindStringSubmatch(line)
		if match == nil {
			lines[i] = "  " + t.Fg.Render(line)

			continue
		}

		lines[i] = "  " + match[1] + t.Strong(t.Fg).Render(match[2]) + match[3] + t.Fg.Render(match[4])
	}

	return strings.Join(lines, "\n")
}

func indentLines(style interface{ Render(...string) string }, lines []string, prefix string) string {
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			rendered = append(rendered, "")

			continue
		}

		rendered = append(rendered, prefix+style.Render(line))
	}

	return strings.Join(rendered, "\n")
}
