// Package modpacksch implements an API client for downloading modpacks from https://api.modpacks.ch/.
//
// API documentation: https://modpacksch.docs.apiary.io/
package modpacksch

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"iter"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/database64128/modpack-dl-go/precheck"
)

const (
	// APIBaseURL is the base URL of the API.
	APIBaseURL = "https://api.modpacks.ch"

	// APIPublicModpack is the path of the public modpack endpoint.
	APIPublicModpack = "/public/modpack"

	// APIPublicCurseForge is the path of the public CurseForge endpoint.
	APIPublicCurseForge = "/public/curseforge"

	// APIUserAgent is the user agent used for API requests.
	// The FTB folks don't like seeing people download their stuff from unofficial clients,
	// so we pretend to be https://github.com/CreeperHost/modpacksch-serverdownloader.
	APIUserAgent = "modpackserverdownloader/1.0"

	// FTBModpackBaseURL is the base URL of the FTB modpack endpoint.
	FTBModpackBaseURL = "https://api.feed-the-beast.com/v1/modpacks/modpack"

	// FTBModpackUserAgent is the user agent used for FTB modpack requests.
	//
	// The FTB folks don't like seeing people download their stuff from unofficial clients,
	// so we pretend to be https://github.com/FTBTeam/FTB-Server-Installer.
	FTBModpackUserAgent = "ftb-server-installer/1.0.48"
)

var (
	ErrMissingURL  = errors.New("missing URL")
	ErrMissingPath = errors.New("missing path")
)

// ModpackClient is a modpack client for the modpacks.ch API.
type ModpackClient interface {
	// GetModpackManifest gets the manifest of a modpack with the given ID.
	GetModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error)

	// GetModpackVersionManifest gets the manifest of a modpack version with the given modpack ID and version ID.
	GetModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error)
}

// PublicModpackClient is a modpack client for the modpacks.ch public modpack API.
//
// PublicModpackClient implements [ModpackClient].
type PublicModpackClient struct {
	client *http.Client
}

// NewPublicModpackClient creates a new [PublicModpackClient].
func NewPublicModpackClient(client *http.Client) *PublicModpackClient {
	return &PublicModpackClient{client: client}
}

// GetModpackManifest implements [ModpackClient.GetModpackManifest].
func (c *PublicModpackClient) GetModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return doGetRequest[ModpackManifest](ctx, c.client, fmt.Sprintf(APIBaseURL+APIPublicModpack+"/%d", modpackID), APIUserAgent)
}

// GetModpackVersionManifest implements [ModpackClient.GetModpackVersionManifest].
func (c *PublicModpackClient) GetModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error) {
	m, err := c.GetPublicModpackVersionManifest(ctx, modpackID, versionID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetPublicModpackVersionManifest gets the manifest of a public modpack version with the given modpack ID and version ID.
func (c *PublicModpackClient) GetPublicModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (PublicModpackVersionManifest, error) {
	return doGetRequest[PublicModpackVersionManifest](ctx, c.client, fmt.Sprintf(APIBaseURL+APIPublicModpack+"/%d/%d", modpackID, versionID), APIUserAgent)
}

// CurseForgeModpackClient is a modpack client for the modpacks.ch CurseForge modpack API.
//
// CurseForgeModpackClient implements [ModpackClient].
type CurseForgeModpackClient struct {
	client *http.Client
}

// NewCurseForgeModpackClient creates a new [CurseForgeModpackClient].
func NewCurseForgeModpackClient(client *http.Client) *CurseForgeModpackClient {
	return &CurseForgeModpackClient{client: client}
}

// GetModpackManifest implements [ModpackClient.GetModpackManifest].
func (c *CurseForgeModpackClient) GetModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return doGetRequest[ModpackManifest](ctx, c.client, fmt.Sprintf(APIBaseURL+APIPublicCurseForge+"/%d", modpackID), APIUserAgent)
}

// GetModpackVersionManifest implements [ModpackClient.GetModpackVersionManifest].
func (c *CurseForgeModpackClient) GetModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error) {
	m, err := c.GetCurseForgeModpackVersionManifest(ctx, modpackID, versionID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetCurseForgeModpackVersionManifest gets the manifest of a CurseForge modpack version with the given modpack ID and version ID.
func (c *CurseForgeModpackClient) GetCurseForgeModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (PublicModpackVersionManifest, error) {
	return doGetRequest[PublicModpackVersionManifest](ctx, c.client, fmt.Sprintf(APIBaseURL+APIPublicCurseForge+"/%d/%d", modpackID, versionID), APIUserAgent)
}

// FTBModpackClient is a modpack client for the FTB modpack API.
//
// FTBModpackClient implements [ModpackClient].
type FTBModpackClient struct {
	client *http.Client
}

// NewFTBModpackClient creates a new [FTBModpackClient].
func NewFTBModpackClient(client *http.Client) *FTBModpackClient {
	return &FTBModpackClient{client: client}
}

// GetModpackManifest implements [ModpackClient.GetModpackManifest].
func (c *FTBModpackClient) GetModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return doGetRequest[ModpackManifest](ctx, c.client, fmt.Sprintf(FTBModpackBaseURL+"/%d", modpackID), FTBModpackUserAgent)
}

// GetModpackVersionManifest implements [ModpackClient.GetModpackVersionManifest].
func (c *FTBModpackClient) GetModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (ModpackVersionManifest, error) {
	m, err := c.GetFTBModpackVersionManifest(ctx, modpackID, versionID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetFTBModpackVersionManifest gets the manifest of an FTB modpack version with the given modpack ID and version ID.
func (c *FTBModpackClient) GetFTBModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (FTBModpackVersionManifest, error) {
	return doGetRequest[FTBModpackVersionManifest](ctx, c.client, fmt.Sprintf(FTBModpackBaseURL+"/%d/%d", modpackID, versionID), FTBModpackUserAgent)
}

var (
	// DefaultPublicModpackClient is the default public modpack client.
	DefaultPublicModpackClient = NewPublicModpackClient(http.DefaultClient)

	// DefaultCurseForgeModpackClient is the default CurseForge modpack client.
	DefaultCurseForgeModpackClient = NewCurseForgeModpackClient(http.DefaultClient)

	// DefaultFTBModpackClient is the default FTB modpack client.
	DefaultFTBModpackClient = NewFTBModpackClient(http.DefaultClient)
)

// GetPublicModpackManifest gets the manifest of a public modpack with the given ID.
func GetPublicModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return DefaultPublicModpackClient.GetModpackManifest(ctx, modpackID)
}

// GetPublicModpackVersionManifest gets the manifest of a public modpack version with the given modpack ID and version ID.
func GetPublicModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (PublicModpackVersionManifest, error) {
	return DefaultPublicModpackClient.GetPublicModpackVersionManifest(ctx, modpackID, versionID)
}

// GetCurseForgeModpackManifest gets the manifest of a CurseForge modpack with the given ID.
func GetCurseForgeModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return DefaultCurseForgeModpackClient.GetModpackManifest(ctx, modpackID)
}

// GetCurseForgeModpackVersionManifest gets the manifest of a CurseForge modpack version with the given modpack ID and version ID.
func GetCurseForgeModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (PublicModpackVersionManifest, error) {
	return DefaultCurseForgeModpackClient.GetCurseForgeModpackVersionManifest(ctx, modpackID, versionID)
}

// GetFTBModpackManifest gets the manifest of an FTB modpack with the given ID.
func GetFTBModpackManifest(ctx context.Context, modpackID int64) (ModpackManifest, error) {
	return DefaultFTBModpackClient.GetModpackManifest(ctx, modpackID)
}

// GetFTBModpackVersionManifest gets the manifest of an FTB modpack version with the given modpack ID and version ID.
func GetFTBModpackVersionManifest(ctx context.Context, modpackID, versionID int64) (FTBModpackVersionManifest, error) {
	return DefaultFTBModpackClient.GetFTBModpackVersionManifest(ctx, modpackID, versionID)
}

// doGetRequest sends a GET request to the given URL and returns the response unmarshaled from JSON.
func doGetRequest[V any](ctx context.Context, client *http.Client, url string, userAgent string) (v V, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return v, fmt.Errorf("failed to create request: %w", err)
	}

	if userAgent != "" {
		req.Header["User-Agent"] = []string{userAgent}
	}

	resp, err := client.Do(req)
	if err != nil {
		return v, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return v, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	const maxResponseBodySize = 1 << 30 // 1 GiB
	if resp.ContentLength > maxResponseBodySize {
		return v, fmt.Errorf("response body too large: %d bytes (max %d bytes)", resp.ContentLength, maxResponseBodySize)
	}
	var buf bytes.Buffer
	buf.Grow(max(0, int(resp.ContentLength)))
	r := io.LimitReader(resp.Body, maxResponseBodySize+1)
	n, err := buf.ReadFrom(r)
	if err != nil {
		return v, fmt.Errorf("failed to read response body: %w", err)
	}
	if n > maxResponseBodySize {
		return v, fmt.Errorf("response body too large: %d bytes (max %d bytes)", n, maxResponseBodySize)
	}

	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		return v, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return v, nil
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
	if m.Provider == "curseforge" {
		return m.Versions[0], true
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

// ModpackVersionManifest provides full information about a modpack version.
type ModpackVersionManifest interface {
	// ModpackVersionManifestInfo returns the information about the modpack version manifest.
	ModpackVersionManifestInfo() ModpackVersionManifestInfo

	// ModpackVersionFiles returns an iterator over the files in the modpack version manifest.
	ModpackVersionFiles() iter.Seq[ModpackVersionFile]
}

// ModpackVersionManifestInfo contains information about a modpack version manifest.
type ModpackVersionManifestInfo struct {
	// ModpackID is the ID of the modpack.
	ModpackID int64

	// VersionID is the ID of the version.
	VersionID int64

	// Name is the name of the version.
	Name string

	// Type is the type of the version.
	Type string

	// Updated is the time the version was last updated.
	Updated time.Time

	// FileCount is the number of files in the version.
	FileCount int

	// Targets is the list of targets for the version.
	Targets []ModpackVersionTarget
}

// ModpackVersionFile is a file in a modpack version's file list.
type ModpackVersionFile interface {
	// ModpackVersionFileInfo returns the information about the modpack version file.
	ModpackVersionFileInfo() ModpackVersionFileInfo

	// SendPrecheckJob creates a precheck job for the file and sends it to pjch.
	SendPrecheckJob(pjch chan<- precheck.Job, curseForgeAPIKey string, serverIgnoreCurseForgeProjects []int64) error
}

// ModpackVersionFileInfo contains information about a modpack version file.
type ModpackVersionFileInfo struct {
	// ID is the ID of the file.
	ID int64

	// Name is the name of the file.
	Name string

	// Type is the type of the file (e.g., "config", "mod", "resource").
	Type string

	// Updated is the time the file was last updated.
	Updated time.Time

	// Path is the path of the file.
	Path string

	// URL is the URL of the file.
	URL string

	// Mirrors is the list of mirrors for the file.
	Mirrors []string

	// Size is the size of the file in bytes.
	Size int64
}

// PublicModpackVersionManifest is the manifest of a public modpack version.
//
// This is the response of GET /public/modpack/{modpack_id}/{version_id}.
//
// PublicModpackVersionManifest implements [ModpackVersionManifest].
type PublicModpackVersionManifest struct {
	Files        []PublicModpackVersionFile `json:"files"`
	Specs        ModpackVersionSpecs        `json:"specs"`
	Targets      []ModpackVersionTarget     `json:"targets"`
	Installs     int64                      `json:"installs"`
	Plays        int64                      `json:"plays"`
	Refreshed    Time                       `json:"refreshed"`
	Changelog    string                     `json:"changelog"`
	Parent       int64                      `json:"parent"`
	Notification string                     `json:"notification"`

	// "links" array has no content.

	Status string `json:"status"`
	ResourceBase
	Private bool `json:"private"`
}

// ModpackVersionManifestInfo implements [ModpackVersionManifest.ModpackVersionManifestInfo].
func (m *PublicModpackVersionManifest) ModpackVersionManifestInfo() ModpackVersionManifestInfo {
	return ModpackVersionManifestInfo{
		ModpackID: m.Parent,
		VersionID: m.ID,
		Name:      m.Name,
		Type:      m.Type,
		Updated:   m.Updated.Time,
		FileCount: len(m.Files),
		Targets:   m.Targets,
	}
}

// ModpackVersionFiles implements [ModpackVersionManifest.ModpackVersionFiles].
func (m *PublicModpackVersionManifest) ModpackVersionFiles() iter.Seq[ModpackVersionFile] {
	return func(yield func(ModpackVersionFile) bool) {
		for i := range m.Files {
			if !yield(&m.Files[i]) {
				return
			}
		}
	}
}

// FTBModpackVersionManifest is the manifest of an FTB modpack version.
//
// FTBModpackVersionManifest implements [ModpackVersionManifest].
type FTBModpackVersionManifest struct {
	Files        []FTBModpackVersionFile `json:"files"`
	Specs        ModpackVersionSpecs     `json:"specs"`
	Targets      []ModpackVersionTarget  `json:"targets"`
	Installs     int64                   `json:"installs"`
	Plays        int64                   `json:"plays"`
	Refreshed    Time                    `json:"refreshed"`
	Changelog    string                  `json:"changelog"`
	Parent       int64                   `json:"parent"`
	Notification string                  `json:"notification"`

	// "links" array has no content.

	Status string `json:"status"`
	ResourceBase
	Private bool `json:"private"`
}

// ModpackVersionManifestInfo implements [ModpackVersionManifest.ModpackVersionManifestInfo].
func (m *FTBModpackVersionManifest) ModpackVersionManifestInfo() ModpackVersionManifestInfo {
	return ModpackVersionManifestInfo{
		ModpackID: m.Parent,
		VersionID: m.ID,
		Name:      m.Name,
		Type:      m.Type,
		Updated:   m.Updated.Time,
		FileCount: len(m.Files),
		Targets:   m.Targets,
	}
}

// ModpackVersionFiles implements [ModpackVersionManifest.ModpackVersionFiles].
func (m *FTBModpackVersionManifest) ModpackVersionFiles() iter.Seq[ModpackVersionFile] {
	return func(yield func(ModpackVersionFile) bool) {
		for i := range m.Files {
			if !yield(&m.Files[i]) {
				return
			}
		}
	}
}

// PublicModpackVersionFile is a file in a public modpack version's file list.
//
// PublicModpackVersionFile implements [ModpackVersionFile].
type PublicModpackVersionFile struct {
	// "version: int64" is in quotes for public modpacks, but not for CurseForge modpacks.

	Path    string   `json:"path"`
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors"`
	SHA1    HexBytes `json:"sha1"`
	Size    int64    `json:"size"`

	// "tags" array has no content.

	ClientOnly bool `json:"clientonly"`
	ServerOnly bool `json:"serveronly"`
	Optional   bool `json:"optional"`
	ResourceBase

	CurseForge CurseForgeFile `json:"curseforge,omitzero"`
}

// ModpackVersionFileInfo implements [ModpackVersionFile.ModpackVersionFileInfo].
func (f *PublicModpackVersionFile) ModpackVersionFileInfo() ModpackVersionFileInfo {
	return ModpackVersionFileInfo{
		ID:      f.ID,
		Name:    f.Name,
		Type:    f.Type,
		Updated: f.Updated.Time,
		Path:    f.Path,
		URL:     f.URL,
		Mirrors: f.Mirrors,
		Size:    f.Size,
	}
}

// SendPrecheckJob implements [ModpackVersionFile.SendPrecheckJob].
func (f *PublicModpackVersionFile) SendPrecheckJob(pjch chan<- precheck.Job, curseForgeAPIKey string, serverIgnoreCurseForgeProjects []int64) error {
	url := f.URL
	if url == "" {
		if f.CurseForge == (CurseForgeFile{}) {
			return ErrMissingURL
		}
		url = f.CurseForge.DownloadURL(f.Name)
	}

	path := filepath.Join(f.Path, f.Name)
	if path == "" {
		return ErrMissingPath
	}

	pjch <- precheck.Job{
		DownloadURL:      url,
		UserAgent:        APIUserAgent,
		CurseForgeAPIKey: curseForgeAPIKey,
		DestinationPath:  path,
		IsClientFile:     !f.ServerOnly,
		IsServerFile:     !f.ClientOnly && (f.CurseForge == (CurseForgeFile{}) || !slices.Contains(serverIgnoreCurseForgeProjects, f.CurseForge.Project)),
		NewHash:          sha1.New,
		Sum:              f.SHA1,
		Size:             f.Size,
	}

	return nil
}

// FTBModpackVersionFile is a file in an FTB modpack version's file list.
//
// FTBModpackVersionFile implements [ModpackVersionFile].
type FTBModpackVersionFile struct {
	Version int64    `json:"version,string"`
	Path    string   `json:"path"`
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors"`
	SHA1    HexBytes `json:"sha1"`
	Hashes  Hashes   `json:"hashes"`
	Size    int64    `json:"size"`

	// "tags" array has no content.

	ClientOnly bool `json:"clientonly"`
	ServerOnly bool `json:"serveronly"`
	Optional   bool `json:"optional"`
	ResourceBase

	CurseForge FTBCurseForgeFile `json:"curseforge,omitzero"`
}

// ModpackVersionFileInfo implements [ModpackVersionFile.ModpackVersionFileInfo].
func (f *FTBModpackVersionFile) ModpackVersionFileInfo() ModpackVersionFileInfo {
	return ModpackVersionFileInfo{
		ID:      f.ID,
		Name:    f.Name,
		Type:    f.Type,
		Updated: f.Updated.Time,
		Path:    f.Path,
		URL:     f.URL,
		Mirrors: f.Mirrors,
		Size:    f.Size,
	}
}

// SendPrecheckJob implements [ModpackVersionFile.SendPrecheckJob].
func (f *FTBModpackVersionFile) SendPrecheckJob(pjch chan<- precheck.Job, curseForgeAPIKey string, serverIgnoreCurseForgeProjects []int64) error {
	url := f.URL
	if url == "" {
		if f.CurseForge == (FTBCurseForgeFile{}) {
			return ErrMissingURL
		}
		url = f.CurseForge.DownloadURL(f.Name)
	}

	path := filepath.Join(f.Path, f.Name)
	if path == "" {
		return ErrMissingPath
	}

	// As of Go 1.27, crypto/sha256 and crypto/sha1 have SHA-NI acceleration on x86-64.
	// Throughput-wise, on an Intel Core Ultra 9 285K, crypto/sha256 is about 2x as fast as crypto/sha1,
	// and 4x as fast as crypto/sha512. So we prefer sha256 over sha1 over sha512.
	var (
		newHash func() hash.Hash
		sum     []byte
	)
	switch {
	case len(f.Hashes.SHA256) > 0:
		newHash = sha256.New
		sum = f.Hashes.SHA256
	case len(f.Hashes.SHA1) > 0:
		newHash = sha1.New
		sum = f.Hashes.SHA1
	case len(f.Hashes.SHA512) > 0:
		newHash = sha512.New
		sum = f.Hashes.SHA512
	default:
		newHash = sha1.New
		sum = f.SHA1
	}

	pjch <- precheck.Job{
		DownloadURL:      url,
		UserAgent:        FTBModpackUserAgent,
		CurseForgeAPIKey: curseForgeAPIKey,
		DestinationPath:  path,
		IsClientFile:     !f.ServerOnly,
		IsServerFile:     !f.ClientOnly && (f.CurseForge == (FTBCurseForgeFile{}) || !slices.Contains(serverIgnoreCurseForgeProjects, f.CurseForge.Project)),
		NewHash:          newHash,
		Sum:              sum,
		Size:             f.Size,
	}

	return nil
}

// Hashes is a set of hashes for a file.
type Hashes struct {
	SHA1     HexBytes `json:"sha1"`
	SHA256   HexBytes `json:"sha256"`
	SHA512   HexBytes `json:"sha512"`
	Murmur   uint64   `json:"murmur"`
	CfMurmur uint64   `json:"cfmurmur"`
}

// CurseForgeFile is a file under a CurseForge project.
type CurseForgeFile struct {
	Project int64 `json:"project"`
	File    int64 `json:"file"`
}

// DownloadURL returns the download URL of the file.
func (f *CurseForgeFile) DownloadURL(name string) string {
	return curseForgeDownloadURL(f.File, name)
}

// FTBCurseForgeFile is like [CurseForgeFile] but for FTB modpacks.
type FTBCurseForgeFile struct {
	Project int64 `json:"project,string"`
	File    int64 `json:"file,string"`
}

// DownloadURL returns the download URL of the file.
func (f *FTBCurseForgeFile) DownloadURL(name string) string {
	return curseForgeDownloadURL(f.File, name)
}

func curseForgeDownloadURL(fileID int64, fileName string) string {
	// If File is 1234567, the URL is https://edge.forgecdn.net/files/1234/567/fileName.
	return fmt.Sprintf("https://edge.forgecdn.net/files/%d/%d/%s", fileID/1000, fileID%1000, url.PathEscape(fileName))
}

// ResourceBase contains basic information about a remote resource.
type ResourceBase struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Updated Time   `json:"updated"`
}

// Time is [time.Time] but uses Unix timestamps in seconds for its representation in JSON.
type Time struct {
	time.Time
}

// MarshalJSON implements [json.Marshaler].
func (t Time) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, t.Time.Unix(), 10), nil
}

// UnmarshalJSON implements [json.Unmarshaler].
func (t *Time) UnmarshalJSON(data []byte) error {
	secs, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse Unix timestamp: %w", err)
	}
	t.Time = time.Unix(secs, 0)
	return nil
}

// HexBytes is a byte slice that uses hexadecimal encoding (base16) for its representation in JSON.
//
// Go 1.27: Use the new format:hex in encoding/json/v2.
type HexBytes []byte

// MarshalText implements [encoding.TextMarshaler].
func (h HexBytes) MarshalText() ([]byte, error) {
	dst := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(dst, h)
	return dst, nil
}

// UnmarshalText implements [encoding.TextUnmarshaler].
func (h *HexBytes) UnmarshalText(text []byte) error {
	dst, err := hex.AppendDecode((*h)[:0], text)
	if err != nil {
		return fmt.Errorf("failed to decode hex: %w", err)
	}
	*h = dst
	return nil
}
