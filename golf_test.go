package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	testBin, testSrcDir string // shared by all tests.
)

func initTmp() func() {
	tmpdir, err := os.MkdirTemp("", "golftest-")
	if err != nil {
		panic(fmt.Errorf("golf: mkdir tmp: %v\n", err))
	}
	if testSrcDir, err = filepath.Abs(tmpdir); err != nil { // note =, not :=
		panic(fmt.Errorf("golf: abs tmp: %v\n", err))
	}
	testBin = filepath.Join(testSrcDir, "golf")
	if err = do("go", []string{"build", "-o", testBin}); err != nil {
		panic(fmt.Errorf("golf: build: %v\n", err))
	}
	return func() {
		if err := os.RemoveAll(testSrcDir); err != nil {
			panic(fmt.Errorf("golf: cleanup: %v\n", err))
		}
	}
}

func TestMain(m *testing.M) {
	cleanup := initTmp()
	ret := m.Run()
	cleanup()
	os.Exit(ret)
}

func TestBasics(t *testing.T) {
	data := []struct {
		desc   string
		script string // -e
		args   []string
		want   string
	}{
		{"hello, world", `Print("hello, world")`, nil, "hello, world"},
		{"Print numbers", `Print(42, 54)`, nil, "42 54"},
		{"output -l", `Print("hello, world")`, []string{"-l"}, "hello, world\n"},
		{"BEGIN/END", `i++`, []string{"-b", "i := 0", "-BEGIN", "i = 10", "-END", "i *= 2", "-E", "Print(i)"}, "22"},
		{"-M", "pi := math.Pi; Print(strconv.Itoa(int(pi)))", []string{"-M", "math", "-M", "strconv"}, "3"},
		{"-g", "pi := math.Pi; Print(strconv.Itoa(int(pi)))", []string{"-g"}, "3"},
	}
	for _, d := range data {
		d := d
		t.Run(d.desc, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"-e", d.script}, d.args...)
			out, err := exec.Command(testBin, args...).Output()
			if err != nil {
				t.Fatalf("%v: %v: go run: %v\n%s", d.desc, args, err, err.(*exec.ExitError).Stderr)
			}
			if diff := cmp.Diff(d.want, string(out)); diff != "" {
				t.Fatalf("%v: unexpected result. diff(-want,+got):\n%v", d.desc, diff)
			}
		})
	}
}

func TestLineModes(t *testing.T) {
	data := []struct {
		desc         string
		script       string // -e
		args         []string
		filesIn      map[string]string
		wantFilesOut map[string]string // nil == input unchanged
		wantStdout   string
	}{
		{"-lane", `Printf("%s:%d:%s\n", Filename,LineNum,Field(1))`,
			[]string{"-lan", "f1", "f2"},
			map[string]string{"f1": "Once upon a time\nthere was a", "f2": "Go programmer\n"},
			nil,
			"f1:1:Once\nf1:2:there\nf2:1:Go\n"},
		{"-laF :", `Printf("%s:%d:%s\n", Filename,LineNum,Field(1))`,
			[]string{"-laF", ":", "f1", "f2"},
			map[string]string{"f1": "Once:upon:a:time\nthere:was:a", "f2": "Go programmer\n"},
			nil,
			"f1:1:Once\nf1:2:there\nf2:1:Go programmer\n"},
		{`-plaF '/\t+/'`, `Line = Field(2)`,
			[]string{"-plaF", `/\t+/`, "f1"},
			map[string]string{"f1": "Once\t\t\tupon\t\t\ta time\nthere\twas\ta"},
			nil,
			"upon\nwas\n"},
		{"-lpi", `Line = strings.ToUpper(Line)`,
			[]string{"-lpi", "f1", "f2"},
			map[string]string{"f1": "Once upon a time\nthere was a", "f2": "Go programmer\n"},
			map[string]string{"f1": "ONCE UPON A TIME\nTHERE WAS A\n", "f2": "GO PROGRAMMER\n"},
			""},
		{"-lp -I .bak", `Line = strings.ToUpper(Line); fmt.Fprintln(os.Stdout, LineNum)`,
			[]string{"-lp", "-I", ".bak", "f1", "f2"},
			map[string]string{"f1": "Once upon a time\nthere was a", "f2": "Go programmer\n"},
			map[string]string{"f1": "ONCE UPON A TIME\nTHERE WAS A\n", "f2": "GO PROGRAMMER\n",
				"f1.bak": "Once upon a time\nthere was a", "f2.bak": "Go programmer\n",
			},
			"1\n2\n1\n"},
		{"-lp -I orig_*", `Line = strings.ToUpper(Line)`,
			[]string{"-lp", "-I", "orig_*", "f1", "f2"},
			map[string]string{"f1": "Once upon a time\nthere was a", "f2": "Go programmer\n"},
			map[string]string{"f1": "ONCE UPON A TIME\nTHERE WAS A\n", "f2": "GO PROGRAMMER\n",
				"orig_f1": "Once upon a time\nthere was a", "orig_f2": "Go programmer\n",
			},
			""},
	}
	for _, d := range data {
		d := d
		t.Run(d.desc, func(t *testing.T) {
			// These tests can't run in parallel, because of Chdir.
			origdir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Chdir(origdir); err != nil {
					t.Fatal(err)
				}
			}()
			tdir := t.TempDir()
			if err := os.Chdir(tdir); err != nil {
				t.Fatal(err)
			}
			for name, data := range d.filesIn {
				if err := os.WriteFile(filepath.Join(tdir, name), []byte(data), 0640); err != nil {
					t.Fatalf("write test input: %v", err)
				}
			}
			args := append([]string{"-e", d.script}, d.args...)
			out, err := exec.Command(testBin, args...).Output()
			if err != nil {
				t.Fatalf("%v: go run: %v\n%s", d.desc, err, err.(*exec.ExitError).Stderr)
			}
			if diff := cmp.Diff(d.wantStdout, string(out)); diff != "" {
				t.Fatalf("%v: golf %v: unexpected stdout. diff(-want,+got):\n%v", d.desc, args, diff)
			}
			if d.wantFilesOut == nil {
				d.wantFilesOut = d.filesIn
			}
			for name, wantData := range d.wantFilesOut {
				data, err := os.ReadFile(filepath.Join(tdir, name))
				if err != nil {
					t.Errorf("can't read expected output file: %v", err)
					continue
				}
				if diff := cmp.Diff(wantData, string(data)); diff != "" {
					t.Fatalf("%v: unexpected content for %q. diff(-want,+got):\n%v", d.desc, name, diff)
				}
			}
		})
	}
}
