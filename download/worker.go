package download

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// Job is a download job.
type Job struct {
	// DownloadURL is the target file's download URL.
	DownloadURL string

	// UserAgent is the user agent to use for the request.
	// If empty, Go's default behavior is preserved.
	UserAgent string

	// TargetFile is the target file.
	TargetFile *os.File

	// SecondaryTargetFile is the secondary target file.
	// Nil means no secondary target file.
	SecondaryTargetFile *os.File
}

// mtimeFromResponse returns the modification time from the response.
func mtimeFromResponse(ctx context.Context, logger *slog.Logger, resp *http.Response) time.Time {
	lastModified := resp.Header["Last-Modified"]
	if len(lastModified) != 1 {
		logger.LogAttrs(ctx, slog.LevelWarn, "Malformed Last-Modified header",
			slog.Any("Last-Modified", lastModified),
		)
		return time.Time{}
	}
	mtime, err := time.Parse(http.TimeFormat, lastModified[0])
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to parse Last-Modified header",
			slog.String("Last-Modified", lastModified[0]),
			slog.Any("error", err),
		)
		return time.Time{}
	}
	return mtime
}

// run runs the job, closes the target files, and returns the modification time of the file
// as reported by the server. It's up to the caller to actually set the modification time.
func (j *Job) run(ctx context.Context, logger *slog.Logger, client *http.Client) (mtime time.Time) {
	defer func() {
		j.TargetFile.Close()
		if j.SecondaryTargetFile != nil {
			j.SecondaryTargetFile.Close()
		}
	}()

	logger.LogAttrs(ctx, slog.LevelInfo, "Downloading file",
		slog.String("name", j.TargetFile.Name()),
		slog.String("url", j.DownloadURL),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.DownloadURL, nil)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create request",
			slog.String("name", j.TargetFile.Name()),
			slog.String("url", j.DownloadURL),
			slog.Any("error", err),
		)
		return
	}

	if j.UserAgent != "" {
		req.Header["User-Agent"] = []string{j.UserAgent}
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to send request",
			slog.String("name", j.TargetFile.Name()),
			slog.String("url", j.DownloadURL),
			slog.Any("error", err),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.LogAttrs(ctx, slog.LevelWarn, "Unexpected status code",
			slog.String("name", j.TargetFile.Name()),
			slog.String("url", j.DownloadURL),
			slog.Int("status", resp.StatusCode),
		)
		return
	}

	if _, err = j.TargetFile.ReadFrom(resp.Body); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to download file",
			slog.String("name", j.TargetFile.Name()),
			slog.String("url", j.DownloadURL),
			slog.Any("error", err),
		)
		return
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Downloaded file",
		slog.String("name", j.TargetFile.Name()),
		slog.String("url", j.DownloadURL),
	)

	if j.SecondaryTargetFile != nil {
		if _, err = j.TargetFile.Seek(0, io.SeekStart); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to seek to start of file",
				slog.String("name", j.TargetFile.Name()),
				slog.Any("error", err),
			)
			return
		}

		if _, err = j.SecondaryTargetFile.ReadFrom(j.TargetFile); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
				slog.String("src", j.TargetFile.Name()),
				slog.String("dst", j.SecondaryTargetFile.Name()),
				slog.Any("error", err),
			)
			return
		}

		logger.LogAttrs(ctx, slog.LevelInfo, "Copied to secondary file",
			slog.String("src", j.TargetFile.Name()),
			slog.String("dst", j.SecondaryTargetFile.Name()),
		)
	}

	return mtimeFromResponse(ctx, logger, resp)
}

// Run runs the job.
func (j *Job) Run(ctx context.Context, logger *slog.Logger, client *http.Client) {
	mtime := j.run(ctx, logger, client)
	if mtime.IsZero() {
		return
	}

	if err := os.Chtimes(j.TargetFile.Name(), mtime, mtime); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to set modification time",
			slog.String("name", j.TargetFile.Name()),
			slog.Any("error", err),
		)
		return
	}

	if j.SecondaryTargetFile != nil {
		if err := os.Chtimes(j.SecondaryTargetFile.Name(), mtime, mtime); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to set modification time",
				slog.String("name", j.SecondaryTargetFile.Name()),
				slog.Any("error", err),
			)
			return
		}
	}
}

// WorkerFleet manages a fleet of workers.
type WorkerFleet struct {
	wg sync.WaitGroup
}

// NewWorkerFleet creates a new worker fleet with the given number of workers.
//
// The workers pick up jobs from the given channel and run them.
//
// After use, close the channel to stop the workers.
// Call the Wait method to wait for the workers to finish.
func NewWorkerFleet(ctx context.Context, logger *slog.Logger, client *http.Client, numWorkers int, jobCh <-chan Job) *WorkerFleet {
	var wf WorkerFleet
	wf.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wf.wg.Done()
			done := ctx.Done()
			for job := range jobCh {
				select {
				case <-done:
					continue
				default:
					job.Run(ctx, logger, client)
				}
			}
		}()
	}
	return &wf
}

// Wait waits for the workers to finish.
func (wf *WorkerFleet) Wait() {
	wf.wg.Wait()
}
