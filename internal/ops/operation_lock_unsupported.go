//go:build !unix

package ops

import "fmt"

func acquireProjectFileLock(path string) (projectFileLock, error) {
	return nil, fmt.Errorf("project locks are unsupported on this platform for %s", path)
}
