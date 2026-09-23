package variant

import (
	"testing"

	"github.com/toku463ne/cld_generic_dangeon/engine"
)

func TestBaseIsDefault(t *testing.T) {
	cfg, err := Config(Base, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := engine.DefaultConfig()
	want.Seed = 7
	if cfg != want {
		t.Fatalf("base config %+v, want %+v", cfg, want)
	}
}

func TestUnknownVariant(t *testing.T) {
	if _, err := Config("nope", 1); err == nil {
		t.Fatal("unknown variant accepted")
	}
}
