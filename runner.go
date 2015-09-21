package expect

import (
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wsxiaoys/terminal/color"
)

var (
	showStdout  = flag.Bool("vv", false, "turn on stdout")
	matchFlag   = flag.String("m", "", "Regular expression selecting which tests to run")
	summaryPath = flag.String("summary", "", "Path to write a summary file to")
	pattern     *regexp.Regexp
	runner      *Runner
	stdout      = os.Stdout
	silentOut   *os.File
	beforeEach  = make([]func(), 0, 2)
	endTestErr  = new(error)
)

func init() {
	flag.Parse()
	if len(*matchFlag) != 0 {
		pattern = regexp.MustCompile("(?i)" + *matchFlag)
	}
	if *showStdout == true {
		silentOut = stdout
	}
	os.Stdout = silentOut
}

func Expectify(suite interface{}, t *testing.T) {
	var name string
	var res *result
	defer func() {
		if err := recover(); err != nil {
			if err == endTestErr {
				finish(t)
				return
			}
			os.Stdout = stdout
			if res != nil {
				res.Report()
			}
			color.Printf("@R 💣  %-75s\n", name)
			panic(err)
		}
	}()

	tp := reflect.TypeOf(suite)
	sv := reflect.ValueOf(suite)
	count := tp.NumMethod()

	runner = &Runner{
		results: make([]*result, 0, 10),
	}

	each, _ := tp.MethodByName("Each")
	if each.Func.IsValid() && each.Type.NumIn() != 2 {
		each = reflect.Method{}
	}

	announced := false
	for i := 0; i < count; i++ {
		method := tp.Method(i)
		// this method is not exported
		if len(method.PkgPath) != 0 {
			continue
		}
		name = method.Name
		typeName := sv.Elem().Type().String()

		if method.Type.NumIn() != 1 {
			continue
		}

		if pattern != nil && pattern.MatchString(name) == false && pattern.MatchString(typeName) == false {
			continue
		}

		os.Stdout = stdout
		res = runner.Start(name, typeName)
		var f = func() {
			method.Func.Call([]reflect.Value{sv})
			if runner.End() == false || testing.Verbose() {
				if announced == false {
					color.Printf("\n@!%s@|\n", typeName)
					announced = true
				}
				res.Report()
			}
		}
		for i := 0; i < len(beforeEach); i++ {
			beforeEach[i]()
		}
		if each.Func.IsValid() {
			each.Func.Call([]reflect.Value{sv, reflect.ValueOf(f)})
		} else {
			f()
		}
		os.Stdout = silentOut
	}
	finish(t)
}

func finish(t *testing.T) {
	passed := 0
	for _, result := range runner.results {
		if result.Passed() {
			passed++
		}
	}
	failed := len(runner.results) - passed
	if failed != 0 {
		os.Stdout = stdout
		fmt.Println("\nFailure summary")
		for _, r := range runner.results {
			if r.Passed() == false {
				r.Summary()
			}
		}
		fmt.Println()
		os.Stdout = silentOut
		t.Fail()
	}
	if path := *summaryPath; len(path) != 0 {
		updatePersistedSummary(path, passed, failed)
	}
}

func updatePersistedSummary(path string, passed int, failed int) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	buffer := make([]byte, 128)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		panic(err)
	} else if n > 0 {
		file.Truncate(0)
		found := regexp.MustCompile(`(\d+) passed\s*(\d+) failed`).FindAllStringSubmatch(string(buffer), 2)
		if len(found) == 1 && len(found[0]) == 3 {
			if p, err := strconv.Atoi(found[0][1]); err == nil {
				passed += p
			}
			if f, err := strconv.Atoi(found[0][2]); err == nil {
				failed += f
			}
		}
	}

	color.Fprintf(file, "\n* @g%d passed\n", passed)
	if failed > 0 {
		color.Fprintf(file, "* @r%d failed\n", passed)
	} else {
		file.Write([]byte("* 0 failed\n"))
	}
}

func BeforeEach(f func()) {
	beforeEach = append(beforeEach, f)
}

type Runner struct {
	results []*result
	current *result
}

func (r *Runner) Start(name string, typeName string) *result {
	r.current = &result{
		method:   name,
		typeName: typeName,
		start:    time.Now(),
		failures: make([]*Failure, 0, 3),
	}
	r.results = append(r.results, r.current)
	return r.current
}

func (r *Runner) End() bool {
	r.current.end = time.Now()
	passed := r.current.Passed()
	r.current = nil
	return passed
}

func (r *Runner) Skip(format string, args ...interface{}) {
	if r.current != nil {
		r.current.Skip(format, args...)
	}
}

func (r *Runner) Errorf(format string, args ...interface{}) {
	file := "???"
	line := 1
	ok := false
	for i := 3; i < 10; i++ {
		_, file, line, ok = runtime.Caller(i)
		if ok == false || strings.HasSuffix(file, "_test.go") {
			break
		}
	}

	if ok {
		if index := strings.LastIndex(file, "/"); index >= 0 {
			file = file[index+1:]
		} else if index = strings.LastIndex(file, "\\"); index >= 0 {
			file = file[index+1:]
		}
	}

	failure := &Failure{
		message:  fmt.Sprintf(format, args...),
		location: fmt.Sprintf("%s:%d", file, line),
	}
	r.current.failures = append(r.current.failures, failure)
}

func (r *Runner) ErrorMessage(format string, args ...interface{}) {
	if r.current != nil {
		r.current.ErrorMessage(format, args...)
	}
}

type result struct {
	method      string
	failures    []*Failure
	typeName    string
	start       time.Time
	end         time.Time
	skipMessage string
	skip        bool
}

type Failure struct {
	message  string
	location string
}

func (r *result) Skip(format string, args ...interface{}) {
	r.skip = true
	r.skipMessage = fmt.Sprintf(format, args...)
}

func (r *result) Passed() bool {
	return r.skip || len(r.failures) == 0
}

func (r *result) ErrorMessage(format string, args ...interface{}) {
	l := len(r.failures)
	if l > 0 {
		r.failures[l-1].message = fmt.Sprintf(format, args...)
	}
}

func (r *result) Report() {
	if r.end.IsZero() {
		r.end = time.Now()
	}
	info := fmt.Sprintf(" %-70s%dms", r.method, r.end.Sub(r.start).Nanoseconds()/1000000)
	if r.skip {
		color.Println(" @y⸚", info)
		color.Println("   @." + r.skipMessage)
	} else if r.Passed() {
		color.Println(" @g✓", info)
	} else {
		color.Println(" @r×", info)
		for _, failure := range r.failures {
			color.Printf("    @.%-40s%s\n", failure.location, failure.message)
		}
	}
}

func (r *result) Summary() {
	info := fmt.Sprintf(" %s.%-40s", r.typeName, r.method)
	if r.skip {
		color.Println(" @y⸚", info)
	} else if r.Passed() {
		color.Println(" @g✓", info)
	} else {
		color.Print(" @r×", info)
		color.Printf("%2d\n", len(r.failures))
	}
}
