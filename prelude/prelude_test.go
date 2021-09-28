package prelude

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestField(t *testing.T) {
	for _, d := range []struct {
		in   []string
		i    int
		want string
	}{
		{[]string{"a", "b", "c"}, 0, "a b c"},
		{[]string{"a", "b", "c"}, 1, "a"},
		{[]string{"a", "b", "c"}, -1, "c"},
		{[]string{"a", "b", "c"}, 10, ""},
		{[]string{"a", "b", "c"}, -10, ""},
		{nil, 0, ""},
		{nil, 1, ""},
		{nil, -1, ""},
		{nil, 2, ""},
		{nil, -2, ""},
		{[]string{""}, 0, ""},
		{[]string{""}, 1, ""},
		{[]string{""}, -1, ""},
		{[]string{""}, 2, ""},
		{[]string{""}, -2, ""},
	} {
		Fields = d.in
		have := Field(d.i)
		if diff := cmp.Diff(have, d.want); diff != "" {
			t.Errorf("Line = %q, Field(%d) diff:\n%s", d.in, d.i, diff)
		}
	}
}
