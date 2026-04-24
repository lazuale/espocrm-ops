//go:build !unix

package ops

import "fmt"

func acquireOperationFileLock(request operationLockRequest) (operationFileLock, error) {
	return nil, fmt.Errorf("operation locks are unsupported on this platform for %s", request.Path)
}
