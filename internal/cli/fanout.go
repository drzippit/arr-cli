package cli

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/drzippit/arr-cli/internal/config"
)

// forEachInstance runs fn for every Instance at the same time and returns the
// results in config order.
func forEachInstance[T any](ctx context.Context, insts []config.Instance, fn func(context.Context, config.Instance) T) []T {
	out := make([]T, len(insts))
	var wg sync.WaitGroup
	for i, inst := range insts {
		wg.Go(func() { out[i] = fn(ctx, inst) })
	}
	wg.Wait()
	return out
}

// failure is one Instance that could not answer a command.
type failure struct {
	instance string
	err      string
}

// reportFailures writes one line per failed Instance to w and returns a
// silentError if there were any, so the exit code is non-zero.
func reportFailures(w io.Writer, failures []failure) error {
	for _, f := range failures {
		fmt.Fprintf(w, "arr: %s: %s\n", f.instance, f.err)
	}
	if len(failures) > 0 {
		return silentError{fmt.Errorf("%d Instance(s) failed", len(failures))}
	}
	return nil
}
