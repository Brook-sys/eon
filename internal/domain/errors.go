package domain

import "errors"

// ErrUnauthorizedRetention is returned when a retention action is requested that the current policy forbids.
var ErrUnauthorizedRetention = errors.New("unauthorized retention action under current policy")
