package operationtrace

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const TimeFormat = time.RFC3339

type DefaultRuntime struct{}

func (DefaultRuntime) Now() time.Time {
	return time.Now().UTC()
}

func (DefaultRuntime) NewOperationID() string {
	return NewOperationID()
}

func NewOperationID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])

	return fmt.Sprintf(
		"op-%s-%s",
		time.Now().UTC().Format("20060102T150405Z"),
		hex.EncodeToString(b[:]),
	)
}
