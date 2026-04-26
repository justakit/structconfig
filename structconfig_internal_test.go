package structconfig

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

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
