/*
Command golf provides some Go one-liner fun.

Invoke it with a snippet of Go code in the -e flag, which will be compiled
and run for you.

Additional flags such as -n turn on awk/perl-like line mode, which are useful
in processing text data. See the examples and flags sections below.

Some variables and functions are provided in the prelude package. These are
inlined and made available to the one-liner. They are common elements of
one-liner coding, for example the current line being processed in line mode.

Examples

Try these on your command line.

  # Put your oneliner in -e. Here we use the builtin println.
  golf -e 'println(9*6)'

  # Explicitly import additional packages (don't have to be from the stdlib)
  # with the -M flag, which may be repeated.
  golf -M fmt,math -e 'fmt.Println(math.Pi)'

  # Some standard library packages are imported automatically.
  # goimports runs for you in any case, so -M is often not needed.
  golf -e 'fmt.Fprint(os.Stderr, "hi\n")'
  golf -le 'Print("The time is ", time.Now())'

  # cat -n (see more about "line mode" below)
  golf -n -e 'fmt.Printf("%6d  %s", LineNum, Line)' MYFILE

  # Use builtin Die function (takes raw error or fmtstring+args)
  golf -l -e 'if data, err := os.ReadFile("MYFILE"); err != nil { Die(err) }; Print(len(data))'

  # head MYFILE
  golf -p -e 'if LineNum == 10 {break File}' MYFILE

  # -a mode (which implies -n) automatically splits input fields.
  # These can be accessed from the Fields slice, or using
  # the convenient Field accessor (supports 1-based and negative indexes).
  ps aux | golf -ale 'Print(Field(5))'

  # Prints "and".
  echo "tom, dick, and harry" | golf -ape 'Line = Field(3)'

  # Input field separation uses strings.Fields by default.
  # Supply the -F flag to override (-F implicitly means -a and -n).
  # Can also be a regexp; see docs for prelude.GSplit.

  # All users on the system.
  golf -F : -e 'Print(Field(1))' /etc/passwd

  # Convert TSV to CSV.
  golf -F '/\t/' -ple 'for i, v := range Fields { Fields[i] = strconv.Quote(v) }; Line = Join(Fields, ",")'

  # sum sizes. Note -b and E replace awk/perl BEGIN and END blocks.
  ls -l | golf -alb 'sum := 0' -e 'sum += GAtoi(Field(5))' -E 'Print(sum)'s

Flags

golf mimics perl's flags, but not perfectly so.

You can cluster one-letter flags, so -lane means the same as
-l -a -n -e as it does in perl.

The -b and -E flags act as a replacement for awk and Perl's BEGIN and END blocks.
They are inserted before and after the -e snippet and only run once each.

Line mode

-n puts golf in line mode: each command-line argument is treated as a filename,
which is opened in succession. Its name will populate the Filename variable.
Lines are then scanned, populating the Line variable. Stdin is read instead of
a named file if no filenames were provided.

These do the same thing as the cat example above:

  cat MYFILE | golf -n -e 'fmt.Printf("%6d  %s", LineNum, Line)'
  golf -n -e 'fmt.Printf("%6d  %s", LineNum, Line)' < MYFILE

  # Unix cat concatenates multiple files. This does, too.
  golf -ne 'Print(Line)' FILE1 FILE2 FILE2

The File and Line labels can be continued/broken from to skip inputs.

-p implies -n and adds a "Print(Line)" call after each line. So you can
even say:

  golf -pe '' FILE1 FILE2 FILE3

*/
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gaal/golf/prelude"
)

var (
	rawSrc   = flag.String("e", "", "one-liner code")
	flgN     = flag.Bool("n", false, "line mode")
	flgL     = flag.Bool("l", false, "automate line-end processing. Trim input newline and add it back on -p")
	flgP     = flag.Bool("p", false, "pipe mode. Implies -n and prints Line after each iteration")
	flgG     = flag.Bool("g", false, "skip goimports")
	flgA     = flag.Bool("a", false, "autosplit Line to Fields. Implies -n")
	flgF     = flag.String("F", " ", "field separator. Implies -a and -n. See docs for GSplit.")
	flgKeep  = flag.Bool("k", false, "keep tempdir, for debugging")
	beginSrc = flag.String("b", "", "code block to insert before record processing")
	endSrc   = flag.String("E", "", "code block to insert after record processing")
	warnings = flag.Bool("w", false, "print warnings on access to undefined fields and so on")
	modules  []string
)

func init() {
	flag.Func("M", "modules to import. May be repeated, or separate with commas", func(s string) error {
		modules = append(modules, strings.Split(s, ",")...)
		return nil
	})
}

var errGolf = fmt.Errorf("golf returned nonzero status")

type Prog struct {
	RawArgs   []string
	BeginSrc  string
	RawSrc    string
	EndSrc    string
	Src       string
	Imports   []string
	FlgN      bool
	FlgP      bool
	FlgL      bool
	FlgA      bool
	FlgF      string
	Warnings  bool
	Goimports bool
	Keep      bool
	Prelude   []byte
}

//go:embed prelude/prelude.go
var golflibsrc []byte

func preludeSrc() []byte {
	start, end := bytes.Index(golflibsrc, []byte("// golf:prelude start\n")), bytes.Index(golflibsrc, []byte("// golf:prelude end\n"))
	if start < 0 {
		prelude.Die("prelude.go missing start marker")
	}
	if end < 0 {
		return golflibsrc[start:]
	}
	return golflibsrc[start:end]
}

var program = template.Must(template.New("program").Parse(`
package main

import (
{{- range .Imports}}
	"{{.}}"
{{- end}}
)

{{ printf "%s" .Prelude}}

func init() {
	IFS = {{ printf "%q" .FlgF }}
	Warnings = {{ .Warnings }}
	{{ if .FlgL }}
	Print = fmt.Println
	{{- end }}
}

func main() {
	// User -b start
	{{.BeginSrc}}
	// User -b end
	{{- if .FlgN}}
	const _golf_p = {{.FlgP}}
	var _golf_p_dirty = false
	_golf_flush_p := func() {
		if _golf_p_dirty {
			Print(Line)
			_golf_p_dirty = false
		}
	}
	_golf_filenames := os.Args[1:]
	if len(_golf_filenames)==0 {
		_golf_filenames=[]string{"/dev/stdin"}
	}
File:
    for _, Filename = range _golf_filenames {
		_golf_flush_p()
		_golf_file, err := os.Open(Filename)
		if err != nil {
			Die(err)
		}
		LineNum = 0
		_golf_scanner := bufio.NewScanner(_golf_file)
	Line:
		for _golf_scanner.Scan() {
			_golf_flush_p()
			LineNum++  // 1-based. Be compatible with awk, perl's default.
			// Scanned line.
			// BUG: restores newlines crudely in non-line mode.
			// Should have \r when they were present in input, and should not
			// insert a trailing newline on the last line if it was absent.
			Line = _golf_scanner.Text() {{- if not .FlgL}} + "\n"{{end}}
			_golf_p_dirty = {{ .FlgP }}
			{{if .FlgA}}
			Fields = GSplit(IFS, Line)
			{{- end}}
			{{- end}}
			// User -e start
			{{.RawSrc}}
			// User -e end
			{{- if .FlgN}}
			continue Line
		}
		if err := _golf_scanner.Err(); err != nil {
			Die("%s: %v", Filename, err)
		}
		continue File
	}
	_golf_flush_p()
	{{- end}}
	// User -E start
	{{.EndSrc}}
	// User -E end
}
`))

func (p *Prog) transform() error {
	s := &bytes.Buffer{}
	if err := program.Execute(s, p); err != nil {
		return err
	}
	// Try to pretty it up, but stay silent about errors. The real compiler will give a better error message later.
	if src, err := format.Source(s.Bytes()); err != nil {
		p.Src = s.String()
	} else {
		p.Src = string(src)
	}

	return nil
}

func do(c string, args []string) error {
	cmd := exec.Command(c, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if cmd.ProcessState.ExitCode() != 0 {
		return errGolf
	}
	return nil
}

// doQ runs the command, but elides the output if it was successful.
func doQ(c string, args []string) error {
	cmd := exec.Command(c, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if cmd.ProcessState.ExitCode() != 0 {
		return fmt.Errorf("%s", string(out))
	}
	return nil
}

func (p *Prog) run() int {
	tmpdir, err := os.MkdirTemp("", "golf-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir tmp: %v\n", err)
		return 1
	}
	if p.Keep {
		fmt.Fprintln(os.Stderr, tmpdir)
	} else {
		defer func() {
			if err := os.RemoveAll(tmpdir); err != nil {
				fmt.Fprintf(os.Stderr, "rmall tmp: %v\n", err)
				// but don't fail the golf.
			}
		}()
	}

	if err := os.Chdir(tmpdir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	tmpfile := filepath.Join(tmpdir, "golfe.go")
	if err := os.WriteFile(tmpfile, []byte(p.Src), 0666); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if p.Goimports {
		if err := doQ("goimports", []string{"-w", "."}); err != nil {
			fmt.Fprintf(os.Stderr, "golf: goimports: %v\n", err)
			return 1
		}
	}

	if err := doQ("go", []string{"mod", "init", "example.com/golf"}); err != nil {
		fmt.Fprintf(os.Stderr, "golf: mod init: %v\n", err)
		return 1
	}
	if err := doQ("go", []string{"mod", "tidy"}); err != nil {
		fmt.Fprintf(os.Stderr, "golf: mod tidy: %v\n", err)
		return 1
	}
	if err := do("go", append([]string{"run", "."}, p.RawArgs...)); err != nil {
		// if err != errGolf {			fmt.Fprintf(os.Stderr, "golf: %v\n", err)		}
		return 1
	}

	return 0
}

func decluster() {
	res := []string{os.Args[0]}
	for i, v := range os.Args[1:] {
		if v[0] != '-' {
			// Skip a non-flag argument.
			// If we add long flags we'll need to match and skip them here too.
			res = append(res, v)
			continue
		}
		if v == "--" {
			res = append(res, os.Args[i:]...)
			break
		}
		for _, vv := range strings.Split(v[1:], "") {
			res = append(res, "-"+vv)
		}
	}
	os.Args = res
}

func main() {
	// The standard Go flag package does not support flag clustering.
	// This is too convenient to give up when golfing, so handle it ourselves.
	decluster()
	flag.Parse()

	// -F implies -a (which in turn implies -n...)
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "F" {
			*flgA = true
		}
	})

	p := &Prog{
		BeginSrc:  *beginSrc,
		RawSrc:    *rawSrc,
		EndSrc:    *endSrc,
		RawArgs:   flag.Args(),
		Imports:   []string{"bufio", "os", "regexp", "strings", "fmt"},
		FlgN:      *flgN || *flgP || *flgA,
		FlgP:      *flgP,
		FlgL:      *flgL,
		FlgA:      *flgA,
		FlgF:      *flgF,
		Warnings:  *warnings,
		Goimports: !*flgG,
		Keep:      *flgKeep,
		Prelude:   preludeSrc(),
	}
	if err := p.transform(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(p.run())
}
