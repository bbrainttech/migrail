package progress

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const (
	appearAfter   = 150 * time.Millisecond
	frameInterval = 80 * time.Millisecond
	nameWidth     = 17
	detailWidth   = 31
	timeWidth     = 6
	clearLine     = "\r\x1b[2K"
)

type Options struct {
	Animate bool
	Verbose bool
}

type Progress struct {
	styled io.Writer
	raw    io.Writer
	theme  theme.Theme
	opts   Options
	mu     sync.Mutex
}

type Phase struct {
	progress *Progress
	started  time.Time
	stop     chan struct{}
	stopped  sync.WaitGroup
	once     sync.Once
}

func New(styled, raw io.Writer, t theme.Theme, opts Options) *Progress {
	return &Progress{styled: styled, raw: raw, theme: t, opts: opts}
}

func (p *Progress) Start(name, detail string) *Phase {
	phase := &Phase{progress: p, started: time.Now(), stop: make(chan struct{})}

	if !p.opts.Animate {
		return phase
	}

	phase.stopped.Add(1)

	go phase.animate(name, detail)

	return phase
}

func (ph *Phase) animate(name, detail string) {
	defer ph.stopped.Done()

	timer := time.NewTimer(appearAfter)
	defer timer.Stop()

	select {
	case <-ph.stop:
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for frame := 0; ; frame++ {
		ph.progress.write(ph.progress.raw, clearLine)
		ph.progress.write(ph.progress.styled, ActiveLine(ph.progress.theme, frame, name, detail))

		select {
		case <-ph.stop:
			ph.progress.write(ph.progress.raw, clearLine)

			return
		case <-ticker.C:
		}
	}
}

func (ph *Phase) Done(name, detail string) {
	ph.finish()

	if ph.progress.opts.Verbose {
		ph.progress.write(ph.progress.styled, DoneLine(ph.progress.theme, name, detail, time.Since(ph.started))+"\n")
	}
}

func (ph *Phase) Abort() {
	ph.finish()
}

func (ph *Phase) finish() {
	ph.once.Do(func() {
		close(ph.stop)
		ph.stopped.Wait()
	})
}

func (p *Progress) write(w io.Writer, text string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = io.WriteString(w, text)
}

func ActiveLine(t theme.Theme, frame int, name, detail string) string {
	spinner := t.Symbols.Spinner[frame%len(t.Symbols.Spinner)]

	return t.Accent.Render(spinner) + " " + t.Fg.Render(name) + "  " + t.Muted.Render(detail)
}

func DoneLine(t theme.Theme, name, detail string, elapsed time.Duration) string {
	return t.Success.Render(t.Symbols.Success) + " " +
		t.Fg.Render(components.PadRight(name, nameWidth)) +
		t.Muted.Render(components.PadRight(detail, detailWidth)) +
		t.Muted.Render(components.PadLeft(formatElapsed(elapsed), timeWidth))
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	return fmt.Sprintf("%.1fs", d.Seconds())
}
