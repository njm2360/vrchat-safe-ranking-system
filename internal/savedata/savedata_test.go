package savedata_test

import (
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func TestMarshalCanonical(t *testing.T) {
	got, err := savedata.Marshal(&savedata.Data{Score: 1234, GeneratedAt: time.Unix(9999, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"score":1234,"generated_at":"1970-01-01T02:46:39Z"}` {
		t.Errorf("Marshal = %q, want %q", string(got), `{"score":1234,"generated_at":"1970-01-01T02:46:39Z"}`)
	}
}

func TestRoundtrip(t *testing.T) {
	original := &savedata.Data{Score: 9999, GeneratedAt: time.Unix(12345, 0).UTC()}
	b, err := savedata.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := savedata.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Score != original.Score {
		t.Errorf("Score = %d, want %d", got.Score, original.Score)
	}
	if !got.GeneratedAt.Equal(original.GeneratedAt) {
		t.Errorf("GeneratedAt = %v, want %v", got.GeneratedAt, original.GeneratedAt)
	}
}

func TestUnmarshalRejectsGarbage(t *testing.T) {
	if _, err := savedata.Unmarshal([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
