package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzippit/arr-cli/internal/config"
)

// defaultWatchInterval is how often queue --watch refreshes without
// --interval.
const defaultWatchInterval = 2 * time.Second

// Terminal control sequences for queue --watch: switch to the alternate
// screen and hide the cursor while watching, and clear the screen before each
// frame, so the scrollback stays as it was.
const (
	enterWatchScreen = "\x1b[?1049h\x1b[?25l"
	leaveWatchScreen = "\x1b[?25h\x1b[?1049l"
	clearScreen      = "\x1b[H\x1b[2J"
)

// watchFrames, when above 0, ends a watch after that many frames, and
// watchWait waits d before the next frame, reporting false when ctx ends
// first. Tests replace both.
var (
	watchFrames = 0
	watchWait   = func(ctx context.Context, d time.Duration) bool {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		}
	}
)

// checkWatch says why queue --watch cannot run as asked: it redraws a
// terminal, so it needs one and makes no sense with --json, and refreshing
// faster than once a second would only load the Instances.
func checkWatch(cmd *cobra.Command, interval time.Duration, flags *globalFlags) error {
	switch {
	case flags.json:
		return errors.New("--watch and --json cannot be used together; a script can run arr queue --json in a loop")
	case interval < time.Second:
		return errors.New("--interval must be 1s or more")
	}
	if _, ok := detectTerminal(cmd.OutOrStdout()); !ok {
		return errors.New("--watch needs a terminal; a script can run arr queue --json in a loop")
	}
	return nil
}

// queueWatch is what queue --watch shows: the queue of insts, joined as arr
// queue joins it when join is set, every interval. command is the command
// line it was started with, such as "arr sab queue", for the heading.
type queueWatch struct {
	command  string
	insts    []config.Instance
	join     bool
	interval time.Duration
}

// watchQueue draws v until Ctrl-C. An Instance that cannot answer is shown
// in the frame, so the watch goes on, and ending it with Ctrl-C is not a
// failure.
func watchQueue(ctx context.Context, w io.Writer, v queueWatch) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if _, err := io.WriteString(w, enterWatchScreen); err != nil {
		return err
	}
	defer io.WriteString(w, leaveWatchScreen)
	for frame := 1; ; frame++ {
		var buf bytes.Buffer
		if err := v.drawFrame(ctx, &buf); err != nil {
			return err
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
		if watchFrames > 0 && frame >= watchFrames {
			return nil
		}
		if !watchWait(ctx, v.interval) {
			return nil
		}
	}
}

// drawFrame writes one frame: a heading, the queue, how each Download Client
// is doing, and the Instances that could not answer.
func (v queueWatch) drawFrame(ctx context.Context, w io.Writer) error {
	results := forEachInstance(ctx, v.insts, fetchQueueAndDownloadClient)
	queues := make([]instanceResult[queueRecord], len(results))
	var clients []*downloadClientSummary
	for i, res := range results {
		queues[i] = res.queue
		if res.downloadClient != nil {
			clients = append(clients, res.downloadClient)
		}
	}
	recs, failures := gatherQueue(v.insts, queues, v.join)
	fmt.Fprintf(w, "%s%s, every %s · %s · Ctrl-C to stop\n\n", clearScreen, v.command, v.interval, now().Format("15:04:05"))
	if err := renderQueue(w, recs, false); err != nil {
		return err
	}
	if len(clients) > 0 || len(failures) > 0 {
		fmt.Fprintln(w)
	}
	for _, c := range clients {
		fmt.Fprintf(w, "%s: %s at %s", c.instance, c.state, formatSpeed(c.speed))
		if c.timeLeft != "" {
			fmt.Fprintf(w, ", %s left", formatTimeLeft(c.timeLeft))
		}
		fmt.Fprintln(w)
	}
	for _, f := range failures {
		fmt.Fprintf(w, "%s: %s\n", f.instance, f.err)
	}
	return nil
}
