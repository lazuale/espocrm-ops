package operation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewOperationID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])

	return fmt.Sprintf(
		"op-%s-%s",
		time.Now().UTC().Format("20060102T150405Z"),
		hex.EncodeToString(b[:]),
	)
}
