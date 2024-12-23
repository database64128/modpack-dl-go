package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/database64128/modpack-dl-go/download"
	"github.com/database64128/modpack-dl-go/modpacksch"
	"github.com/database64128/modpack-dl-go/precheck"
	"github.com/lmittmann/tint"
)

var (
	modpackID                      int64
	versionID                      int64
	clientPath                     string
	serverPath                     string
	migrateFromPath                string
	preserveMigrationSource        bool
	curseforge                     bool
	downloadConcurrency            int
	serverIgnoreCurseForgeProjects int64s
	logLevel                       slog.Level
)

func init() {
	flag.Int64Var(&modpackID, "modpackID", 0, "ID of the modpack to download")
	flag.Int64Var(&versionID, "versionID", 0, "Optional. Download the specified version of the modpack, instead of the latest version")
	flag.StringVar(&clientPath, "clientPath", "", "Optional. Download the modpack client to the specified path")
	flag.StringVar(&serverPath, "serverPath", "", "Optional. Download the modpack server to the specified path")
	flag.StringVar(&migrateFromPath, "migrateFromPath", "", "Optional. Migrate the modpack from the specified path")
	flag.BoolVar(&preserveMigrationSource, "preserveMigrationSource", false, "Migrate by copying instead of moving files")
	flag.BoolVar(&curseforge, "curseforge", false, "ID is a CurseForge project ID instead of a modpacks.ch public modpack ID")
	flag.IntVar(&downloadConcurrency, "downloadConcurrency", 32, "Optional. Number of concurrent downloads")
	flag.Var(&serverIgnoreCurseForgeProjects, "serverIgnoreCurseForgeProjects", "Optional. Comma-separated list of CurseForge project IDs to ignore when downloading the server")
	flag.TextVar(&logLevel, "logLevel", slog.LevelInfo, "Log level")
}

func main() {
	flag.Parse()

	if modpackID == 0 {
		fmt.Println("Please specify a modpack ID with '-modpackID'.")
		flag.Usage()
		os.Exit(1)
	}

	if downloadConcurrency <= 0 {
		fmt.Println("Download concurrency must be positive.")
		flag.Usage()
		os.Exit(1)
	}

	logger := slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level: logLevel,
	}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		logger.LogAttrs(ctx, slog.LevelInfo, "Received exit signal")
		stop()
	}()

	var client modpacksch.ModpackClient
	if !curseforge {
		client = modpacksch.DefaultPublicModpackClient
	} else {
		client = modpacksch.DefaultCurseForgeModpackClient
	}

	modpackManifest, err := client.GetModpackManifest(ctx, modpackID)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to get modpack manifest",
			slog.Int64("modpackID", modpackID),
			tint.Err(err),
		)
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Got modpack manifest",
		slog.Int64("modpackID", modpackManifest.ID),
		slog.String("name", modpackManifest.Name),
		slog.String("synopsis", modpackManifest.Synopsis),
		slog.Any("versions", modpackManifest.Versions),
	)

	if versionID == 0 {
		version, ok := modpackManifest.LatestVersion()
		if !ok {
			logger.LogAttrs(ctx, slog.LevelError, "Modpack has no versions")
			os.Exit(1)
		}
		versionID = version.ID
	}

	versionManifest, err := client.GetModpackVersionManifest(ctx, modpackID, versionID)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Failed to get modpack version manifest",
			slog.Int64("modpackID", modpackID),
			slog.Int64("versionID", versionID),
			tint.Err(err),
		)
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Got modpack version manifest",
		slog.Int64("modpackID", versionManifest.Parent),
		slog.Int64("versionID", versionManifest.ID),
		slog.String("name", versionManifest.Name),
		slog.String("type", versionManifest.Type),
		slog.Time("updated", versionManifest.Updated.Time),
		slog.Int("fileCount", len(versionManifest.Files)),
		slog.Any("targets", versionManifest.Targets),
	)

	if clientPath == "" && serverPath == "" {
		logger.LogAttrs(ctx, slog.LevelInfo, "User did not ask to download anything")
		return
	}

	pjch := make(chan precheck.Job)
	pwf := precheck.NewWorkerFleet(ctx, logger, pjch)
	dwf := download.NewWorkerFleet(ctx, logger, http.DefaultClient, downloadConcurrency, pwf.DownloadJobChannel())

	for i := range versionManifest.Files {
		file := &versionManifest.Files[i]
		pj, ok, err := file.PrecheckJob(migrateFromPath, clientPath, serverPath, serverIgnoreCurseForgeProjects, preserveMigrationSource)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to create precheck job",
				slog.Int64("modpackID", versionManifest.Parent),
				slog.Int64("versionID", versionManifest.ID),
				slog.String("name", file.Name),
				slog.String("path", file.Path),
				tint.Err(err),
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

// int64s implements [flag.Value].
type int64s []int64

// String returns the int64s as a comma-separated list.
func (i int64s) String() string {
	if len(i) == 0 {
		return ""
	}
	// Currently, a CurseForge project ID is up to 7 digits long.
	b := make([]byte, 0, 8*len(i)-1)
	b = strconv.AppendInt(b, i[0], 10)
	for _, n := range i[1:] {
		b = append(b, ',')
		b = strconv.AppendInt(b, n, 10)
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Set parses value as a comma-separated list of int64s.
func (i *int64s) Set(value string) error {
	dst := slices.Grow(*i, strings.Count(value, ",")+1)

	for {
		var (
			s     string
			found bool
		)

		s, value, found = strings.Cut(value, ",")
		if s = strings.TrimSpace(s); s != "" {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			dst = append(dst, n)
		}

		if !found {
			break
		}
	}

	*i = dst
	return nil
}
