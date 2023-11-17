package precheck

import (
	"bytes"
	"context"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/database64128/modpack-dl-go/download"
)

// Job is a precheck job.
//
// A precheck job short-circuits the download process if any of the following
// conditions are met:
//
//   - The target file already exists at DestinationPath or SecondaryDestinationPath.
//     In this case, the file is copied to the other path if it's not already there.
//   - The target file already exists at MigrateFromPath. In this case, the file is
//     moved/copied to DestinationPath and copied to SecondaryDestinationPath.
type Job struct {
	// DownloadURL is the target file's download URL.
	DownloadURL string

	// MigrateFromPath is the path to a possible existing file.
	// The path may be empty. The file may not exist or may have different content.
	MigrateFromPath string

	// PreserveMigrationSource controls whether to preserve the file at
	// MigrateFromPath should a migration happen.
	PreserveMigrationSource bool

	// DestinationPath is the destination path for downloading the file
	// or migrating an existing file to.
	DestinationPath string

	// SecondaryDestinationPath specifies where to put a copy of the file.
	// If empty, no copy is made.
	SecondaryDestinationPath string

	// NewHash is the function that returns a [hash.Hash] for verifying the file content.
	NewHash func() hash.Hash

	// Sum is the expected hash sum of the file.
	Sum []byte

	// Size is the expected size of the file.
	Size int64
}

// createFile creates the file at the given path.
// The parent directory will be created if it doesn't exist.
// It returns the opened created file or an error.
func createFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	}
	return f, nil
}

// checkFile checks the file at the given path.
// It returns the opened checked file, whether the check succeeded, or an error.
func (j *Job) checkFile(path string) (*os.File, bool, error) {
	f, err := createFile(path)
	if err != nil {
		return nil, false, err
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, false, err
	}
	if fi.Size() != j.Size {
		return f, false, nil
	}

	h := j.NewHash()
	if _, err = io.Copy(h, f); err != nil {
		f.Close()
		return nil, false, err
	}

	b := make([]byte, 0, h.Size())
	b = h.Sum(b)
	return f, bytes.Equal(b, j.Sum), nil
}

// sendDownloadJob sends a download job to the download job channel.
func (j *Job) sendDownloadJob(ctx context.Context, logger *slog.Logger, djch chan<- download.Job, f1, f2 *os.File) {
	djch <- download.Job{
		DownloadURL:         j.DownloadURL,
		TargetFile:          f1,
		SecondaryTargetFile: f2,
	}
}

// copyWholeFile copies the whole file from src to dst.
// It returns the number of bytes copied or an error.
func copyWholeFile(dst, src *os.File) (int64, error) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek to start of source file: %w", err)
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek to start of destination file: %w", err)
	}
	return dst.ReadFrom(src)
}

// Run runs the job.
func (j *Job) Run(ctx context.Context, logger *slog.Logger, djch chan<- download.Job) {
	f1, ok1, err := j.checkFile(j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to check file at destination path",
			slog.String("path", j.DestinationPath),
			slog.Any("error", err),
		)
		return
	}

	hasSecondary := j.SecondaryDestinationPath != ""
	if !hasSecondary {
		if ok1 {
			logger.LogAttrs(ctx, slog.LevelInfo, "Skipping existing file",
				slog.String("path", j.DestinationPath),
			)
			f1.Close()
			return
		}
		j.sendDownloadJob(ctx, logger, djch, f1, nil)
		return
	}

	f2, ok2, err := j.checkFile(j.SecondaryDestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to check file at secondary destination path",
			slog.String("path", j.SecondaryDestinationPath),
			slog.Any("error", err),
		)
		f1.Close()
		return
	}

	// Both files exist and are valid.
	if ok1 && ok2 {
		logger.LogAttrs(ctx, slog.LevelInfo, "Skipping existing files",
			slog.String("path", j.DestinationPath),
			slog.String("secondaryPath", j.SecondaryDestinationPath),
		)
		f1.Close()
		f2.Close()
		return
	}

	// Only one of the files exists and is valid.
	if ok1 || ok2 {
		var src, dst *os.File
		if ok1 {
			src = f1
			dst = f2
		} else {
			src = f2
			dst = f1
		}
		if _, err = copyWholeFile(dst, src); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
				slog.String("src", src.Name()),
				slog.String("dst", dst.Name()),
				slog.Any("error", err),
			)
		}
		src.Close()
		dst.Close()
		return
	}

	// Neither file exists or is valid.
	// Check if the migration source exists.
	if j.MigrateFromPath == "" {
		j.sendDownloadJob(ctx, logger, djch, f1, f2)
		return
	}

	f3, ok3, err := j.checkFile(j.MigrateFromPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to check file at migration source path",
			slog.String("path", j.MigrateFromPath),
			slog.Any("error", err),
		)
		f1.Close()
		f2.Close()
		return
	}
	if !ok3 {
		j.sendDownloadJob(ctx, logger, djch, f1, f2)
		f3.Close()
		return
	}

	// The migration source exists and is valid.
	// TODO: Support moving the file instead of copying it.

	var hasCopyError bool
	if _, err = copyWholeFile(f1, f3); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
			slog.String("src", f3.Name()),
			slog.String("dst", f1.Name()),
			slog.Any("error", err),
		)
		hasCopyError = true
	}
	if _, err = copyWholeFile(f2, f3); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
			slog.String("src", f3.Name()),
			slog.String("dst", f2.Name()),
			slog.Any("error", err),
		)
		hasCopyError = true
	}

	f3.Close()

	if !hasCopyError && !j.PreserveMigrationSource {
		if err = os.Remove(j.MigrateFromPath); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to remove migration source file",
				slog.String("path", j.MigrateFromPath),
				slog.Any("error", err),
			)
		}
	}
}

// WorkerFleet manages a fleet of workers.
type WorkerFleet struct {
	wg   sync.WaitGroup
	djch chan download.Job
}

// NewWorkerFleet creates a fleet of [runtime.NumCPU] workers.
//
// These workers pick up precheck jobs from the given channel and
// produce download jobs to a download job channel.
//
// After use, close the precheck job channel to stop the workers.
// Call the Wait method to wait for all workers to finish, and it
// will close the download job channel.
func NewWorkerFleet(ctx context.Context, logger *slog.Logger, pjch <-chan Job) (wf *WorkerFleet, djch <-chan download.Job) {
	wf = &WorkerFleet{
		djch: make(chan download.Job),
	}
	ncpu := runtime.NumCPU()
	wf.wg.Add(ncpu)
	for i := 0; i < ncpu; i++ {
		go func() {
			defer wf.wg.Done()
			for pj := range pjch {
				pj.Run(ctx, logger, wf.djch)
			}
		}()
	}
	return
}

// Wait waits for all workers to finish and closes the download job channel.
func (wf *WorkerFleet) Wait() {
	wf.wg.Wait()
	close(wf.djch)
}
