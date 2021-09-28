# golf: Go one-liner fun.

[![GoDev](https://img.shields.io/static/v1?label=godev&message=reference&color=00add8)][godev]

Invoke golf with a snippet of Go code in the `-e` flag, which will be compiled
and run for you.

Additional flags such as `-n` turn on awk/perl-like line mode, which are useful
in processing text data. See the examples below and in the godoc.

Some variables and functions are provided in the prelude package. These are
inlined and made available to the one-liner. They are common elements of
one-liner coding, for example the current line being processed in line mode.

[godev]: https://pkg.go.dev/github.com/gaal/golf#section-documentation

## Examples

```
  # cat -n (see more about "line mode" in command godoc)
  golf -ne 'fmt.Printf("%6d  %s", LineNum, Line)' FILE1 FILE2

  # head MYFILE
  golf -pe 'if LineNum == 10 { break File }' MYFILE

  # -a mode (which implies -n) automatically splits input fields.
  # These can be accessed from the Fields slice, or using
  # the convenient Field accessor (supports 1-based and negative indexes).
  ps aux | golf -ale 'Print(Field(5))'

  # sum sizes. Note flags replacing awk/perl BEGIN and END blocks.
  ls -l | golf -al -BEGIN 'sum := 0' -e 'sum += GAtoi(Field(5))' -END 'Print(sum)'

  # Convert TSV to CSV.
  golf -F '/\t/' -ple 'for i, v := range Fields { Fields[i] = strconv.Quote(v) }; Line = Join(Fields, ",")'

  # Upper-case the contents of files, editing them in-place.
  # -i does the same with no backup.
  golf -pI .bak -e 'Line = strings.ToUpper(Line)' FILE1 FILE2
```

## Install

```
go get -u github.com/gaal/golf
```

## License

MIT - See [LICENSE][license] file

[license]: https://github.com/gaal/golf/blob/master/LICENSE