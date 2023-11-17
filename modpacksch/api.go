// Package modpacksch implements an API client for downloading modpacks from https://api.modpacks.ch/.
//
// API documentation: https://modpacksch.docs.apiary.io/
package modpacksch

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
	Refreshed    int64            `json:"refreshed"`
	Notification string           `json:"notification"`
	Rating       ModpackRating    `json:"rating"`
	Status       string           `json:"status"`
	Released     int64            `json:"released"`
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
	return m.Versions[0], true
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
	Updated    int64    `json:"updated"`
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
	Refreshed    int64                  `json:"refreshed"`
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

// FileBase contains basic information about a remote file.
type FileBase struct {
	URL     string   `json:"url"`
	Mirrors []string `json:"mirrors"`
	SHA1    string   `json:"sha1"`
	Size    int64    `json:"size"`
	ID      int64    `json:"id"`
	Type    string   `json:"type"`
	Updated int64    `json:"updated"`
}

// ResourceBase contains basic information about a remote resource.
type ResourceBase struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Updated int64  `json:"updated"`
}
