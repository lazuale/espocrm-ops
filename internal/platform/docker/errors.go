package docker

import (
	"fmt"
	"strings"
)

type CommandRunError struct {
	Name   string
	Args   []string
	Err    error
	Stderr string
}

func (e CommandRunError) Error() string {
	if stderr := strings.TrimSpace(e.Stderr); stderr != "" {
		return fmt.Sprintf("%s %v failed: %v: %s", e.Name, e.Args, e.Err, stderr)
	}

	return fmt.Sprintf("%s %v failed: %v", e.Name, e.Args, e.Err)
}

func (e CommandRunError) Unwrap() error {
	return e.Err
}

type UnavailableError struct {
	Err error
}

func (e UnavailableError) Error() string {
	return fmt.Sprintf("docker is not available: %v", e.Err)
}

func (e UnavailableError) Unwrap() error {
	return e.Err
}

type ContainerInspectError struct {
	Container string
	Err       error
}

func (e ContainerInspectError) Error() string {
	return fmt.Sprintf("inspect container %s: %v", e.Container, e.Err)
}

func (e ContainerInspectError) Unwrap() error {
	return e.Err
}

type ContainerNotRunningError struct {
	Container string
}

func (e ContainerNotRunningError) Error() string {
	return fmt.Sprintf("container %s is not running", e.Container)
}

type DBClientDetectionError struct {
	Container string
	Err       error
}

func (e DBClientDetectionError) Error() string {
	return fmt.Sprintf("detect db client in container %s: %v", e.Container, e.Err)
}

func (e DBClientDetectionError) Unwrap() error {
	return e.Err
}

type DBExecutionError struct {
	Action    string
	Container string
	Err       error
}

func (e DBExecutionError) Error() string {
	return fmt.Sprintf("%s in container %s: %v", e.Action, e.Container, e.Err)
}

func (e DBExecutionError) Unwrap() error {
	return e.Err
}
