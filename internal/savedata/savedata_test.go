package savedata_test

import (
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func TestMarshalCanonical(t *testing.T) {
	got, err := savedata.Marshal(&savedata.Data{Score: 1234})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"score":1234}` {
		t.Errorf("Marshal = %q, want %q", string(got), `{"score":1234}`)
	}
}

func TestRoundtrip(t *testing.T) {
	original := &savedata.Data{Score: 9999}
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
}

func TestUnmarshalRejectsGarbage(t *testing.T) {
	if _, err := savedata.Unmarshal([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
