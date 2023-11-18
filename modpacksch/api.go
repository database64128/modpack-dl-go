// Package modpacksch implements an API client for downloading modpacks from https://api.modpacks.ch/.
//
// API documentation: https://modpacksch.docs.apiary.io/
package modpacksch

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/database64128/modpack-dl-go/precheck"
)

const (
	// APIBaseURL is the base URL of the API.
	APIBaseURL = "https://api.modpacks.ch"

	// APIPublicModpack is the path of the public modpack endpoint.
	APIPublicModpack = "/public/modpack"

	// APIUserAgent is the user agent used for API requests.
	// The FTB folks don't like seeing people download their stuff from unofficial clients,
	// so we pretend to be https://github.com/CreeperHost/modpacksch-serverdownloader.
	APIUserAgent = "modpackserverdownloader/1.0"
)

// ModpackClient is a client for the modpacks.ch API.
type ModpackClient struct {
	client *http.Client
}

// NewModpackClient creates a new [ModpackClient].
func NewModpackClient(client *http.Client) *ModpackClient {
	return &ModpackClient{client: client}
}

// GetPublicModpackManifest gets the manifest of a public modpack with the given ID.
func (c *ModpackClient) GetPublicModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return getPublicModpackManifest(ctx, c.client, modpackID)
}

// GetPublicModpackVersionManifest gets the manifest of a public modpack version with the given modpack and version IDs.
func (c *ModpackClient) GetPublicModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error) {
	return getPublicModpackVersionManifest(ctx, c.client, modpackID, versionID)
}

// GetPublicModpackManifest gets the manifest of a public modpack with the given ID.
func GetPublicModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return getPublicModpackManifest(ctx, http.DefaultClient, modpackID)
}

// GetPublicModpackVersionManifest gets the manifest of a public modpack version with the given modpack and version IDs.
func GetPublicModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error) {
	return getPublicModpackVersionManifest(ctx, http.DefaultClient, modpackID, versionID)
}

func getPublicModpackManifest(ctx context.Context, client *http.Client, modpackID int64) (ModpackManifest, error) {
	resp, err := sendGetRequest(ctx, client, fmt.Sprintf(APIBaseURL+APIPublicModpack+"/%d", modpackID))
	if err != nil {
		return ModpackManifest{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var manifest ModpackManifest
	if err = json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return ModpackManifest{}, fmt.Errorf("failed to decode response: %w", err)
	}
	return manifest, nil
}

func getPublicModpackVersionManifest(ctx context.Context, client *http.Client, modpackID, versionID int64) (ModpackVersionManifest, error) {
	resp, err := sendGetRequest(ctx, client, fmt.Sprintf(APIBaseURL+APIPublicModpack+"/%d/%d", modpackID, versionID))
	if err != nil {
		return ModpackVersionManifest{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var manifest ModpackVersionManifest
	if err = json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return ModpackVersionManifest{}, fmt.Errorf("failed to decode response: %w", err)
	}
	return manifest, nil
}

// sendGetRequest sends a GET request to the given URL.
func sendGetRequest(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header["User-Agent"] = []string{APIUserAgent}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	return resp, nil
}

// ModpackManifest is the manifest of a modpack.
// This is the response of GET /public/modpack/{modpack_id}.
type ModpackManifest struct {
	Synopsis     string           `json:"synopsis"`
	Description  string           `json:"description"`
	Art          []ModpackArt     `json:"art"`
	Links        []ModpackLink    `json:"links"`
	Authors      []ModpackAuthor  `json:"authors"`
	Versions     []ModpackVersion `json:"versions"`
	Installs     int64            `json:"installs"`
	Plays        int64            `json:"plays"`
	Tags         []ModpackTag     `json:"tags"`
	Featured     bool             `json:"featured"`
	Refreshed    Time             `json:"refreshed"`
	Notification string           `json:"notification"`
	Rating       ModpackRating    `json:"rating"`
	Status       string           `json:"status"`
	Released     Time             `json:"released"`
	Provider     string           `json:"provider"`
	Plays14D     int64            `json:"plays_14d"`
	ResourceBase
	Private bool `json:"private"`
}

// LatestVersion returns the latest version of a modpack.
func (m *ModpackManifest) LatestVersion() (ModpackVersion, bool) {
	if len(m.Versions) == 0 {
		return ModpackVersion{}, false
	}
	return m.Versions[len(m.Versions)-1], true
}

// ModpackArt is an image of a modpack.
type ModpackArt struct {
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Compressed bool     `json:"compressed"`
	URL        string   `json:"url"`
	Mirrors    []string `json:"mirrors"`
	SHA1       string   `json:"sha1"`
	Size       int64    `json:"size"`
	ID         int64    `json:"id"`
	Type       string   `json:"type"`
	Updated    Time     `json:"updated"`
}

// ModpackLink is a modpack's miscellaneous link.
type ModpackLink struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Link string `json:"link"`
	Type string `json:"type"`
}

// ModpackAuthor is a modpack's author.
type ModpackAuthor struct {
	Website string `json:"website"`
	ResourceBase
}

// ModpackVersion is a modpack's version.
type ModpackVersion struct {
	Specs   ModpackVersionSpecs    `json:"specs"`
	Targets []ModpackVersionTarget `json:"targets"`
	ResourceBase
	Private bool `json:"private"`
}

// ModpackVersionSpecs is a modpack's version specifications.
type ModpackVersionSpecs struct {
	ID          int64 `json:"id"`
	Minimum     int   `json:"minimum"`
	Recommended int   `json:"recommended"`
}

// ModpackVersionTarget is a modpack's version target.
type ModpackVersionTarget struct {
	Version string `json:"version"`
	ResourceBase
}

// ModpackTag is a modpack's tag.
type ModpackTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ModpackRating is a modpack's rating.
type ModpackRating struct {
	ID             int64 `json:"id"`
	Configured     bool  `json:"configured"`
	Verified       bool  `json:"verified"`
	Age            int   `json:"age"`
	Gambling       bool  `json:"gambling"`
	Frightening    bool  `json:"frightening"`
	Alcoholdrugs   bool  `json:"alcoholdrugs"`
	Nuditysexual   bool  `json:"nuditysexual"`
	Sterotypeshate bool  `json:"sterotypeshate"`
	Language       bool  `json:"language"`
	Violence       bool  `json:"violence"`
}

// ModpackVersionManifest is the manifest of a modpack version.
// This is the response of GET /public/modpack/{modpack_id}/{version_id}.
type ModpackVersionManifest struct {
	Files        []ModpackVersionFile   `json:"files"`
	Specs        ModpackVersionSpecs    `json:"specs"`
	Targets      []ModpackVersionTarget `json:"targets"`
	Installs     int64                  `json:"installs"`
	Plays        int64                  `json:"plays"`
	Refreshed    Time                   `json:"refreshed"`
	Changelog    string                 `json:"changelog"`
	Parent       int64                  `json:"parent"`
	Notification string                 `json:"notification"`

	// "links" array has no content.

	Status string `json:"status"`
	ResourceBase
	Private bool `json:"private"`
}

// ModpackVersionFile is a file in a modpack version's file list.
type ModpackVersionFile struct {
	Version string   `json:"version"`
	Path    string   `json:"path"`
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors"`
	SHA1    string   `json:"sha1"`
	Size    int64    `json:"size"`

	// "tags" array has no content.

	ClientOnly bool `json:"clientonly"`
	ServerOnly bool `json:"serveronly"`
	Optional   bool `json:"optional"`
	ResourceBase
}

// PrecheckJob returns a precheck job for the file.
func (f *ModpackVersionFile) PrecheckJob(
	migrateFromPath, clientPath, serverPath string,
	preserveMigrationSource bool,
) (precheck.Job, bool, error) {
	var destinationPath, secondaryDestinationPath string
	if !f.ServerOnly && clientPath != "" {
		destinationPath = filepath.Join(clientPath, f.Path, f.Name)
	}
	if !f.ClientOnly && serverPath != "" {
		secondaryDestinationPath = filepath.Join(serverPath, f.Path, f.Name)
	}

	if destinationPath == "" {
		if secondaryDestinationPath == "" {
			return precheck.Job{}, false, nil
		}
		destinationPath = secondaryDestinationPath
		secondaryDestinationPath = ""
	}

	if migrateFromPath != "" {
		migrateFromPath = filepath.Join(migrateFromPath, f.Path, f.Name)
	}

	sum, err := hex.DecodeString(f.SHA1)
	if err != nil {
		return precheck.Job{}, false, fmt.Errorf("failed to decode SHA1: %w", err)
	}

	return precheck.Job{
		DownloadURL:              f.URL,
		UserAgent:                APIUserAgent,
		MigrateFromPath:          migrateFromPath,
		PreserveMigrationSource:  preserveMigrationSource,
		DestinationPath:          destinationPath,
		SecondaryDestinationPath: secondaryDestinationPath,
		NewHash:                  sha1.New,
		Sum:                      sum,
		Size:                     f.Size,
	}, true, nil
}

// FileBase contains basic information about a remote file.
type FileBase struct {
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors"`
	SHA1    string   `json:"sha1"`
	Size    int64    `json:"size"`
	ID      int64    `json:"id"`
	Type    string   `json:"type"`
	Updated Time     `json:"updated"`
}

// ResourceBase contains basic information about a remote resource.
type ResourceBase struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Updated Time   `json:"updated"`
}

// Time marshals into and unmarshals from a Unix timestamp in seconds.
type Time time.Time

// MarshalJSON implements [json.Marshaler].
func (t Time) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, time.Time(t).Unix(), 10), nil
}

// UnmarshalJSON implements [json.Unmarshaler].
func (t *Time) UnmarshalJSON(data []byte) error {
	secs, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse Unix timestamp: %w", err)
	}
	*t = Time(time.Unix(secs, 0))
	return nil
}
