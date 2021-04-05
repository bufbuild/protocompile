package protocompile

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/linker"
	"github.com/jhump/protocompile/options"
	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
	"github.com/jhump/protocompile/sourceinfo"
)

// Compiler handles compilation tasks, to turn protobuf source files, or other
// intermediate representations, into fully linked descriptors.
//
// The compilation process involves five steps for each protobuf source file:
//   1. Parsing the source into an AST (abstract syntax tree).
//   2. Converting the AST into descriptor protos.
//   3. Linking descriptor protos into fully linked descriptors.
//   4. Interpreting options.
//   5. Computing source code information.
//
// With fully linked descriptors, code generators and protoc plugins could be
// invoked (though that step is not implemented by this package and not a
// responsibility of this type).
type Compiler struct {
	// Resolves path/file names into source code or intermediate representions
	// for protobuf source files. This is how the compiler loads the files to
	// be compiled as well as all dependencies. This field is the only required
	// field.
	Resolver Resolver
	// The maximum parallelism to use when compiling. If unspecified or set to
	// a non-positive value, then min(runtime.NumCPU(), runtime.GOMAXPROCS(-1))
	// will be used.
	MaxParallelism int
	// A custom error and warning reporter. If unspecified a default reporter
	// is used. A default reporter fails the compilation after encountering any
	// errors and ignores all warnings.
	Reporter reporter.Reporter

	// If true, source code information will be included in the resulting
	// descriptors. Source code information is metadata in the file descriptor
	// that provides position information (i.e. the line and column where file
	// elements were defined) as well as comments.
	//
	// If Resolver returns descriptors or descriptor protos for a file, then
	// those descriptors will not be modified. If they do not already include
	// source code info, they will be left that way when the compile operation
	// concludes. Similarly, if they already have source code info but this flag
	// is false, existing info will be left in place.
	IncludeSourceInfo bool
}

// Compile compiles the given file names into fully-linked descriptors. The
// compiler's resolver is used to locate source code (or intermediate artifacts
// such as parsed ASTs or descriptor protos) and then do what is necessary to
// transform that into descriptors (parsing, linking, etc).
func (c *Compiler) Compile(ctx context.Context, files ...string) (linker.Files, error) {
	if len(files) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	par := c.MaxParallelism
	if par <= 0 {
		par = runtime.GOMAXPROCS(-1)
		cpus := runtime.NumCPU()
		if par > cpus {
			par = cpus
		}
	}

	h := reporter.NewHandler(c.Reporter)

	e := executor{
		c:       c,
		h:       h,
		s:       semaphore.NewWeighted(int64(par)),
		cancel:  cancel,
		sym:     &linker.Symbols{},
		results: map[string]*result{},
	}

	results := make([]*result, len(files))
	for i, f := range files {
		results[i] = e.compile(ctx, f)
	}

	descs := make([]linker.File, len(files))
	for i, r := range results {
		select {
		case <-r.ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if r.err != nil {
			return nil, r.err
		}
		descs[i] = r.res
	}

	return descs, nil
}

type result struct {
	ready chan struct{}
	res   linker.File
	err   error
}

func (r *result) fail(err error) {
	r.err = err
	close(r.ready)
}

func (r *result) complete(f linker.File) {
	r.res = f
	close(r.ready)
}

type executor struct {
	c      *Compiler
	h      *reporter.Handler
	s      *semaphore.Weighted
	cancel context.CancelFunc
	sym    *linker.Symbols

	mu      sync.Mutex
	results map[string]*result
}

func (e *executor) compile(ctx context.Context, file string) *result {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.results[file]
	if r != nil {
		return r
	}

	r = &result{
		ready: make(chan struct{}),
	}
	e.results[file] = r
	go func() {
		e.doCompile(ctx, file, r)
	}()
	return r
}

func (e *executor) doCompile(ctx context.Context, file string, r *result) {
	t := task{e: e}
	if err := e.s.Acquire(ctx, 1); err != nil {
		r.fail(err)
		return
	}
	defer t.release()

	sr, err := e.c.Resolver.FindFileByPath(file)
	if err != nil {
		r.fail(err)
		return
	}

	defer func() {
		// if results included a result, don't leave it open if it can be closed
		if sr.Source == nil {
			return
		}
		if c, ok := sr.Source.(io.Closer); ok {
			_ = c.Close()
		}
	}()

	desc, err := t.asFile(ctx, file, sr)
	if err != nil {
		r.fail(err)
		return
	}
	r.complete(desc)
}

// A compilation task. The executor has a semaphore that limits the number
// of concurrent, running tasks.
type task struct {
	e        *executor
	// If true, this task needs to acquire a semaphore permit before running.
	// If false, this task needs to release its semaphore permit on completion.
	released bool
}

func (t *task) release() {
	if !t.released {
		t.e.s.Release(1)
		t.released = true
	}
}

func (t *task) asFile(ctx context.Context, name string, r SearchResult) (linker.File, error) {
	if r.Desc != nil {
		if r.Desc.Path() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.Desc.Path())
		}
		return linker.NewFileRecursive(r.Desc)
	}

	parseRes, err := t.asParseResult(name, r)
	if err != nil {
		return nil, err
	}

	var deps []linker.File
	if len(parseRes.Proto().Dependency) > 0 {
		results := make([]*result, len(parseRes.Proto().Dependency))
		for i, dep := range parseRes.Proto().Dependency {
			results[i] = t.e.compile(ctx, dep)
		}
		deps = make([]linker.File, len(results))

		// release our semaphore so dependencies can be processed w/out risk of deadlock
		t.e.s.Release(1)
		t.released = true

		// now we wait for them all to be computed
		for i, res := range results {
			select {
			case <-res.ready:
				if res.err != nil {
					return nil, res.err
				}
				deps[i] = res.res
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// all deps resolved; reacquire semaphore so we can proceed
		if err := t.e.s.Acquire(ctx, 1); err != nil {
			return nil, err
		}
		t.released = false
	}

	return t.link(parseRes, deps)
}

func (t *task) link(parseRes parser.Result, deps linker.Files) (linker.File, error) {
	file, err := linker.Link(parseRes, deps, t.e.sym, t.e.h)
	if err != nil {
		return nil, err
	}
	optsIndex, err := options.InterpretOptions(file, t.e.h)
	if err != nil {
		return nil, err
	}
	if t.e.c.IncludeSourceInfo && parseRes.AST() != nil {
		parseRes.Proto().SourceCodeInfo = sourceinfo.GenerateSourceInfo(parseRes.AST(), optsIndex)
	}
	return file, nil
}

func (t *task) asParseResult(name string, r SearchResult) (parser.Result, error) {
	if r.Proto != nil {
		if r.Proto.GetName() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.Proto.GetName())
		}
		return parser.ResultWithoutAST(r.Proto), nil
	}

	file, err := t.asAST(name, r)
	if err != nil {
		return nil, err
	}

	return parser.ResultFromAST(file, true, t.e.h)
}

func (t *task) asAST(name string, r SearchResult) (*ast.FileNode, error) {
	if r.AST != nil {
		if r.AST.Start().Filename != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.AST.Start().Filename)
		}
		return r.AST, nil
	}

	return parser.Parse(name, r.Source, t.e.h)
}
