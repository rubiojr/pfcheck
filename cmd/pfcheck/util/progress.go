package util

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// IsStderrTTY reports whether standard error is an interactive terminal.
// Progress animations are only drawn when this is true so piped/redirected
// output stays clean.
func IsStderrTTY() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// IsStdinPiped reports whether standard input is connected to a pipe or a
// redirected file rather than an interactive terminal, i.e. there is data to
// read without blocking on a human.
func IsStdinPiped() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

// HumanBytes formats a byte count using binary (IEC) units.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Spinner shows an animated status message on stderr for long, indeterminate
// operations (e.g. loading the model). On a non-TTY it prints the message once.
type Spinner struct {
	mu   sync.Mutex
	msg  string
	tty  bool
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// NewSpinner creates a Spinner with the given status message.
func NewSpinner(msg string) *Spinner {
	return &Spinner{
		msg:  msg,
		tty:  IsStderrTTY(),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// SetMessage updates the status text shown by the spinner. Safe to call from
// any goroutine while the spinner is running.
func (s *Spinner) SetMessage(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

func (s *Spinner) message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.msg
}

// Start begins the animation (or prints the message once on a non-TTY).
func (s *Spinner) Start() {
	if !s.tty {
		fmt.Fprintln(os.Stderr, s.message())
		close(s.done)
		return
	}
	go func() {
		frames := []rune{'|', '/', '-', '\\'}
		t := time.NewTicker(120 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-s.stop:
				fmt.Fprint(os.Stderr, "\r\033[K")
				close(s.done)
				return
			case <-t.C:
				fmt.Fprintf(os.Stderr, "\r\033[K%c %s", frames[i%len(frames)], s.message())
				i++
			}
		}
	}()
}

// Stop ends the animation and clears the line. Safe to call more than once.
func (s *Spinner) Stop() {
	if s.tty {
		s.once.Do(func() { close(s.stop) })
	}
	<-s.done
}

// DownloadBar renders download progress on stderr. The header is printed lazily
// on the first update, so nothing is shown when a download turns out to be
// unnecessary (e.g. the file already exists).
type DownloadBar struct {
	header  string
	tty     bool
	started bool
}

// NewDownloadBar creates a DownloadBar. header may be empty.
func NewDownloadBar(header string) *DownloadBar {
	return &DownloadBar{header: header, tty: IsStderrTTY()}
}

// Update is a pfcheck.ProgressFunc that draws the current progress.
func (b *DownloadBar) Update(downloaded, total int64) {
	if !b.started {
		b.started = true
		if b.header != "" {
			fmt.Fprintln(os.Stderr, b.header)
		}
	}
	if !b.tty {
		return
	}
	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "\r\033[K  %6.2f%%  %s / %s",
			pct, HumanBytes(downloaded), HumanBytes(total))
	} else {
		fmt.Fprintf(os.Stderr, "\r\033[K  %s downloaded", HumanBytes(downloaded))
	}
}

// Finish terminates the progress line. It is a no-op if no update occurred.
func (b *DownloadBar) Finish() {
	if b.started && b.tty {
		fmt.Fprintln(os.Stderr)
	}
}
