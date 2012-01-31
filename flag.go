// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
	Package flag implements command-line flag parsing in the GNU style.
	It is almost exactly the same as the standard flag package,
	the only difference being the extra argument to Parse.

	Command line flag syntax:
		-f		// single letter flag
		-fg		// two single letter flags together
		--flag	// multiple letter flag
		--flag x  // non-boolean flags only
		-f x		// non-boolean flags only
		-fx		// if f is a non-boolean flag, x is its argument.

	The last three forms are not permitted for boolean flags because the
	meaning of the command
		cmd -f *
	will change if there is a file called 0, false, etc.  There is currently
	no way to turn off a boolean flag.

	Flag parsing stops after the terminator "--", or just before the first
	non-flag argument ("-" is a non-flag argument) if the interspersed
	argument to Parse is false. 
*/
package flag

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

// ErrHelp is the error returned if the flag -help is invoked but no such flag is defined.
var ErrHelp = errors.New("flag: help requested")

// -- bool Value
type boolValue bool

func newBoolValue(val bool, p *bool) *boolValue {
	*p = val
	return (*boolValue)(p)
}

func (b *boolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b = boolValue(v)
	return err
}

func (b *boolValue) String() string { return fmt.Sprintf("%v", *b) }

// -- int Value
type intValue int

func newIntValue(val int, p *int) *intValue {
	*p = val
	return (*intValue)(p)
}

func (i *intValue) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	*i = intValue(v)
	return err
}

func (i *intValue) String() string { return fmt.Sprintf("%v", *i) }

// -- int64 Value
type int64Value int64

func newInt64Value(val int64, p *int64) *int64Value {
	*p = val
	return (*int64Value)(p)
}

func (i *int64Value) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	*i = int64Value(v)
	return err
}

func (i *int64Value) String() string { return fmt.Sprintf("%v", *i) }

// -- uint Value
type uintValue uint

func newUintValue(val uint, p *uint) *uintValue {
	*p = val
	return (*uintValue)(p)
}

func (i *uintValue) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	*i = uintValue(v)
	return err
}

func (i *uintValue) String() string { return fmt.Sprintf("%v", *i) }

// -- uint64 Value
type uint64Value uint64

func newUint64Value(val uint64, p *uint64) *uint64Value {
	*p = val
	return (*uint64Value)(p)
}

func (i *uint64Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 64)
	*i = uint64Value(v)
	return err
}

func (i *uint64Value) String() string { return fmt.Sprintf("%v", *i) }

// -- string Value
type stringValue string

func newStringValue(val string, p *string) *stringValue {
	*p = val
	return (*stringValue)(p)
}

func (s *stringValue) Set(val string) error {
	*s = stringValue(val)
	return nil
}

func (s *stringValue) String() string { return fmt.Sprintf("%s", *s) }

// -- float64 Value
type float64Value float64

func newFloat64Value(val float64, p *float64) *float64Value {
	*p = val
	return (*float64Value)(p)
}

func (f *float64Value) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	*f = float64Value(v)
	return err
}

func (f *float64Value) String() string { return fmt.Sprintf("%v", *f) }

// -- time.Duration Value
type durationValue time.Duration

func newDurationValue(val time.Duration, p *time.Duration) *durationValue {
	*p = val
	return (*durationValue)(p)
}

func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	*d = durationValue(v)
	return err
}

func (d *durationValue) String() string { return (*time.Duration)(d).String() }

// Value is the interface to the dynamic value stored in a flag.
// (The default value is represented as a string.)
type Value interface {
	String() string
	Set(string) error
}

// ErrorHandling defines how to handle flag parsing errors.
type ErrorHandling int

const (
	ContinueOnError ErrorHandling = iota
	ExitOnError
	PanicOnError
)

// A FlagSet represents a set of defined flags.
type FlagSet struct {
	// Usage is the function called when an error occurs while parsing flags.
	// The field is a function (not a method) that may be changed to point to
	// a custom error handler.
	Usage func()

	// Errors will be printed to Stderr, which is set to os.Stderr by default.
	Stderr io.Writer

	name             string
	parsed           bool
	actual           map[string]*Flag
	formal           map[string]*Flag
	args             []string // arguments after flags
	procArgs         []string // arguments being processed (gnu only)
	procFlag         string   // flag being processed (gnu only)
	allowIntersperse bool     // (gnu only)
	exitOnError      bool     // does the program exit if there's an error?
	errorHandling    ErrorHandling
}

// A Flag represents the state of a flag.
type Flag struct {
	Name     string // name as it appears on command line
	Usage    string // help message
	Value    Value  // value as set
	DefValue string // default value (as text); for usage message
}

// sortFlags returns the flags as a slice in lexicographical sorted order.
func sortFlags(flags map[string]*Flag) []*Flag {
	list := make(sort.StringSlice, len(flags))
	i := 0
	for _, f := range flags {
		list[i] = f.Name
		i++
	}
	list.Sort()
	result := make([]*Flag, len(list))
	for i, name := range list {
		result[i] = flags[name]
	}
	return result
}

// VisitAll visits the flags in lexicographical order, calling fn for each.
// It visits all flags, even those not set.
func (f *FlagSet) VisitAll(fn func(*Flag)) {
	for _, flag := range sortFlags(f.formal) {
		fn(flag)
	}
}

// VisitAll visits the command-line flags in lexicographical order, calling
// fn for each.  It visits all flags, even those not set.
func VisitAll(fn func(*Flag)) {
	commandLine.VisitAll(fn)
}

// Visit visits the flags in lexicographical order, calling fn for each.
// It visits only those flags that have been set.
func (f *FlagSet) Visit(fn func(*Flag)) {
	for _, flag := range sortFlags(f.actual) {
		fn(flag)
	}
}

// Visit visits the command-line flags in lexicographical order, calling fn
// for each.  It visits only those flags that have been set.
func Visit(fn func(*Flag)) {
	commandLine.Visit(fn)
}

// Lookup returns the Flag structure of the named flag, returning nil if none exists.
func (f *FlagSet) Lookup(name string) *Flag {
	return f.formal[name]
}

// Lookup returns the Flag structure of the named command-line flag,
// returning nil if none exists.
func Lookup(name string) *Flag {
	return commandLine.formal[name]
}

// Set sets the value of the named flag.
func (f *FlagSet) Set(name, value string) error {
	flag, ok := f.formal[name]
	if !ok {
		return fmt.Errorf("no such flag -%v", name)
	}
	err := flag.Value.Set(value)
	if err != nil {
		return err
	}
	if f.actual == nil {
		f.actual = make(map[string]*Flag)
	}
	f.actual[name] = flag
	return nil
}

// Set sets the value of the named command-line flag.
func Set(name, value string) error {
	return commandLine.Set(name, value)
}

type flagsByLength []*Flag

func (f flagsByLength) Less(i, j int) bool {
	return len(f[i].Name) < len(f[j].Name)
}
func (f flagsByLength) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f flagsByLength) Len() int {
	return len(f)
}

type flagsByName [][]*Flag

func (f flagsByName) Less(i, j int) bool {
	return f[i][0].Name < f[j][0].Name
}
func (f flagsByName) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f flagsByName) Len() int {
	return len(f)
}

// PrintDefaults prints to f.Stderr the default values of all defined flags in the set.
// If there is more than one name for a given flag, the usage information and
// default value from the shortest will be printed.
func (f *FlagSet) PrintDefaults() {
	// group together all flags for a given value
	flags := make(map[interface{}]flagsByLength)
	f.VisitAll(func(f *Flag) {
		flags[f.Value] = append(flags[f.Value], f)
	})

	// sort the output flags by shortest name for each group.
	var byName flagsByName
	for _, f := range flags {
		sort.Sort(f)
		byName = append(byName, f)
	}
	sort.Sort(byName)

	var line bytes.Buffer
	for _, fs := range byName {
		line.Reset()
		for i, f := range fs {
			if i > 0 {
				line.WriteString(", ")
			}
			line.WriteString(flagWithMinus(f.Name))
		}
		format := "%s  (= %s)\n    %s\n"
		if _, ok := fs[0].Value.(*stringValue); ok {
			// put quotes on the value
			format = "%s (= %q)\n    %s\n"
		}
		fmt.Fprintf(f.Stderr, format, line.Bytes(), fs[0].DefValue, fs[0].Usage)
	}
}

// PrintDefaults prints to standard error the default values of all defined command-line flags.
func PrintDefaults() {
	commandLine.PrintDefaults()
}

// defaultUsage is the default function to print a usage message.
func defaultUsage(f *FlagSet) {
	fmt.Fprintf(f.Stderr, "Usage of %s:\n", f.name)
	f.PrintDefaults()
}

// NOTE: Usage is not just defaultUsage(commandLine)
// because it serves (via godoc flag Usage) as the example
// for how to write your own usage function.

// Usage prints to standard error a usage message documenting all defined command-line flags.
// The function is a variable that may be changed to point to a custom function.
var Usage = func() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	PrintDefaults()
}

// NFlag returns the number of flags that have been set.
func (f *FlagSet) NFlag() int { return len(f.actual) }

// NFlag returns the number of command-line flags that have been set.
func NFlag() int { return len(commandLine.actual) }

// Arg returns the i'th argument.  Arg(0) is the first remaining argument
// after flags have been processed.
func (f *FlagSet) Arg(i int) string {
	if i < 0 || i >= len(f.args) {
		return ""
	}
	return f.args[i]
}

// Arg returns the i'th command-line argument.  Arg(0) is the first remaining argument
// after flags have been processed.
func Arg(i int) string {
	return commandLine.Arg(i)
}

// NArg is the number of arguments remaining after flags have been processed.
func (f *FlagSet) NArg() int { return len(f.args) }

// NArg is the number of arguments remaining after flags have been processed.
func NArg() int { return len(commandLine.args) }

// Args returns the non-flag arguments.
func (f *FlagSet) Args() []string { return f.args }

// Args returns the non-flag command-line arguments.
func Args() []string { return commandLine.args }

// BoolVar defines a bool flag with specified name, default value, and usage string.
// The argument p points to a bool variable in which to store the value of the flag.
func (f *FlagSet) BoolVar(p *bool, name string, value bool, usage string) {
	f.Var(newBoolValue(value, p), name, usage)
}

// BoolVar defines a bool flag with specified name, default value, and usage string.
// The argument p points to a bool variable in which to store the value of the flag.
func BoolVar(p *bool, name string, value bool, usage string) {
	commandLine.Var(newBoolValue(value, p), name, usage)
}

// Bool defines a bool flag with specified name, default value, and usage string.
// The return value is the address of a bool variable that stores the value of the flag.
func (f *FlagSet) Bool(name string, value bool, usage string) *bool {
	p := new(bool)
	f.BoolVar(p, name, value, usage)
	return p
}

// Bool defines a bool flag with specified name, default value, and usage string.
// The return value is the address of a bool variable that stores the value of the flag.
func Bool(name string, value bool, usage string) *bool {
	return commandLine.Bool(name, value, usage)
}

// IntVar defines an int flag with specified name, default value, and usage string.
// The argument p points to an int variable in which to store the value of the flag.
func (f *FlagSet) IntVar(p *int, name string, value int, usage string) {
	f.Var(newIntValue(value, p), name, usage)
}

// IntVar defines an int flag with specified name, default value, and usage string.
// The argument p points to an int variable in which to store the value of the flag.
func IntVar(p *int, name string, value int, usage string) {
	commandLine.Var(newIntValue(value, p), name, usage)
}

// Int defines an int flag with specified name, default value, and usage string.
// The return value is the address of an int variable that stores the value of the flag.
func (f *FlagSet) Int(name string, value int, usage string) *int {
	p := new(int)
	f.IntVar(p, name, value, usage)
	return p
}

// Int defines an int flag with specified name, default value, and usage string.
// The return value is the address of an int variable that stores the value of the flag.
func Int(name string, value int, usage string) *int {
	return commandLine.Int(name, value, usage)
}

// Int64Var defines an int64 flag with specified name, default value, and usage string.
// The argument p points to an int64 variable in which to store the value of the flag.
func (f *FlagSet) Int64Var(p *int64, name string, value int64, usage string) {
	f.Var(newInt64Value(value, p), name, usage)
}

// Int64Var defines an int64 flag with specified name, default value, and usage string.
// The argument p points to an int64 variable in which to store the value of the flag.
func Int64Var(p *int64, name string, value int64, usage string) {
	commandLine.Var(newInt64Value(value, p), name, usage)
}

// Int64 defines an int64 flag with specified name, default value, and usage string.
// The return value is the address of an int64 variable that stores the value of the flag.
func (f *FlagSet) Int64(name string, value int64, usage string) *int64 {
	p := new(int64)
	f.Int64Var(p, name, value, usage)
	return p
}

// Int64 defines an int64 flag with specified name, default value, and usage string.
// The return value is the address of an int64 variable that stores the value of the flag.
func Int64(name string, value int64, usage string) *int64 {
	return commandLine.Int64(name, value, usage)
}

// UintVar defines a uint flag with specified name, default value, and usage string.
// The argument p points to a uint variable in which to store the value of the flag.
func (f *FlagSet) UintVar(p *uint, name string, value uint, usage string) {
	f.Var(newUintValue(value, p), name, usage)
}

// UintVar defines a uint flag with specified name, default value, and usage string.
// The argument p points to a uint  variable in which to store the value of the flag.
func UintVar(p *uint, name string, value uint, usage string) {
	commandLine.Var(newUintValue(value, p), name, usage)
}

// Uint defines a uint flag with specified name, default value, and usage string.
// The return value is the address of a uint  variable that stores the value of the flag.
func (f *FlagSet) Uint(name string, value uint, usage string) *uint {
	p := new(uint)
	f.UintVar(p, name, value, usage)
	return p
}

// Uint defines a uint flag with specified name, default value, and usage string.
// The return value is the address of a uint  variable that stores the value of the flag.
func Uint(name string, value uint, usage string) *uint {
	return commandLine.Uint(name, value, usage)
}

// Uint64Var defines a uint64 flag with specified name, default value, and usage string.
// The argument p points to a uint64 variable in which to store the value of the flag.
func (f *FlagSet) Uint64Var(p *uint64, name string, value uint64, usage string) {
	f.Var(newUint64Value(value, p), name, usage)
}

// Uint64Var defines a uint64 flag with specified name, default value, and usage string.
// The argument p points to a uint64 variable in which to store the value of the flag.
func Uint64Var(p *uint64, name string, value uint64, usage string) {
	commandLine.Var(newUint64Value(value, p), name, usage)
}

// Uint64 defines a uint64 flag with specified name, default value, and usage string.
// The return value is the address of a uint64 variable that stores the value of the flag.
func (f *FlagSet) Uint64(name string, value uint64, usage string) *uint64 {
	p := new(uint64)
	f.Uint64Var(p, name, value, usage)
	return p
}

// Uint64 defines a uint64 flag with specified name, default value, and usage string.
// The return value is the address of a uint64 variable that stores the value of the flag.
func Uint64(name string, value uint64, usage string) *uint64 {
	return commandLine.Uint64(name, value, usage)
}

// StringVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a string variable in which to store the value of the flag.
func (f *FlagSet) StringVar(p *string, name string, value string, usage string) {
	f.Var(newStringValue(value, p), name, usage)
}

// StringVar defines a string flag with specified name, default value, and usage string.
// The argument p points to a string variable in which to store the value of the flag.
func StringVar(p *string, name string, value string, usage string) {
	commandLine.Var(newStringValue(value, p), name, usage)
}

// String defines a string flag with specified name, default value, and usage string.
// The return value is the address of a string variable that stores the value of the flag.
func (f *FlagSet) String(name string, value string, usage string) *string {
	p := new(string)
	f.StringVar(p, name, value, usage)
	return p
}

// String defines a string flag with specified name, default value, and usage string.
// The return value is the address of a string variable that stores the value of the flag.
func String(name string, value string, usage string) *string {
	return commandLine.String(name, value, usage)
}

// Float64Var defines a float64 flag with specified name, default value, and usage string.
// The argument p points to a float64 variable in which to store the value of the flag.
func (f *FlagSet) Float64Var(p *float64, name string, value float64, usage string) {
	f.Var(newFloat64Value(value, p), name, usage)
}

// Float64Var defines a float64 flag with specified name, default value, and usage string.
// The argument p points to a float64 variable in which to store the value of the flag.
func Float64Var(p *float64, name string, value float64, usage string) {
	commandLine.Var(newFloat64Value(value, p), name, usage)
}

// Float64 defines a float64 flag with specified name, default value, and usage string.
// The return value is the address of a float64 variable that stores the value of the flag.
func (f *FlagSet) Float64(name string, value float64, usage string) *float64 {
	p := new(float64)
	f.Float64Var(p, name, value, usage)
	return p
}

// Float64 defines a float64 flag with specified name, default value, and usage string.
// The return value is the address of a float64 variable that stores the value of the flag.
func Float64(name string, value float64, usage string) *float64 {
	return commandLine.Float64(name, value, usage)
}

// DurationVar defines a time.Duration flag with specified name, default value, and usage string.
// The argument p points to a time.Duration variable in which to store the value of the flag.
func (f *FlagSet) DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	f.Var(newDurationValue(value, p), name, usage)
}

// DurationVar defines a time.Duration flag with specified name, default value, and usage string.
// The argument p points to a time.Duration variable in which to store the value of the flag.
func DurationVar(p *time.Duration, name string, value time.Duration, usage string) {
	commandLine.Var(newDurationValue(value, p), name, usage)
}

// Duration defines a time.Duration flag with specified name, default value, and usage string.
// The return value is the address of a time.Duration variable that stores the value of the flag.
func (f *FlagSet) Duration(name string, value time.Duration, usage string) *time.Duration {
	p := new(time.Duration)
	f.DurationVar(p, name, value, usage)
	return p
}

// Duration defines a time.Duration flag with specified name, default value, and usage string.
// The return value is the address of a time.Duration variable that stores the value of the flag.
func Duration(name string, value time.Duration, usage string) *time.Duration {
	return commandLine.Duration(name, value, usage)
}

// Var defines a flag with the specified name and usage string. The type and
// value of the flag are represented by the first argument, of type Value, which
// typically holds a user-defined implementation of Value. For instance, the
// caller could create a flag that turns a comma-separated string into a slice
// of strings by giving the slice the methods of Value; in particular, Set would
// decompose the comma-separated string into the slice.
func (f *FlagSet) Var(value Value, name string, usage string) {
	// Remember the default value as a string; it won't change.
	flag := &Flag{name, usage, value, value.String()}
	_, alreadythere := f.formal[name]
	if alreadythere {
		fmt.Fprintf(f.Stderr, "%s flag redefined: %s\n", f.name, name)
		panic("flag redefinition") // Happens only if flags are declared with identical names
	}
	if f.formal == nil {
		f.formal = make(map[string]*Flag)
	}
	f.formal[name] = flag
}

// Var defines a flag with the specified name and usage string. The type and
// value of the flag are represented by the first argument, of type Value, which
// typically holds a user-defined implementation of Value. For instance, the
// caller could create a flag that turns a comma-separated string into a slice
// of strings by giving the slice the methods of Value; in particular, Set would
// decompose the comma-separated string into the slice.
func Var(value Value, name string, usage string) {
	commandLine.Var(value, name, usage)
}

// failf prints to standard error a formatted error and usage message and
// returns the error.
func (f *FlagSet) failf(format string, a ...interface{}) error {
	err := fmt.Errorf(format, a...)
	fmt.Fprintln(f.Stderr, err)
	f.usage()
	return err
}

// usage calls the Usage method for the flag set, or the usage function if
// the flag set is commandLine.
func (f *FlagSet) usage() {
	if f == commandLine {
		Usage()
	} else if f.Usage == nil {
		defaultUsage(f)
	} else {
		f.Usage()
	}
}

func (f *FlagSet) parseOneGnu() (flagName string, finished bool, err error) {
	if len(f.procArgs) == 0 {
		return "", true, nil
	}

	// processing previously encountered single-rune flag
	if flag := f.procFlag; len(flag) > 0 {
		_, n := utf8.DecodeRuneInString(flag)
		f.procFlag = flag[n:]
		return flag[0:n], false, nil
	}

	a := f.procArgs[0]

	// one non-flag argument
	if a == "-" || a == "" || a[0] != '-' {
		if f.allowIntersperse {
			f.args = append(f.args, a)
			f.procArgs = f.procArgs[1:]
			return "", false, nil
		}
		f.args = append(f.args, f.procArgs...)
		f.procArgs = nil
		return "", true, nil
	}

	// end of flags
	if f.procArgs[0] == "--" {
		f.args = append(f.args, f.procArgs[1:]...)
		f.procArgs = nil
		return "", true, nil
	}

	// long flag signified with "--" prefix
	if a[1] == '-' {
		// TODO allow '=' so we can set bools to false?
		flagName = a[2:]
		f.procArgs = f.procArgs[1:]
		return
	}

	// some number of single-rune flags
	a = a[1:]
	_, n := utf8.DecodeRuneInString(a)
	flagName = a[0:n]
	f.procFlag = a[n:]
	f.procArgs = f.procArgs[1:]
	return
}

func flagWithMinus(name string) string {
	if len(name) > 1 {
		return "--" + name
	}
	return "-" + name
}

func (f *FlagSet) parseGnuFlagArg(name string) (finished bool, err error) {
	m := f.formal
	flag, alreadythere := m[name] // BUG
	if !alreadythere {
		if name == "help" || name == "h" { // special case for nice help message.
			f.usage()
			return false, ErrHelp
		}
		// TODO print --xxx when flag is more than one rune.
		return false, f.failf("flag provided but not defined: %s", flagWithMinus(name))
	}
	if fv, ok := flag.Value.(*boolValue); ok { // special case: doesn't need an arg
		fv.Set("true")
	} else {
		// It must have a value, which might be the next argument.
		var hasValue bool
		var value string
		if f.procFlag != "" {
			// value directly follows flag
			value = f.procFlag
			hasValue = true
			f.procFlag = ""
		}
		if !hasValue && len(f.procArgs) > 0 {
			// value is the next arg
			hasValue = true
			value, f.procArgs = f.procArgs[0], f.procArgs[1:]
		}
		if !hasValue {
			return false, f.failf("flag needs an argument: %s", flagWithMinus(name))
		}
		if err := flag.Value.Set(value); err != nil {
			return false, f.failf("invalid value %q for flag %s: %v", value, flagWithMinus(name), err)
		}
	}
	if f.actual == nil {
		f.actual = make(map[string]*Flag)
	}
	f.actual[name] = flag
	return
}

// Parse parses flag definitions from the argument list, which should not
// include the command name.  Must be called after all flags in the FlagSet
// are defined and before flags are accessed by the program.
// The return value will be ErrHelp if --help was set but not defined.
// If allowIntersperse is set, arguments and flags can be interspersed, that
// is flags can follow positional arguments.
func (f *FlagSet) Parse(allowIntersperse bool, arguments []string) error {
	f.parsed = true
	f.procArgs = arguments
	f.procFlag = ""
	f.args = nil
	f.allowIntersperse = allowIntersperse
	for {
		name, finished, err := f.parseOneGnu()
		if !finished {
			if name != "" {
				finished, err = f.parseGnuFlagArg(name)
			}
		}
		if err != nil {
			switch f.errorHandling {
			case ContinueOnError:
				return err
			case ExitOnError:
				os.Exit(2)
			case PanicOnError:
				panic(err)
			}
		}
		if !finished {
			continue
		}
		if err == nil {
			break
		}
	}
	return nil
}

// Parsed reports whether f.Parse has been called.
func (f *FlagSet) Parsed() bool {
	return f.parsed
}

// Parse parses the command-line flags from os.Args[1:].  Must be called
// after all flags are defined and before flags are accessed by the program.
// If allowIntersperse is set, arguments and flags can be interspersed, that
// is flags can follow positional arguments.
func Parse(allowIntersperse bool) {
	// Ignore errors; commandLine is set for ExitOnError.
	commandLine.Parse(allowIntersperse, os.Args[1:])
}

// Parsed returns true if the command-line flags have been parsed.
func Parsed() bool {
	return commandLine.Parsed()
}

// The default set of command-line flags, parsed from os.Args.
var commandLine = NewFlagSet(os.Args[0], ExitOnError)

// NewFlagSet returns a new, empty flag set with the specified name and
// error handling property.
func NewFlagSet(name string, errorHandling ErrorHandling) *FlagSet {
	f := &FlagSet{
		name:          name,
		errorHandling: errorHandling,
		Stderr:        os.Stderr,
	}
	return f
}

// Init sets the name and error handling property for a flag set.
// By default, the zero FlagSet uses an empty name and the
// ContinueOnError error handling policy.
func (f *FlagSet) Init(name string, errorHandling ErrorHandling) {
	f.name = name
	f.errorHandling = errorHandling
}