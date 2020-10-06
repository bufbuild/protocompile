package protocompile

import (
	"context"
	"io"
	"runtime"
	"sync"

	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/linker"
	"github.com/jhump/protocompile/options"
	"github.com/jhump/protocompile/parser"
	"github.com/jhump/protocompile/reporter"
	"github.com/jhump/protocompile/sourceinfo"
)

type Compiler struct {
	Resolver       Resolver
	MaxParallelism int
	Reporter       reporter.Reporter

	IncludeSourceInfo bool
}

func (c *Compiler) Compile(ctx context.Context, files ...string) ([]protoreflect.FileDescriptor, error) {
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

	descs := make([]protoreflect.FileDescriptor, len(files))
	for i, r := range results {
		select {
		case <- r.ready:
		case <- ctx.Done():
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

type task struct {
	e        *executor
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
			case <- res.ready:
				if res.err != nil {
					return nil, res.err
				}
				deps[i] = res.res
			case <- ctx.Done():
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
	optsIndex, err := options.InterpretOptions(false, file, t.e.h)
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
		return parser.ResultWithoutAST(r.Proto), nil
	}

	file, err := t.asAST(name, r)
	if err != nil {
		return nil, err
	}

	return parser.ToFileDescriptorProto(name, file, true, t.e.h)
}

func (t *task) asAST(name string, r SearchResult) (*ast.FileNode, error) {
	if r.AST != nil {
		return r.AST, nil
	}

	return parser.Parse(name, r.Source, t.e.h)
}
