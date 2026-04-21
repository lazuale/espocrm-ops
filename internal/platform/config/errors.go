package config

import "fmt"

type PasswordSourceConflictError struct {
	Label string
}

func (e PasswordSourceConflictError) Error() string {
	return fmt.Sprintf("use either %s or %s-file, not both", e.Label, e.Label)
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

type PasswordFileEmptyError struct {
	Path string
}

func (e PasswordFileEmptyError) Error() string {
	return fmt.Sprintf("password file is empty: %s", e.Path)
}

type PasswordRequiredError struct {
	Label string
}

func (e PasswordRequiredError) Error() string {
	return fmt.Sprintf("%s is required", e.Label)
}
