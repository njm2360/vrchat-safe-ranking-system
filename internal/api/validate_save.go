package api

import (
	"errors"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

const (
	generatedAtMaxPast   = 7 * 24 * time.Hour
	generatedAtMaxFuture = 5 * time.Minute
)

var errInvalidSaveData = errors.New("invalid save data")

func validateSaveData(data *savedata.Data, now time.Time) error {
	if data.GeneratedAt.After(now.Add(generatedAtMaxFuture)) ||
		data.GeneratedAt.Before(now.Add(-generatedAtMaxPast)) {
		return errInvalidSaveData
	}
	return nil
}
