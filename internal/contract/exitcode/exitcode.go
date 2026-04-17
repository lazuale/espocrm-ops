package exitcode

const (
	OK = 0

	UsageError      = 2
	ManifestError   = 3
	ValidationError = 4
	RestoreError    = 5
	FilesystemError = 6
	InternalError   = 10
)
