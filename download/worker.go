package download

import "os"

// Job is a download job.
type Job struct {
	// DownloadURL is the target file's download URL.
	DownloadURL string

	// TargetFile is the target file.
	TargetFile *os.File

	// SecondaryTargetFile is the secondary target file.
	// Nil means no secondary target file.
	SecondaryTargetFile *os.File
}
