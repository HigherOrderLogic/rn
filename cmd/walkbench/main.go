// Command walkbench measures how long it takes Rune's production,
// goroutine-parallel filesystem walker (workspace/walkdir.ListFiles)
// to traverse a directory tree, and compares it against a plain
// single-threaded filepath.WalkDir over the same tree.
//
// It exists to put real numbers behind the "single-threaded UI,
// goroutine parallelism where it pays" story: ListFiles runs a pool
// of runtime.NumCPU()*8 workers fed by a channel, with an inline
// work-stealing fallback when every worker is busy.
//
// Usage:
//
//	go run ./cmd/walkbench [root]
//
// root defaults to the user's home directory. Pass "/" to scan the
// whole filesystem (expect permission errors on system paths; they
// are counted, not fatal).
//
// Flags:
//
//	-mode  which walker to run: "both" (default), "conc", or "seq".
//	-n     number of timed repetitions (default 1); the min is reported.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/unstablebuild/rune-go-sdk/api/config"
	"github.com/unstablebuild/rune-go-sdk/api/workspaceapi"
	"unstable.build/go-tui/workspace"
	"unstable.build/go-tui/workspace/walkdir"
)

func main() {
	mode := flag.String("mode", "both", `walker: "both", "conc", or "seq"`)
	n := flag.Int("n", 1, "timed repetitions; min is reported")
	workers := flag.Int("workers", 0, "ListFiles worker count (0 = default NumCPU*8)")
	cpuprof := flag.String("cpuprofile", "", "write CPU profile to file")
	flag.Parse()

	root := os.Getenv("HOME")
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve %q: %v", root, err)
	}

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			fatal("create cpuprofile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fatal("start cpuprofile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	fmt.Printf("root:    %s\n", abs)
	defWorkers := *workers
	if defWorkers == 0 {
		defWorkers = max(runtime.NumCPU()/2, 2)
	}
	fmt.Printf("cpus:    %d (workers: %d)\n", runtime.NumCPU(), defWorkers)
	fmt.Printf("repeats: %d (reporting fastest)\n", *n)
	if *workers > 0 {
		fmt.Printf("override workers: %d\n", *workers)
	}
	fmt.Println()

	var concDur, seqDur time.Duration
	if *mode == "both" || *mode == "conc" {
		files, errs, dur := best(*n, func() (int, int, time.Duration) { return runConcurrent(abs, *workers) })
		concDur = dur
		fmt.Printf("walkdir.ListFiles (parallel):  %8d files  %8d errors  %v\n",
			files, errs, dur.Round(time.Millisecond))
	}
	if *mode == "both" || *mode == "seq" {
		files, errs, dur := best(*n, func() (int, int, time.Duration) { return runSequential(abs) })
		seqDur = dur
		fmt.Printf("filepath.WalkDir  (single):    %8d files  %8d errors  %v\n",
			files, errs, dur.Round(time.Millisecond))
	}
	if *mode == "both" && concDur > 0 {
		fmt.Printf("\nspeedup: %.2fx\n", float64(seqDur)/float64(concDur))
	}
}

// best runs fn n times and returns the result with the smallest
// duration, which filters out scheduler and I/O noise.
func best(n int, fn func() (int, int, time.Duration)) (files, errs int, dur time.Duration) {
	for i := 0; i < n; i++ {
		f, e, d := fn()
		if i == 0 || d < dur {
			files, errs, dur = f, e, d
		}
	}
	return files, errs, dur
}

// runConcurrent drives Rune's production parallel walker over root.
func runConcurrent(root string, workers int) (files, errs int, dur time.Duration) {
	uri, err := workspaceapi.CurrentUserHostURI(root)
	if err != nil {
		fatal("uri: %v", err)
	}
	scheme, err := workspace.NewFileScheme(context.Background(), config.NopConfig(), uri)
	if err != nil {
		fatal("file scheme: %v", err)
	}

	ctx := context.Background()
	if workers > 0 {
		ctx = walkdir.ContextWithWorkerCount(ctx, workers)
	}
	start := time.Now()
	it, err := walkdir.ListFiles(ctx, scheme, ".")
	if err != nil {
		fatal("list files: %v", err)
	}
	for {
		_, ok := it.Next(context.Background())
		if !ok {
			break
		}
		files++
	}
	dur = time.Since(start)
	if it.Err() != nil {
		errs = 1 // aggregated permission errors collapse into one multierr
	}
	_ = it.Close()
	return files, errs, dur
}

// runSequential is the naive baseline: one goroutine, one syscall at a
// time, over the same tree.
func runSequential(root string) (files, errs int, dur time.Duration) {
	start := time.Now()
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			errs++
			return nil // count and keep going, like the parallel walker
		}
		if d.Type().IsRegular() {
			files++
		}
		return nil
	})
	dur = time.Since(start)
	return files, errs, dur
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
