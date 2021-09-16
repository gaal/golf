// Package prelude provides convenient functionality for Go one-liners.
//
// Most of its source code is embedded directly in the generated source as a Prelude.
// It is however also used for godoc, and can be used by the golf command.
package prelude

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Code between these comments is embedded in the golf binary.
// golf:prelude start

var (
	Filename string   // Current filename
	LineNum  int      // Current line number (1-based)
	Line     string   // Current line
	Fields   []string // Split field slice. See the convenience Field accessor.

	IFS      = " "   // Input field separator. Overridden by -F.
	OFS      = " "   // Output field separator
	Warnings = false // Whether to print warnings. Overridden by -w.
)

var (
	// Join is an alias for strings.Join.
	Join = strings.Join

	// RE is an alias for regexp.MustCompile.
	RE = regexp.MustCompile

	// Print forwards fmt.Print (or fmt.Println, when -l is specified).
	//
	// Unlike Perl's print builtin, it does not default to printing
	// the implicit Line variable when no arguments are provided.
	Print = fmt.Print
)

// GAtoi calls strconv.Atoi on s, and issues an optional warning
// if that returned an error.
func GAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil && Warnings {
		Warn(err)
	}
	return i // defaults to 0 on parse fail
}

// Die prints an error to stderr and exits the program with a failure status.
//
// Arguments follow the semantics of Warn.
func Die(xs ...interface{}) {
	Warn(xs...)
	os.Exit(1)
}

// Warn prints an error to stderr.
//
// If no arguments are supplied, a generic message is printed.
// If the first argument is a string, it is taken as a format string.
// Otherwise everything is printed using default fmt formatting.
func Warn(xs ...interface{}) {
	if len(xs) == 0 {
		xs = []interface{}{"Something went wrong while golfing\n"}
	}
	switch x := xs[0].(type) {
	case string:
		if !strings.HasSuffix(x, "\n") {
			x = x + "\n"
		}
		fmt.Fprintf(os.Stderr, x, xs[1:]...)
	default:
		fmt.Fprintln(os.Stderr, xs...)
	}
}

// GSplit splits an input string, with some golf affordances.
// It is a bit more similar to Perl's split than it is to strings.Split.
//
// When sep is a single space, strings.Fields is used.
// This is the default behavior with -a and no -F override,
// and operates how awk/perl do by default:
// The input is trimmed of leading and trailing whitespace, then
// any amount of (any) whitespace is taken as a field separator.
//
// When sep has the form /pat/, pat is compiled into a regexp and
// regexp.Split is used.
//
// Otherwise, sep is taken as a literal for strings.Split.
func GSplit(sep, input string) []string {
	if sep == " " {
		return strings.Fields(input)
	}
	if len(sep) > 1 && sep[0] == '/' && sep[len(sep)-1] == '/' {
		// Sure, we could memoize regexp compilation.
		if re, err := regexp.Compile(sep[1 : len(sep)-1]); err != nil {
			Die("Invalid Split regexp separator (check -F flag): %v", err)
		} else {
			return re.Split(input, -1)
		}
	}
	return strings.Split(input, sep)
}

// Field retrieves a split field.
// Index 0 returns the entire line re-joined using the OFS.
// Positive values are taken to be a 1-based index to Fields.
// Negative values index from the end (so -1 is the last Fields element).
// Indexes out of range silently return the empty string.
func Field(n int) string {
	switch {
	case n == 0:
		return strings.Join(Fields, OFS)
	case n < 0:
		n = len(Fields) + n
	case n > 0:
		n--
	}
	if n < 0 || n > len(Fields)-1 {
		if Warnings {
			Warn("undefined field: %d: %v", n, Fields)
		}
		return ""
	}
	return Fields[n]
}

// golf:prelude end
