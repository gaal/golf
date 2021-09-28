// Package prelude provides convenient functionality for Go one-liners.
//
// Most of its source code is embedded directly in the generated source as a
// Prelude.
// It is however also used for godoc, and can be used by the golf command.
package prelude

import (
	"bytes"
	// Required for go:embed.
	_ "embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Code between these comments is embedded in the golf binary.
// Why do we inline the Prelude source inside the generated golf program
// instead of "import ."-ing it? Because this way, we don't have to worry
// about where the prelude package is installed, modules, and so on.
// The effect on program size or build times is negligible either way.

// golf:prelude start

var (
	// Updated automatically in -n mode.

	// Filename is the current filename.
	Filename string
	// LineNum is the current line number, 1-based.
	LineNum int
	// Line is the current line. It may be edited by the script.
	// Its contents are automatically printed in -p mode.
	Line string

	// Fields is the Split field slice. See the convenience Field accessor.
	// Updated automatically in -a mode.
	Fields []string

	// IFS is the input field separator used in -a mode. Overridden by -F.
	IFS = " "
	// OFS is the output field separator used by Field(0).
	OFS = " "
	// Warnings controls whether to print warnings. Overridden by -w.
	Warnings = false
	// GolfFlgL controls whether to strip/add newlines on I/O. Overridden by -l.
	GolfFlgL = false

	// -i settings. Note that -i without argument is allowed, it means no backup.

	// GolfInPlace reports whether we are in-place edit mode.
	// It is set when -i or -I were given on the command-line.
	GolfInPlace = false
	// GolfInPlaceBak is the file pattern for in-place edit backups.
	GolfInPlaceBak string

	// CurOut is the default writer for Print and Printf.
	// Overridden to each Filename in -i.
	CurOut io.WriteCloser = os.Stdout
)

var (
	// Join is an alias for strings.Join.
	Join = strings.Join

	// RE is an alias for regexp.MustCompile.
	RE = regexp.MustCompile
)

// Print prints a string to CurOut.
//
// With no arguments, the string to be printed defaults to Line.
//
// In -l mode, a newline is appended to the string.
//
// In -i mode, the "current output" is the replacement for the current
// Filename. Otherwise, it is os.Stdout.
func Print(xs ...interface{}) {
	if len(xs) == 0 {
		xs = append(xs, Line)
	}
	if GolfFlgL {
		fmt.Fprintln(CurOut, xs...)
	} else {
		fmt.Fprint(CurOut, xs...)
	}
}

// Printf prints a string to CurOut.
//
// In -i mode, the "current output" is the replacement for the current
// Filename. Otherwise, it is os.Stdout.
//
// Newline is not added automatically, even in -l mode, nor is Line used
// as a default string.
func Printf(format string, xs ...interface{}) {
	fmt.Fprintf(CurOut, format, xs...)
}

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
// If multiple arguments are provided and the first one is a string,
// it is taken as a format string.
// Otherwise everything is printed using default fmt formatting.
func Warn(xs ...interface{}) {
	if len(xs) == 0 {
		xs = []interface{}{"Something went wrong while golfing\n"}
	}
	if s, ok := xs[0].(string); ok && len(xs) > 1 {
		if !strings.HasSuffix(s, "\n") {
			s = s + "\n"
		}
		fmt.Fprintf(os.Stderr, s, xs[1:]...)
		return
	}
	fmt.Fprintln(os.Stderr, xs...)
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
			Die("Invalid GSplit regexp separator (check -F flag): %v", err)
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

// BackupName returns the filename used as a backup in in-place edit mode.
//
// Replacement rules follow Perl -i:
//   - if ext contains no '*' characters, it is appended to orig as a suffix.
//   - otherwise, each * is replaced with orig.
func BackupName(orig, ext string) string {
	if strings.Contains(ext, "*") {
		return strings.ReplaceAll(ext, "*", orig)
	}
	return orig + ext
}

// golf:prelude end

//go:embed prelude.go
var golflibsrc []byte

// Source returns the source code of the prelude.
func Source() []byte {
	var (
		start = bytes.Index(golflibsrc, []byte("// golf:prelude start\n"))
		end   = bytes.Index(golflibsrc, []byte("// golf:prelude end\n"))
	)
	if start < 0 {
		Die("prelude.go missing start marker")
	}
	if end < 0 {
		return golflibsrc[start:]
	}
	return golflibsrc[start:end]
}
