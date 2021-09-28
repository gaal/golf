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
  golf -M fmt -M math -e 'fmt.Println(math.Pi)'

  # Some standard library packages are imported automatically.
  # goimports can run for you with -g, so -M is often not needed.
  golf -e 'fmt.Fprint(os.Stderr, "hi\n")'
  golf -gle 'Print("The time is ", time.Now())'

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

  # Prints "and". Could also say "Field(-2)".
  echo "tom, dick, and harry" | golf -ape 'Line = Field(3)'

  # Input field separation uses strings.Fields by default.
  # Supply the -F flag to override (-F implicitly means -a and -n).
  # Can also be a regexp; see docs for prelude.GSplit.

  # All users on the system.
  golf -F : -e 'Print(Field(1))' /etc/passwd

  # Convert TSV to CSV.
  golf -F '/\t/' -ple 'for i, v := range Fields { Fields[i] = strconv.Quote(v) }; Line = Join(Fields, ",")'

  # sum sizes. Note -b and E replace awk/perl BEGIN and END blocks.
  ls -l | golf -alb 'sum := 0' -e 'sum += GAtoi(Field(5))' -E 'Print(sum)'

Flags

golf mimics perl's flags, but not perfectly so.

You can cluster one-letter flags, so -lane means the same as
-l -a -n -e as it does in perl.

The -b and -E flags act as replacements for awk and Perl's BEGIN and END blocks.
They are inserted before and after the -e snippet and only run once each. They
are inserted in the same scope as the -e script, so variables declared in BEGIN
are available for later blocks. -BEGIN and -END are aliases for -b and -E
respectively.

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

In-place mode

-i causes edits to happen in-place: each input file is opened, unlinked, and
the default output (Print, Printf) is sent to a new file with the original's
name.

-I does the same, but keeps a backup of the original, according to the same
renaming rules that perl -i uses:

  - if the replacement contains no "*", it is used as a literal suffix.
  - otherwise, each occurrence of * is replaced with the original filename.

Like perl, we do not support crossing filesystem boundaries in backups, nor
do we create directories.

Unlike perl, in-place backup uses the -I flag, not the -i flag with an argument.
Go's standard flag library does not support optional flags. So these don't act
the same:

  perl -ib FILE1 FILE2  # Runs the perl program in FILE1 with backup to FILE2.
  golf -ib WORD FILE    # Runs WORD in BEGIN stage, FILE will end up truncated.

No script mode

golf does not support a script mode (e.g., "golf FILE", or files with #!golf).

If you are writing a Go program in an editor, just go run it. If looking for
convenience, see if package Prelude contains anything useful.
*/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/gaal/golf/prelude"
)

var (
	rawSrc     = flag.String("e", "", "one-liner code")
	flgN       = flag.Bool("n", false, "line mode")
	flgL       = flag.Bool("l", false, "automate line-end processing. Trim input newline and add it back on -p")
	flgP       = flag.Bool("p", false, "pipe mode. Implies -n and prints Line after each iteration")
	flgG       = flag.Bool("g", false, "run goimports")
	flgA       = flag.Bool("a", false, "autosplit Line to Fields. Implies -n")
	flgF       = flag.String("F", " ", "field separator. Implies -a and -n. See docs for GSplit")
	inplace    = flag.Bool("i", false, "in-place edit mode. See package doc for in-place edit")
	inplaceBak = flag.String("I", "", "in-place edit mode, with backup. See package doc for in-place edit")
	flgKeep    = flag.Bool("k", false, "keep tempdir, for debugging")
	warnings   = flag.Bool("w", false, "print warnings on access to undefined fields and so on")
	goVer      = flag.String("goVer", "1.17", "go version to declare in go.mod file")
	help       = flag.Bool("help", false, "print usage help and exit")
	modules    stringsValue
	beginSrc   stringsValue
	endSrc     stringsValue

	longFlags      = map[string]bool{}
	shortBoolFlags = map[string]bool{}
)

func init() {
	flag.BoolVar(help, "h", false, "print usage help and exit") // alias
	flag.Var(&modules, "M", "modules to import. May be repeated")
	flag.Var(&beginSrc, "b", "code block(s) to insert before record processing")
	flag.Var(&beginSrc, "BEGIN", "code block(s) to insert before record processing") // alias
	flag.Var(&endSrc, "E", "code block(s) to insert after record processing")        //alias
	flag.Var(&endSrc, "END", "code block(s) to insert after record processing")      //alias

	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		if len(f.Name) > 1 {
			longFlags[f.Name] = true
		} else {
			if fv, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && fv.IsBoolFlag() {
				shortBoolFlags[f.Name] = true
			}
		}
	})
}

type stringsValue []string

func (v *stringsValue) Set(s string) error {
	*v = append(*v, s)
	return nil
}

func (v *stringsValue) String() string {
	return fmt.Sprintf("%q", []string(*v))
}

var errGolf = fmt.Errorf("golf returned nonzero status")

type Prog struct {
	RawArgs    []string
	BeginSrc   []string
	RawSrc     string
	EndSrc     []string
	Src        string
	Imports    []string
	FlgN       bool
	FlgP       bool
	FlgL       bool
	FlgA       bool
	FlgF       string
	InPlace    bool
	InPlaceBak string
	Warnings   bool
	Goimports  bool
	Keep       bool
	Prelude    []byte
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
	GolfFlgL = {{ .FlgL }}
	GolfInPlace = {{ .InPlace }}
	GolfInPlaceBak = {{ printf "%q" .InPlaceBak }}
}

func main() {
	// User -b start
	{{- range .BeginSrc}}
	{{.}}
	{{- end }}
	// User -b end
	{{- if .FlgN}}
	const _golfP = {{.FlgP}}
	var _golfPDirty = false
	_golfFlushP := func() {
		if _golfPDirty {
			Print(Line)
			_golfPDirty = false
		}
	}
	_golfCloseOut := func() {
		if CurOut == os.Stdout {
			return
		}
		if err := CurOut.Close(); err != nil {
			Warn("golf: can't close current output: %v", err)
		}
		CurOut = os.Stdout
	}

	_golfFilenames := os.Args[1:]
	if len(_golfFilenames)==0 {
		_golfFilenames=[]string{"/dev/stdin"}
		GolfInPlace = false
		GolfInPlaceBak = ""
	}
File:
    for _, Filename = range _golfFilenames {
		_golfFlushP()
		_golfCloseOut()
		_golfFile, err := os.Open(Filename)
		if err != nil {
			Die(err)
		}
		// NOTE: assumes POSIX fs semantics: a file can be renamed or deleted
		// after being opened.
		if GolfInPlace {
			if GolfInPlaceBak == "" {
				// In the no-backup case, we still need to unlink the input
				// before os.Create, because otherwise the input will be
				// truncated before we read it.
				if err := os.Remove(Filename); err !=nil {
					Die("golf: can't remove input file: %v", err)
				}
			} else {
				bakname := BackupName(Filename, GolfInPlaceBak)
				if os.Rename(Filename, bakname); err != nil {
					Die("golf: in-place backup: %v", err)
				}
			}

			if CurOut, err = os.Create(Filename); err != nil {
				Die("golf: can't create output: %v", err)
			}
		}
		LineNum = 0
		_golfScanner := bufio.NewScanner(_golfFile)
	Line:
		for _golfScanner.Scan() {
			_golfFlushP()
			LineNum++  // 1-based. Be compatible with awk, perl's default.
			// Scanned line.
			// BUG: restores newlines crudely in non-line mode.
			// Should have \r when they were present in input, and should not
			// insert a trailing newline on the last line if it was absent.
			Line = _golfScanner.Text() {{- if not .FlgL}} + "\n"{{end}}
			_golfPDirty = {{ .FlgP }}
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
		if err := _golfScanner.Err(); err != nil {
			Die("%s: %v", Filename, err)
		}
		continue File
	}
	_golfFlushP()
	_golfCloseOut()
	{{- end}}
	// User -E start
	{{- range .EndSrc}}
	{{.}}
	{{- end }}
	// User -E end
}
`))

func (p *Prog) transform() error {
	s := &bytes.Buffer{}
	if err := program.Execute(s, p); err != nil {
		return err
	}

	// Try to pretty it up, but stay silent about errors. The real compiler
	// will give a better error message later.
	if src, err := format.Source(s.Bytes()); err != nil {
		p.Src = s.String()
	} else {
		p.Src = string(src)
	}
	return nil
}

// do runs the command with stdio connected.
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
		prelude.Warn("golf: mkdir tmp: %v\n", err)
		return 1
	}
	if tmpdir, err = filepath.Abs(tmpdir); err != nil { // note =, not :=
		prelude.Warn("golf: abs tmp: %v\n", err)
		return 1
	}

	if p.Keep {
		fmt.Fprintln(os.Stderr, tmpdir)
	} else {
		defer func() {
			if err := os.RemoveAll(tmpdir); err != nil {
				prelude.Warn("golf: rmall tmp: %v\n", err)
				// but don't fail the golf.
			}
		}()
	}

	origdir, err := os.Getwd()
	if err != nil {
		prelude.Warn("golf: original dir: %v\n", err)
		return 1
	}

	if err := os.Chdir(tmpdir); err != nil {
		prelude.Warn("golf: %v", err)
		return 1
	}

	tmpfile := filepath.Join(tmpdir, "golfe.go")
	if err := os.WriteFile(tmpfile, []byte(p.Src), 0666); err != nil {
		prelude.Warn("golf: %v", err)
		return 1
	}

	if p.Goimports {
		prelude.Warn(">>goimports")
		if err := doQ("goimports", []string{"-w", "."}); err != nil {
			prelude.Warn("golf: goimports: %v\n", err)
			return 1
		}
	}

	needTidy := false
	for _, v := range p.Imports {
		if strings.Contains(v, "/") {
			needTidy = true
			break
		}
	}

	if needTidy {
		if err := doQ("go", []string{"mod", "init", "example.com/golf"}); err != nil {
			fmt.Fprintf(os.Stderr, "golf: mod init: %v\n", err)
			return 1
		}
		if err := doQ("go", []string{"mod", "tidy"}); err != nil {
			fmt.Fprintf(os.Stderr, "golf: mod tidy: %v\n", err)
			return 1
		}
	} else {
		// Write it ourselves, which is faster.
		tidy := fmt.Sprintf("module example.com/golf\n\ngo %s\n", *goVer)
		if err := os.WriteFile("go.mod", []byte(tidy), 0666); err != nil {
			fmt.Fprintf(os.Stderr, "golf: writing mod file: %v\n", err)
		}
	}

	/* y u no faster?
	if err := do("go", []string{"tool", "compile", "golfe.go"}); err != nil {
		prelude.Warn("compile: %v", err)
		return 1
	}
	if err := do("go", []string{"tool", "link", "golfe.o"}); err != nil {
		prelude.Warn("link: %v", err)
		return 1
	}
	if err := do("./a.out", p.RawArgs); err != nil {
		prelude.Warn("run: %v", err)
		return 1
	}
	*/

	const binname = "golfing" // should this add .exe on win32?

	if err := do("go", []string{"build", "-o", binname, "."}); err != nil {
		if err != errGolf {
			prelude.Warn("golf: %v", err)
		}
		return 1
	}

	if err := os.Chdir(origdir); err != nil {
		prelude.Warn("golf: returning to original dir: %v", err)
		return 1
	}

	if err := do(filepath.Join(tmpdir, binname), p.RawArgs); err != nil {
		if err != errGolf {
			prelude.Warn("golf: %v", err)
		}
		return 1
	}

	return 0
}

func decluster() {
	res := []string{os.Args[0]}
	for i, v := range os.Args[1:] {
		if v[0] != '-' || longFlags[v[1:]] {
			// Skip a non-flag arguments and known long flags.
			res = append(res, v)
			continue
		}
		if v == "--" {
			res = append(res, os.Args[i:]...)
			break
		}
		for i, vv := range strings.Split(v[1:], "") {
			if i < (len(v)-2) && !shortBoolFlags[vv] {
				// This doesn't protect against -ib, unfortunately.
				// (Our version of -i does not take an arg.)
				prelude.Warn("-%s cannot be used inside a flag cluster", vv)
				flag.PrintDefaults()
				os.Exit(1)
			}
			res = append(res, "-"+vv)
		}
	}
	os.Args = res
}

func dedupe(s []string) []string {
	sort.Strings(s)
	w := 0
	for r, cur := range s {
		if r > 0 && cur == s[r-1] {
			continue
		}
		s[w] = cur
		w++
	}
	return s[:w]
}

const helpString = `Command golf provides some Go one-liner fun.

Invoke it with a snippet of Go code in the -e flag, which will be compiled
and run for you.

  golf -e 'for _, v := range []string{"hi", "bye"} { fmt.Println(v) }'

Additional flags such as -n turn on awk/perl-like line mode, which are useful
in processing text data. See the examples and more in the Godoc for
github.com/gaal/golf.

`

func main() {
	// The standard Go flag package does not support flag clustering.
	// This is too convenient to give up when golfing, so handle it ourselves.
	decluster()
	flag.Parse()

	if *help {
		// TODO: intentional -h output belongs on stdout.
		prelude.Warn(helpString)
		flag.PrintDefaults()
		os.Exit(0)
	}

	// -F implies -a (which in turn implies -n...)
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "F" {
			*flgA = true
		}
	})

	// Both -a and -n imply -n.
	*flgN = *flgN || *flgP || *flgA

	// -I implies -i.
	*inplace = *inplace || len(*inplaceBak) > 0

	imps := []string{"io", "os", "regexp", "strconv", "strings", "fmt"}
	if *flgN {
		imps = append(imps, "bufio")
	}
	if len(modules) > 0 {
		imps = append(imps, modules...)
	}
	imps = dedupe(imps)

	p := &Prog{
		BeginSrc:   beginSrc,
		RawSrc:     *rawSrc,
		EndSrc:     endSrc,
		RawArgs:    flag.Args(),
		Imports:    imps,
		FlgN:       *flgN,
		FlgP:       *flgP,
		FlgL:       *flgL,
		FlgA:       *flgA,
		FlgF:       *flgF,
		InPlace:    *inplace,
		InPlaceBak: *inplaceBak,
		Warnings:   *warnings,
		Goimports:  *flgG,
		Keep:       *flgKeep,
		Prelude:    prelude.Source(),
	}
	if err := p.transform(); err != nil {
		prelude.Warn("golf: %v", err)
		os.Exit(1)
	}
	os.Exit(p.run())
}
