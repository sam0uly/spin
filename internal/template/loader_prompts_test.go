package template

import (
	"errors"
	"testing"
)

// TestDefaultInvalidPinnedPrompt_AlwaysKeeps pins the contract: the fallback for "this clone looks bad, what do.
func TestDefaultInvalidPinnedPrompt_AlwaysKeeps(t *testing.T) {
	keep, err := defaultInvalidPinnedPrompt("any-name", "/any/path", errors.New("any error"))
	if err != nil {
		t.Fatalf("default fallback must not return err; got %v", err)
	}
	if !keep {
		t.Errorf("default fallback must keep the clone; got keep=%v", keep)
	}
}

// TestDefaultExistingDestPrompt_ReturnsWipe pins the contract: the fallback for "dest already exists, what do you.
func TestDefaultExistingDestPrompt_ReturnsWipe(t *testing.T) {
	got, err := defaultExistingDestPrompt("any-name", "/any/path")
	if err != nil {
		t.Fatalf("default fallback must not return err; got %v", err)
	}
	if got != DestWipe {
		t.Errorf("default fallback must wipe; got %v", got)
	}
}
