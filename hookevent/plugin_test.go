package hookevent

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"
)

const pluginHooks = "../hooks/hooks.json"

func registeredEvents(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(pluginHooks)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(file.Hooks))
}

func TestPluginRegistersEveryHandledEvent(t *testing.T) {
	want := append(slices.Collect(maps.Keys(eventCues)), messageDisplay)
	slices.Sort(want)
	if got := registeredEvents(t); !slices.Equal(got, want) {
		t.Errorf("registered events = %v, want %v", got, want)
	}
}
