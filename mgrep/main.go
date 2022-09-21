package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/FatwaArya/go-mgrep/worker"
	"github.com/FatwaArya/go-mgrep/worklist"
	"github.com/alexflint/go-arg"
)

func discoversDirs(wl *worklist.Worklist, path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Println("Error reading directory: ", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			nextPath := filepath.Join(path, entry.Name())
			discoversDirs(wl, nextPath)
		} else {
			wl.Add(worklist.NewJob(filepath.Join(path, entry.Name())))
		}
	}
}

var args struct {
	SearchString string `arg:"positional,required"`
	SearchDir    string `arg:"positional"`
}

func main() {
	arg.MustParse(&args)

	var workersWg sync.WaitGroup

	wl := worklist.New(100)

	results := make(chan worker.Result, 100)

	numWorkers := 10

	workersWg.Add(1)
	go func() {
		defer workersWg.Done()
		discoversDirs(&wl, args.SearchDir)
		wl.Finalize(numWorkers)
	}()

	for i := 0; i < numWorkers; i++ {
		workersWg.Add(1)
		go func() {
			defer workersWg.Done()
			for {
				workEntry := wl.Next()
				if workEntry.Path != "" {
					workerResult := worker.FindInFile(workEntry.Path, args.SearchString)
					if workerResult != nil {
						for _, r := range workerResult.Inner {
							results <- r
						}
					}
				} else {
					// When the path is empty, this indicates that there are no more jobs available,
					// so we quit.
					return
				}
			}
		}()
	}

	blockWorkersWg := make(chan struct{})
	go func() {
		workersWg.Wait()
		// Close channel
		close(blockWorkersWg)
	}()

	var displayWg sync.WaitGroup

	displayWg.Add(1)
	go func() {
		for {
			select {
			case r := <-results:
				fmt.Printf("%v[%v]:%v\n", r.Path, r.LineNum, r.Line)
			case <-blockWorkersWg:
				// Make sure channel is empty before aborting display goroutine
				if len(results) == 0 {
					displayWg.Done()
					return
				}
			}
		}
	}()
	displayWg.Wait()
}
