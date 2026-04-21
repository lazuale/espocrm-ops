package config

import (
	"fmt"
	"strings"
)

type MissingEnvFileError struct {
	Path string
}

func (e MissingEnvFileError) Error() string {
	return fmt.Sprintf("missing env file %s", e.Path)
}

type UnsupportedContourError struct {
	Contour string
}

func (e UnsupportedContourError) Error() string {
	return fmt.Sprintf("unsupported contour %q. use dev or prod", e.Contour)
}

type InvalidEnvFileError struct {
	Path    string
	Message string
}

func (e InvalidEnvFileError) Error() string {
	if strings.TrimSpace(e.Path) == "" {
		return e.Message
	}
	return fmt.Sprintf("invalid env file %s: %s", e.Path, e.Message)
}

type EnvParseError struct {
	Path    string
	Line    int
	Message string
}

func (e EnvParseError) Error() string {
	return fmt.Sprintf("env file %q contains unsupported syntax on line %d: %s", e.Path, e.Line, e.Message)
}

type MissingEnvValueError struct {
	Path string
	Name string
}

func (e MissingEnvValueError) Error() string {
	return fmt.Sprintf("%s is not set in %s", e.Name, e.Path)
}
