package structconfig

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestBuildMergedFlagReadError(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("bad-map", "", "")
	flags.Set("bad-map", "val")

	s := &StructConfig{
		flags: flags,
		infos: []varInfo{
			{
				Name: "BadMap",
				Key:  "badmap",
				Flag: "bad-map",
				typ:  reflect.TypeFor[map[string]bool](),
			},
		},
	}

	_, err := s.buildMerged()
	if err == nil {
		t.Fatal("expected error from buildMerged")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "--bad-map") {
		t.Errorf("expected flag name in error, got: %v", err)
	}
	if !strings.Contains(errStr, `"BadMap"`) {
		t.Errorf("expected field name in error, got: %v", err)
	}
	if !strings.Contains(errStr, `"badmap"`) {
		t.Errorf("expected key in error, got: %v", err)
	}
}

func TestReadFlagValueUnsupportedMapElementType(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	info := varInfo{
		Flag: "bad-map",
		typ:  reflect.TypeFor[map[string]bool](),
	}

	_, err := readFlagValue(flags, info)
	if err == nil {
		t.Fatal("expected error for unsupported map element type")
	}

	if !strings.Contains(err.Error(), "unsupported map element type") {
		t.Fatalf("unexpected error: %v", err)
	}
}
