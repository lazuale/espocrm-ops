package failure

type Kind string

const (
	KindValidation Kind = "validation"
	KindManifest   Kind = "manifest"
	KindConflict   Kind = "conflict"
	KindNotFound   Kind = "not_found"
	KindIO         Kind = "io"
	KindExternal   Kind = "external"
	KindCorrupted  Kind = "corrupted"
	KindRestore    Kind = "restore"
	KindInternal   Kind = "internal"
)

type Failure struct {
	Kind    Kind
	Code    string
	Summary string
	Action  string
	Err     error
}

func (f Failure) Error() string {
	if f.Err == nil {
		return ""
	}

	return f.Err.Error()
}

func (f Failure) Unwrap() error {
	return f.Err
}
