package util

import (
	"errors"
)

var (
	ErrClusterNotReady         = errors.New("source cluster not ready yet")
	ErrGenerateSourceInterface = errors.New("failed to generate source interface")
)
