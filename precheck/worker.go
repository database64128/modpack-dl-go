package precheck

import (
	"bytes"
	"context"
	"errors"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/database64128/modpack-dl-go/download"
	"github.com/lmittmann/tint"
)

// Job is a precheck job.
//
// A precheck job short-circuits the download process if any of the following
// conditions are met:
//
//   - The target file already exists at one of the destination paths.
//     In this case, the file is copied to the other path if it's not already there.
//   - The target file already exists at the migration source path.
//     In this case, the file is moved/copied to the destination paths.
type Job struct {
	// DownloadURL is the target file's download URL.
	DownloadURL string

	// UserAgent is the user agent to use for the request.
	// If empty, Go's default behavior is preserved.
	UserAgent string

	// DestinationPath is the destination path for downloading the file
	// or migrating an existing file to.
	DestinationPath string

	// IsClientFile indicates whether the file should be included in the modpack client.
	IsClientFile bool

	// IsServerFile indicates whether the file should be included in the modpack server.
	IsServerFile bool

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
func createFile(root *os.Root, path string) (*os.File, error) {
	f, err := root.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		// Work around https://github.com/golang/go/issues/75114.
		for {
			if err = root.MkdirAll(filepath.Dir(path), 0755); err != nil {
				if errors.Is(err, os.ErrExist) {
					continue
				}
				return nil, err
			}
			break
		}
		return root.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	}
	return f, nil
}

// checkFileContent checks the given file's content.
// The file offset will be at the end of the file after the check.
// It returns whether the content matches the expected hash sum or an error.
func (j *Job) checkFileContent(f *os.File) (bool, error) {
	h := j.NewHash()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	b := make([]byte, 0, h.Size())
	b = h.Sum(b)
	return bytes.Equal(b, j.Sum), nil
}

// checkFile checks the file's size and content.
// After the check, the file offset will be restored to the start of the file.
// It returns whether the check succeeded or an error.
func (j *Job) checkFile(f *os.File) (bool, error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	if fi.Size() != j.Size {
		return false, nil
	}

	ok, err := j.checkFileContent(f)
	if err != nil {
		return false, err
	}

	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return ok, nil
}

// openAndCheckFile opens the file at the given path for reading and checks it.
// It returns the opened checked file, whether the check succeeded, or an error.
func (j *Job) openAndCheckFile(root *os.Root, path string) (*os.File, bool, error) {
	f, err := root.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	ok, err := j.checkFile(f)
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return f, ok, nil
}

// createAndCheckFile creates and then checks the file at the given path.
// It returns the opened checked file, whether the check succeeded, or an error.
func (j *Job) createAndCheckFile(root *os.Root, path string) (*os.File, bool, error) {
	f, err := createFile(root, path)
	if err != nil {
		return nil, false, err
	}

	ok, err := j.checkFile(f)
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return f, ok, nil
}

// sendDownloadJob sends a download job to the download job channel.
func (j *Job) sendDownloadJob(djch chan<- download.Job, f1, f2 *os.File) {
	djch <- download.Job{
		DownloadURL:         j.DownloadURL,
		UserAgent:           j.UserAgent,
		TargetFile:          f1,
		SecondaryTargetFile: f2,
	}
}

// runOneDestination runs the job with a single destination.
func (j *Job) runOneDestination(
	ctx context.Context,
	logger *slog.Logger,
	djch chan<- download.Job,
	destinationRoot *os.Root,
	migrateFromRoot *os.Root,
	preserveMigrationSource bool,
) {
	dst, ok, err := j.createAndCheckFile(destinationRoot, j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create or check file at destination path",
			slog.String("root", destinationRoot.Name()),
			slog.String("path", j.DestinationPath),
			tint.Err(err),
		)
		return
	}
	if ok {
		logger.LogAttrs(ctx, slog.LevelInfo, "Skipping existing file",
			slog.String("root", destinationRoot.Name()),
			slog.String("path", j.DestinationPath),
		)
		dst.Close()
		return
	}

	if migrateFromRoot == nil {
		j.sendDownloadJob(djch, dst, nil)
		return
	}

	src, ok, err := j.openAndCheckFile(migrateFromRoot, j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to check file at migration source path",
			slog.String("root", migrateFromRoot.Name()),
			slog.String("path", j.DestinationPath),
			tint.Err(err),
		)
		dst.Close()
		return
	}
	if !ok {
		j.sendDownloadJob(djch, dst, nil)
		src.Close()
		return
	}

	srcName := src.Name()
	dstName := dst.Name()

	if !preserveMigrationSource {
		// First close the files and attempt a rename.
		src.Close()
		dst.Close()

		if err = os.Rename(srcName, dstName); err == nil {
			logger.LogAttrs(ctx, slog.LevelInfo, "Moved existing file",
				slog.String("src", srcName),
				slog.String("dst", dstName),
			)
			return
		}

		logger.LogAttrs(ctx, slog.LevelDebug, "Rename failed, falling back to copy & remove",
			slog.String("src", srcName),
			slog.String("dst", dstName),
			tint.Err(err),
		)

		// Open the files again to fall back to copy & remove.
		dst, err = os.OpenFile(dstName, os.O_RDWR, 0644)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to open file at destination path",
				slog.String("path", dstName),
				tint.Err(err),
			)
			return
		}

		src, err = os.Open(srcName)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to open file at migration source path",
				slog.String("path", srcName),
				tint.Err(err),
			)
			dst.Close()
			return
		}
	}

	if _, err = dst.ReadFrom(src); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
			slog.String("src", srcName),
			slog.String("dst", dstName),
			tint.Err(err),
		)
		src.Close()
		dst.Close()
		return
	}

	src.Close()
	dst.Close()

	logger.LogAttrs(ctx, slog.LevelInfo, "Copied existing file",
		slog.String("src", srcName),
		slog.String("dst", dstName),
	)

	if preserveMigrationSource {
		return
	}

	if err = os.Remove(srcName); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to remove migration source file",
			slog.String("path", srcName),
			tint.Err(err),
		)
		return
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Removed migration source file", slog.String("path", srcName))
}

// runTwoDestinations runs the job with two destinations.
func (j *Job) runTwoDestinations(
	ctx context.Context,
	logger *slog.Logger,
	djch chan<- download.Job,
	destinationRoot *os.Root,
	secondaryDestinationRoot *os.Root,
	migrateFromRoot *os.Root,
	preserveMigrationSource bool,
) {
	dst1, ok1, err := j.createAndCheckFile(destinationRoot, j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create or check file at destination path",
			slog.String("root", destinationRoot.Name()),
			slog.String("path", j.DestinationPath),
			tint.Err(err),
		)
		return
	}

	dst2, ok2, err := j.createAndCheckFile(secondaryDestinationRoot, j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create or check file at secondary destination path",
			slog.String("root", secondaryDestinationRoot.Name()),
			slog.String("path", j.DestinationPath),
			tint.Err(err),
		)
		dst1.Close()
		return
	}

	// Both files exist and are valid.
	if ok1 && ok2 {
		logger.LogAttrs(ctx, slog.LevelInfo, "Skipping existing files",
			slog.String("root", destinationRoot.Name()),
			slog.String("secondaryRoot", secondaryDestinationRoot.Name()),
			slog.String("path", j.DestinationPath),
		)
		dst1.Close()
		dst2.Close()
		return
	}

	// Only one of the files exists and is valid.
	if ok1 || ok2 {
		var src, dst *os.File
		if ok1 {
			src = dst1
			dst = dst2
		} else {
			src = dst2
			dst = dst1
		}

		if _, err = dst.ReadFrom(src); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
				slog.String("src", src.Name()),
				slog.String("dst", dst.Name()),
				tint.Err(err),
			)
		}

		logger.LogAttrs(ctx, slog.LevelInfo, "Copied existing file",
			slog.String("src", src.Name()),
			slog.String("dst", dst.Name()),
		)

		src.Close()
		dst.Close()
		return
	}

	// Neither file exists or is valid.
	// Check if the migration source exists.
	if migrateFromRoot == nil {
		j.sendDownloadJob(djch, dst1, dst2)
		return
	}

	src, srcOK, err := j.openAndCheckFile(migrateFromRoot, j.DestinationPath)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to check file at migration source path",
			slog.String("root", migrateFromRoot.Name()),
			slog.String("path", j.DestinationPath),
			tint.Err(err),
		)
		dst1.Close()
		dst2.Close()
		return
	}
	if !srcOK {
		j.sendDownloadJob(djch, dst1, dst2)
		src.Close()
		return
	}

	// The migration source exists and is valid.
	dst1Name := dst1.Name()
	dst2Name := dst2.Name()
	srcName := src.Name()

	var hasCopyError bool
	if _, err = dst1.ReadFrom(src); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
			slog.String("src", srcName),
			slog.String("dst", dst1Name),
			tint.Err(err),
		)
		hasCopyError = true
	} else {
		logger.LogAttrs(ctx, slog.LevelInfo, "Copied existing file",
			slog.String("src", srcName),
			slog.String("dst", dst1Name),
		)
	}

	dst1.Close()

	if !preserveMigrationSource {
		// First close the files and attempt a rename.
		dst2.Close()
		src.Close()

		if err = os.Rename(srcName, dst2Name); err == nil {
			logger.LogAttrs(ctx, slog.LevelInfo, "Moved existing file",
				slog.String("src", srcName),
				slog.String("dst", dst2Name),
			)
			return
		}

		logger.LogAttrs(ctx, slog.LevelDebug, "Rename failed, falling back to copy & remove",
			slog.String("src", srcName),
			slog.String("dst", dst2Name),
			tint.Err(err),
		)

		// Open the files again to fall back to copy & remove.
		dst2, err = os.OpenFile(dst2Name, os.O_RDWR, 0644)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to open file at secondary destination path",
				slog.String("path", dst2Name),
				tint.Err(err),
			)
			return
		}

		src, err = os.Open(srcName)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to open file at migration source path",
				slog.String("path", srcName),
				tint.Err(err),
			)
			dst2.Close()
			return
		}
	} else {
		if _, err = src.Seek(0, io.SeekStart); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to seek to start of file",
				slog.String("path", srcName),
				tint.Err(err),
			)
			dst2.Close()
			src.Close()
			return
		}
	}

	if _, err = dst2.ReadFrom(src); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to copy file",
			slog.String("src", srcName),
			slog.String("dst", dst2Name),
			tint.Err(err),
		)
		hasCopyError = true
	} else {
		logger.LogAttrs(ctx, slog.LevelInfo, "Copied existing file",
			slog.String("src", srcName),
			slog.String("dst", dst2Name),
		)
	}

	dst2.Close()
	src.Close()

	if hasCopyError || preserveMigrationSource {
		return
	}

	if err = os.Remove(srcName); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "Failed to remove migration source file",
			slog.String("path", srcName),
			tint.Err(err),
		)
		return
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Removed migration source file", slog.String("path", srcName))
}

// Run runs the job.
func (j *Job) Run(
	ctx context.Context,
	logger *slog.Logger,
	djch chan<- download.Job,
	clientRoot *os.Root,
	serverRoot *os.Root,
	migrateFromRoot *os.Root,
	preserveMigrationSource bool,
) {
	switch {
	case clientRoot != nil && serverRoot != nil && j.IsClientFile && j.IsServerFile:
		j.runTwoDestinations(ctx, logger, djch, clientRoot, serverRoot, migrateFromRoot, preserveMigrationSource)
	case clientRoot != nil && serverRoot == nil && j.IsClientFile:
		j.runOneDestination(ctx, logger, djch, clientRoot, migrateFromRoot, preserveMigrationSource)
	case clientRoot == nil && serverRoot != nil && j.IsServerFile:
		j.runOneDestination(ctx, logger, djch, serverRoot, migrateFromRoot, preserveMigrationSource)
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
func NewWorkerFleet(
	ctx context.Context,
	logger *slog.Logger,
	pjch <-chan Job,
	clientRoot *os.Root,
	serverRoot *os.Root,
	migrateFromRoot *os.Root,
	preserveMigrationSource bool,
) *WorkerFleet {
	wf := WorkerFleet{
		djch: make(chan download.Job),
	}
	ncpu := runtime.NumCPU()
	wf.wg.Add(ncpu)
	for range ncpu {
		go func() {
			defer wf.wg.Done()
			done := ctx.Done()
			for pj := range pjch {
				select {
				case <-done:
					continue
				default:
					pj.Run(ctx, logger, wf.djch, clientRoot, serverRoot, migrateFromRoot, preserveMigrationSource)
				}
			}
		}()
	}
	return &wf
}

// DownloadJobChannel returns the download job channel.
func (wf *WorkerFleet) DownloadJobChannel() <-chan download.Job {
	return wf.djch
}

// Wait waits for all workers to finish and closes the download job channel.
func (wf *WorkerFleet) Wait() {
	wf.wg.Wait()
	close(wf.djch)
}
