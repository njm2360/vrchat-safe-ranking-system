package savedata

import (
	"encoding/json"
	"time"
)

type Data struct {
	Score       int64     `json:"score"`
	GeneratedAt time.Time `json:"generated_at"`
}

func Marshal(d *Data) ([]byte, error) {
	return json.Marshal(d)
}

func Unmarshal(b []byte) (*Data, error) {
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
