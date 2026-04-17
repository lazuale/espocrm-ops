package config

import "fmt"

type PasswordSourceConflictError struct {
	Label string
}

func (e PasswordSourceConflictError) Error() string {
	return fmt.Sprintf("use either %s or %s-file, not both", e.Label, e.Label)
}

func (e PasswordSourceConflictError) ErrorCode() string {
	return "preflight_failed"
}

type PasswordFileReadError struct {
	Path string
	Err  error
}

func (e PasswordFileReadError) Error() string {
	return fmt.Sprintf("read password file %s: %v", e.Path, e.Err)
}

func (e PasswordFileReadError) Unwrap() error {
	return e.Err
}

func (e PasswordFileReadError) ErrorCode() string {
	return "filesystem_error"
}

type PasswordFileEmptyError struct {
	Path string
}

func (e PasswordFileEmptyError) Error() string {
	return fmt.Sprintf("password file is empty: %s", e.Path)
}

func (e PasswordFileEmptyError) ErrorCode() string {
	return "preflight_failed"
}

type PasswordRequiredError struct {
	Label string
}

func (e PasswordRequiredError) Error() string {
	return fmt.Sprintf("%s is required", e.Label)
}

func (e PasswordRequiredError) ErrorCode() string {
	return "preflight_failed"
}
