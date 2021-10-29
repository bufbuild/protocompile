package protocompile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
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

	// We lock now and create all tasks under lock to make sure that no
	// async task can create a duplicate result. For example, if files
	// contains both "foo.proto" and "bar.proto", then there is a race
	// after we start compiling "foo.proto" between this loop and the
	// async compilation task to create the result for "bar.proto". But
	// we need to know if the file is directly requested for compilation,
	// so we need this loop to define the result. So this loop holds the
	// lock the whole time so async tasks can't create a result first.
	results := make([]*result, len(files))
	func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for i, f := range files {
			results[i] = e.compileLocked(ctx, f, true)
		}
	}()

	descs := make([]linker.File, len(files))
	var firstError error
	for i, r := range results {
		select {
		case <-r.ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if r.err != nil {
			fmt.Printf("got error for %q: %v\n", r.name, r.err)
			if firstError == nil {
				firstError = r.err
			}
		}
		descs[i] = r.res
	}

	if err := h.Error(); err != nil {
		return descs, err
	}
	// this should probably never happen; if any task returned an
	// error, h.Error() should be non-nil
	return descs, firstError
}

type result struct {
	name  string
	ready chan struct{}

	// true if this file was explicitly provided to the compiler; otherwise
	// this file is an import that is implicitly included
	explicitFile bool

	// produces a linker.File or error, only available when ready is closed
	res linker.File
	err error

	mu sync.Mutex
	// the results that are dependencies of this result; this result is
	// blocked, waiting on these dependencies to complete
	blockedOn []string
}

func (r *result) fail(err error) {
	r.err = err
	close(r.ready)
}

func (r *result) complete(f linker.File) {
	r.res = f
	close(r.ready)
}

func (r *result) setBlockedOn(deps []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedOn = deps
}

func (r *result) getBlockedOn() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.blockedOn
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

	return e.compileLocked(ctx, file, false)
}

func (e *executor) compileLocked(ctx context.Context, file string, explicitFile bool) *result {
	r := e.results[file]
	if r != nil {
		return r
	}

	r = &result{
		name:         file,
		ready:        make(chan struct{}),
		explicitFile: explicitFile,
	}
	e.results[file] = r
	go func() {
		e.doCompile(ctx, file, r)
	}()
	return r
}

type errFailedToResolve struct {
	err  error
	path string
}

func (e errFailedToResolve) Error() string {
	errMsg := e.err.Error()
	if strings.Contains(errMsg, e.path) {
		// underlying error already refers to path in question, so we don't need to add more context
		return errMsg
	}
	return fmt.Sprintf("could not resolve path %q: %s", e.path, e.err.Error())
}

func (e errFailedToResolve) Unwrap() error {
	return e.err
}

func (e *executor) doCompile(ctx context.Context, file string, r *result) {
	t := task{e: e, h: e.h.SubHandler(), r: r}
	if err := e.s.Acquire(ctx, 1); err != nil {
		r.fail(err)
		return
	}
	defer t.release()

	sr, err := e.c.Resolver.FindFileByPath(file)
	if err != nil {
		r.fail(errFailedToResolve{err, file})
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
	e *executor

	// handler for this task
	h *reporter.Handler

	// If true, this task needs to acquire a semaphore permit before running.
	// If false, this task needs to release its semaphore permit on completion.
	released bool

	// the result that is populated by this task
	r *result
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
		t.r.setBlockedOn(parseRes.Proto().Dependency)

		results := make([]*result, len(parseRes.Proto().Dependency))
		checked := map[string]struct{}{}
		for i, dep := range parseRes.Proto().Dependency {
			pos := findImportPos(parseRes, dep)
			if name == dep {
				// doh! file imports itself
				handleImportCycle(t.h, pos, []string{name}, dep)
				return nil, t.h.Error()
			}

			res := t.e.compile(ctx, dep)
			// check for dependency cycle to prevent deadlock
			if err := t.e.checkForDependencyCycle(res, []string{name, dep}, pos, checked); err != nil {
				return nil, err
			}
			results[i] = res
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
					if rerr, ok := res.err.(errFailedToResolve); ok {
						// We don't report errors to get file from resolver to handler since
						// it's usually considered immediately fatal. However, if the reason
						// we were resolving is due to an import, turn this into an error with
						// source position that pinpoints the import statement and report it.
						return nil, reporter.Error(findImportPos(parseRes, res.name), rerr)
					}
					return nil, res.err
				}
				deps[i] = res.res
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// all deps resolved
		t.r.setBlockedOn(nil)
		// reacquire semaphore so we can proceed
		if err := t.e.s.Acquire(ctx, 1); err != nil {
			return nil, err
		}
		t.released = false
	}

	return t.link(parseRes, deps)
}

func (e *executor) checkForDependencyCycle(res *result, sequence []string, pos ast.SourcePos, checked map[string]struct{}) error {
	if _, ok := checked[res.name]; ok {
		// already checked this one
		return nil
	}
	checked[res.name] = struct{}{}
	deps := res.getBlockedOn()
	for _, dep := range deps {
		// is this a cycle?
		for _, file := range sequence {
			if file == dep {
				handleImportCycle(e.h, pos, sequence, dep)
				return e.h.Error()
			}
		}

		e.mu.Lock()
		depRes := e.results[dep]
		e.mu.Unlock()
		if depRes == nil {
			continue
		}
		if err := e.checkForDependencyCycle(depRes, append(sequence, dep), pos, checked); err != nil {
			return err
		}
	}
	return nil
}

func handleImportCycle(h *reporter.Handler, pos ast.SourcePos, importSequence []string, dep string) {
	var buf bytes.Buffer
	buf.WriteString("cycle found in imports: ")
	for _, imp := range importSequence {
		fmt.Fprintf(&buf, "%q -> ", imp)
	}
	fmt.Fprintf(&buf, "%q", dep)
	h.HandleErrorf(pos, buf.String())
}

func findImportPos(res parser.Result, dep string) ast.SourcePos {
	root := res.AST()
	if root == nil {
		return ast.UnknownPos(res.FileNode().Name())
	}
	for _, decl := range root.Decls {
		if imp, ok := decl.(*ast.ImportNode); ok {
			if imp.Name.AsString() == dep {
				return root.NodeInfo(imp.Name).Start()
			}
		}
	}
	// this should never happen...
	return ast.UnknownPos(res.FileNode().Name())
}

func (t *task) link(parseRes parser.Result, deps linker.Files) (linker.File, error) {
	file, err := linker.Link(parseRes, deps, t.e.sym, t.h)
	if err != nil {
		return nil, err
	}
	optsIndex, err := options.InterpretOptions(file, t.h)
	if err != nil {
		return nil, err
	}
	// now that options are interpreted, we can do some additional checks
	if err := file.ValidateExtensions(t.h); err != nil {
		return nil, err
	}
	if t.r.explicitFile {
		file.CheckForUnusedImports(t.h)
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

	return parser.ResultFromAST(file, true, t.h)
}

func (t *task) asAST(name string, r SearchResult) (*ast.FileNode, error) {
	if r.AST != nil {
		if r.AST.Name() != name {
			return nil, fmt.Errorf("search result for %q returned descriptor for %q", name, r.AST.Name())
		}
		return r.AST, nil
	}

	return parser.Parse(name, r.Source, t.h)
}
