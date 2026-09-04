package api

import (
	"errors"

	"github.com/stashapp/stash/pkg/logger"
)

var errPersonalDataUnavailable = errors.New("personal data is temporarily unavailable")

func personalDataError(operation string, err error) error {
	logger.Errorf("personal data operation %s failed: %v", operation, err)
	return errPersonalDataUnavailable
}
