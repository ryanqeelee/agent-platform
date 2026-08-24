package policy

import (
	"errors"
)

var ErrSharingUnavailable = errors.New("organization and cross-workspace sharing are unavailable")

func SharingAvailable() bool { return false }
