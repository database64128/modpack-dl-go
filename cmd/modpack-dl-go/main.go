package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/database64128/modpack-dl-go/download"
	"github.com/database64128/modpack-dl-go/modpacksch"
	"github.com/database64128/modpack-dl-go/precheck"
)

var (
	modpackID               = flag.Int64("modpackID", 0, "ID of the modpack to download")
	versionID               = flag.Int64("versionID", 0, "Optional. Download the specified version of the modpack, instead of the latest version")
	clientPath              = flag.String("clientPath", "", "Optional. Download the modpack client to the specified path")
	serverPath              = flag.String("serverPath", "", "Optional. Download the modpack server to the specified path")
	migrateFromPath         = flag.String("migrateFromPath", "", "Optional. Migrate the modpack from the specified path")
	preserveMigrationSource = flag.Bool("preserveMigrationSource", false, "Migrate by copying instead of moving files")
	downloadConcurrency     = flag.Int("downloadConcurrency", 32, "Optional. Number of concurrent downloads")
)

var logLevel slog.Level

func init() {
	flag.TextVar(&logLevel, "logLevel", slog.LevelInfo, "Log level")
}

func main() {
	flag.Parse()

	if *modpackID == 0 {
		fmt.Println("Please specify a modpack ID with '-modpackID'.")
		flag.Usage()
		os.Exit(1)
	}

	if *downloadConcurrency <= 0 {
		fmt.Println("Download concurrency must be positive.")
		flag.Usage()
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.LogAttrs(ctx, slog.LevelInfo, "Received exit signal", slog.Any("signal", sig))
		cancel()
	}()

	modpackManifest, err := modpacksch.GetPublicModpackManifest(ctx, *modpackID)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to get modpack manifest",
			slog.Int64("modpackID", *modpackID),
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Got modpack manifest",
		slog.Int64("modpackID", modpackManifest.ID),
		slog.String("name", modpackManifest.Name),
		slog.String("synopsis", modpackManifest.Synopsis),
		slog.Any("versions", modpackManifest.Versions),
	)

	if *versionID == 0 {
		version, ok := modpackManifest.LatestVersion()
		if !ok {
			logger.LogAttrs(ctx, slog.LevelError, "Modpack has no versions")
			os.Exit(1)
		}
		*versionID = version.ID
	}

	versionManifest, err := modpacksch.GetPublicModpackVersionManifest(ctx, *modpackID, *versionID)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to get modpack version manifest",
			slog.Int64("modpackID", *modpackID),
			slog.Int64("versionID", *versionID),
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Got modpack version manifest",
		slog.Int64("modpackID", versionManifest.Parent),
		slog.Int64("versionID", versionManifest.ID),
		slog.String("name", versionManifest.Name),
		slog.String("type", versionManifest.Type),
		slog.Time("updated", time.Time(versionManifest.Updated)),
		slog.Int("fileCount", len(versionManifest.Files)),
	)

	if *clientPath == "" && *serverPath == "" {
		logger.LogAttrs(ctx, slog.LevelInfo, "User did not ask to download anything")
		return
	}

	pjch := make(chan precheck.Job)
	pwf := precheck.NewWorkerFleet(ctx, logger, pjch)
	dwf := download.NewWorkerFleet(ctx, logger, http.DefaultClient, *downloadConcurrency, pwf.DownloadJobChannel())

	for i := range versionManifest.Files {
		file := &versionManifest.Files[i]
		pj, ok, err := file.PrecheckJob(*migrateFromPath, *clientPath, *serverPath, *preserveMigrationSource)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create precheck job",
				slog.Int64("modpackID", versionManifest.Parent),
				slog.Int64("versionID", versionManifest.ID),
				slog.String("name", file.Name),
				slog.String("path", file.Path),
				slog.Any("error", err),
			)
			continue
		}
		if !ok {
			continue
		}
		pjch <- pj
	}

	close(pjch)
	pwf.Wait()
	dwf.Wait()
}
