package apperr

import "errors"

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

type Error struct {
	Kind    Kind
	Code    string
	Message string
	Cause   error
}

func (e Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

func (e Error) Unwrap() error {
	return e.Cause
}

func (e Error) ErrorKind() Kind {
	return e.Kind
}

func (e Error) ErrorCode() string {
	return e.Code
}

func Wrap(kind Kind, code string, err error) Error {
	message := string(kind)
	if err != nil {
		message = err.Error()
	}

	return Error{
		Kind:    kind,
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

type kinded interface {
	ErrorKind() Kind
}

type coded interface {
	ErrorCode() string
}

func KindOf(err error) (Kind, bool) {
	var k kinded
	if errors.As(err, &k) {
		return k.ErrorKind(), true
	}

	return "", false
}

func CodeOf(err error) (string, bool) {
	var c coded
	if errors.As(err, &c) {
		code := c.ErrorCode()
		if code != "" {
			return code, true
		}
	}

	return "", false
}
