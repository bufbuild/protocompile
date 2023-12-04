// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package benchmarks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal/protoc"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/parser/fastscan"
	"github.com/bufbuild/protocompile/protoutil"
	"github.com/bufbuild/protocompile/reporter"
)

const (
	googleapisCommit = "cb6fbe8784479b22af38c09a5039d8983e894566"
)

var (
	protocPath string

	skipDownload = os.Getenv("SKIP_DOWNLOAD_GOOGLEAPIS") == "true"

	googleapisURI     = fmt.Sprintf("https://github.com/googleapis/googleapis/archive/%s.tar.gz", googleapisCommit)
	googleapisDir     string
	googleapisSources []string
)

func TestMain(m *testing.M) {
	var err error
	protocPath, err = protoc.BinaryPath("../../")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to compute protoc path: %v\n", err)
		os.Exit(1)
	}
	if info, err := os.Stat(protocPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			_, _ = fmt.Fprintf(os.Stderr, "Path %s not found. Run `make generate` in the project root first.\n", protocPath)
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "Error querying for path %s: %v\n", protocPath, err)
		}
		os.Exit(1)
	} else if info.IsDir() {
		_, _ = fmt.Fprintf(os.Stderr, "Path %s is a directory but expecting an executable file.\n", protocPath)
		os.Exit(1)
	}

	var stat int
	defer func() {
		os.Exit(stat)
	}()
	// After this point, we can set stat and return instead of directly calling os.Exit.
	// That allows deferred functions to execute, to perform cleanup, before exiting.

	if !skipDownload {
		dir, err := os.MkdirTemp("", "testdownloads")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Could not create temporary directory: %v\n", err)
			stat = 1
			return
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Failed to cleanup temp directory %s: %v\n", dir, err)
			}
		}()

		if err := downloadAndExpand(googleapisURI, dir); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to download and expand googleapis: %v\n", err)
			stat = 1
			return
		}

		googleapisDir = filepath.Join(dir, fmt.Sprintf("googleapis-%s", googleapisCommit)) + "/"
		var sourceSize int64
		err = filepath.Walk(googleapisDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".proto") {
				relPath := strings.TrimPrefix(path, googleapisDir)
				googleapisSources = append(googleapisSources, relPath)
				sourceSize += info.Size()
			}
			return nil
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to enumerate googleapis source files: %v\n", err)
			stat = 1
			return
		}
		sort.Strings(googleapisSources)
		fmt.Printf("%d total source files found in googleapis (%d bytes).\n", len(googleapisSources), sourceSize)
	}

	stat = m.Run()
}

func downloadAndExpand(url, targetDir string) (e error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer func() {
			if err = resp.Body.Close(); err != nil && e == nil {
				e = err
			}
		}()
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s resulted in status code %s", url, resp.Status)
	}
	if err := os.MkdirAll(targetDir, 0777); err != nil {
		return err
	}
	f, err := os.CreateTemp(targetDir, "testdownload.*.tar.gz")
	if err != nil {
		return err
	}
	defer func() {
		if f != nil {
			if err := f.Close(); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", f.Name(), err)
			}
		}
	}()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Downloaded %v; %d bytes (%v).\n", url, n, time.Since(start))
	archiveName := f.Name()
	if err := f.Close(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", f.Name(), err)
	}
	f = nil

	f, err = os.OpenFile(archiveName, os.O_RDONLY, 0)
	if err != nil {
		return err
	}

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		if err = gzr.Close(); err != nil && e == nil {
			e = err
		}
	}()

	tr := tar.NewReader(gzr)
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr == nil {
			continue
		}
		target := filepath.Join(targetDir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0777); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
			count++
		default:
			// skip anything else
		}
	}
	fmt.Printf("Expanded archive into %d files.\n", count)

	return nil
}

func BenchmarkGoogleapisProtocompile(b *testing.B) {
	benchmarkGoogleapisProtocompile(b, false, func() *protocompile.Compiler {
		return &protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				ImportPaths: []string{googleapisDir},
			}),
			SourceInfoMode: protocompile.SourceInfoExtraComments,
			// leave MaxParallelism unset to let it use all cores available
		}
	})
}

func BenchmarkGoogleapisProtocompileCanonical(b *testing.B) {
	benchmarkGoogleapisProtocompile(b, true, func() *protocompile.Compiler {
		return &protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				ImportPaths: []string{googleapisDir},
			}),
			SourceInfoMode: protocompile.SourceInfoStandard,
			// leave MaxParallelism unset to let it use all cores available
		}
	})
}

func BenchmarkGoogleapisProtocompileNoSourceInfo(b *testing.B) {
	benchmarkGoogleapisProtocompile(b, false, func() *protocompile.Compiler {
		return &protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				ImportPaths: []string{googleapisDir},
			}),
			SourceInfoMode: protocompile.SourceInfoNone,
			// leave MaxParallelism unset to let it use all cores available
		}
	})
}

func benchmarkGoogleapisProtocompile(b *testing.B, canonicalBytes bool, factory func() *protocompile.Compiler) {
	for i := 0; i < b.N; i++ {
		benchmarkProtocompile(b, factory(), googleapisSources, canonicalBytes)
	}
}

func benchmarkProtocompile(b *testing.B, c *protocompile.Compiler, sources []string, canonicalBytes bool) {
	fds, err := c.Compile(context.Background(), sources...)
	require.NoError(b, err)
	var fdSet descriptorpb.FileDescriptorSet
	fdSet.File = make([]*descriptorpb.FileDescriptorProto, len(fds))
	for i, fd := range fds {
		if canonicalBytes {
			fdSet.File[i] = fd.(linker.Result).CanonicalProto()
		} else {
			fdSet.File[i] = protoutil.ProtoFromFileDescriptor(fd)
		}
	}
	// protoc is writing output to file descriptor set, so we should, too
	writeToNull(b, &fdSet)
}

func BenchmarkGoogleapisProtoparse(b *testing.B) {
	benchmarkGoogleapisProtoparse(b, func() *protoparse.Parser {
		return &protoparse.Parser{
			ImportPaths:           []string{googleapisDir},
			IncludeSourceCodeInfo: true,
		}
	})
}

func BenchmarkGoogleapisProtoparseNoSourceInfo(b *testing.B) {
	benchmarkGoogleapisProtoparse(b, func() *protoparse.Parser {
		return &protoparse.Parser{
			ImportPaths:           []string{googleapisDir},
			IncludeSourceCodeInfo: false,
		}
	})
}

func benchmarkGoogleapisProtoparse(b *testing.B, factory func() *protoparse.Parser) {
	par := runtime.GOMAXPROCS(-1)
	cpus := runtime.NumCPU()
	if par > cpus {
		par = cpus
	}
	for i := 0; i < b.N; i++ {
		// Buf currently batches files into chunks and then runs all chunks in parallel
		chunks := make([][]string, par)
		j := 0
		total := 0
		for ch := 0; ch < par; ch++ {
			chunkStart := j
			chunkEnd := (ch + 1) * len(googleapisSources) / par
			chunks[ch] = googleapisSources[chunkStart:chunkEnd]
			j = chunkEnd
			total += len(chunks[ch])
		}
		require.Len(b, googleapisSources, total)
		var wg sync.WaitGroup
		results := make([][]*desc.FileDescriptor, par)
		errors := make([]error, par)
		for ch, chunk := range chunks {
			ch, chunk := ch, chunk
			wg.Add(1)
			go func() {
				defer wg.Done()
				p := factory()
				results[ch], errors[ch] = p.ParseFiles(chunk...)
			}()
		}
		wg.Wait()
		for _, err := range errors {
			require.NoError(b, err)
		}
		var fdSet descriptorpb.FileDescriptorSet
		fdSet.File = make([]*descriptorpb.FileDescriptorProto, 0, len(googleapisSources))
		for _, chunk := range results {
			for _, fd := range chunk {
				fdSet.File = append(fdSet.File, fd.AsFileDescriptorProto())
			}
		}
		writeToNull(b, &fdSet)
	}
}

func BenchmarkGoogleapisFastScan(b *testing.B) {
	par := runtime.GOMAXPROCS(-1)
	cpus := runtime.NumCPU()
	if par > cpus {
		par = cpus
	}
	type entry struct {
		filename   string
		scanResult fastscan.Result
	}
	for i := 0; i < b.N; i++ {
		workCh := make(chan string, par)
		resultsCh := make(chan entry, par)
		grp, ctx := errgroup.WithContext(context.Background())
		// producer
		grp.Go(func() error {
			defer close(workCh)
			for _, name := range googleapisSources {
				select {
				case workCh <- filepath.Join(googleapisDir, name):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
		var numProcs atomic.Int32
		numProcs.Store(int32(par))
		for i := 0; i < par; i++ {
			// consumers/processors
			grp.Go(func() error {
				defer func() {
					if numProcs.Add(-1) == 0 {
						// last one to leave closes the channel
						close(resultsCh)
					}
				}()
				for {
					var filename string
					select {
					case name, ok := <-workCh:
						if !ok {
							return nil
						}
						filename = name
					case <-ctx.Done():
						return ctx.Err()
					}
					r, err := os.Open(filename)
					if err != nil {
						return err
					}
					res, err := fastscan.Scan(filename, r)
					_ = r.Close()
					if err != nil {
						return err
					}
					select {
					case resultsCh <- entry{filename: filename, scanResult: res}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			})
		}
		results := make(map[string]fastscan.Result, len(googleapisSources))
		grp.Go(func() error {
			// accumulator
			for {
				select {
				case entry, ok := <-resultsCh:
					if !ok {
						return nil
					}
					results[entry.filename] = entry.scanResult
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})

		err := grp.Wait()
		require.NoError(b, err)
	}
}

func BenchmarkGoogleapisProtoc(b *testing.B) {
	benchmarkGoogleapisProtoc(b, "--include_source_info")
}

func BenchmarkGoogleapisProtocNoSourceInfo(b *testing.B) {
	benchmarkGoogleapisProtoc(b)
}

func benchmarkGoogleapisProtoc(b *testing.B, extraArgs ...string) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			args := make([]string, 0, len(googleapisSources)+5)
			args = append(args, "-I", googleapisDir, "-o", os.DevNull)
			args = append(args, extraArgs...)
			args = append(args, googleapisSources...)
			cmd := exec.Command(protocPath, args...)
			cmd.Stdin = nil
			cmd.Stdout = nil
			var errBuffer bytes.Buffer
			cmd.Stderr = &errBuffer

			err := cmd.Run()
			if err != nil {
				_, _ = os.Stderr.Write(errBuffer.Bytes())
				b.Fatalf("failed to invoke protoc: %v", err)
			}
		}
	})
}

func BenchmarkGoogleapisProtocompileSingleThreaded(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c := &protocompile.Compiler{
				Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
					ImportPaths: []string{googleapisDir},
				}),
				SourceInfoMode: protocompile.SourceInfoExtraComments,
				// to really test performance compared to protoc and protoparse, we
				// need to run a single-threaded compile
				MaxParallelism: 1,
			}
			benchmarkProtocompile(b, c, googleapisSources, false)
		}
	})
}

func BenchmarkGoogleapisProtoparseSingleThreaded(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p := protoparse.Parser{
				ImportPaths:           []string{googleapisDir},
				IncludeSourceCodeInfo: true,
			}
			fds, err := p.ParseFiles(googleapisSources...)
			require.NoError(b, err)
			var fdSet descriptorpb.FileDescriptorSet
			fdSet.File = make([]*descriptorpb.FileDescriptorProto, len(fds))
			for i, fd := range fds {
				fdSet.File[i] = fd.AsFileDescriptorProto()
			}
			writeToNull(b, &fdSet)
		}
	})
}

func writeToNull(b *testing.B, fds *descriptorpb.FileDescriptorSet) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("failed to open output file %s: %v", os.DevNull, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", f.Name(), err)
		}
	}()
	data, err := proto.Marshal(fds)
	if err != nil {
		b.Fatalf("failed to marshal file descriptor set: %v", err)
	}
	_, err = f.Write(data)
	if err != nil {
		b.Fatalf("failed to write file descriptor set to file: %v", err)
	}
}

func TestGoogleapisProtocompileResultMemory(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{googleapisDir},
		}),
		SourceInfoMode: protocompile.SourceInfoExtraComments,
	}
	fds, err := c.Compile(context.Background(), googleapisSources...)
	require.NoError(t, err)
	measure(t, fds)
}

func TestGoogleapisProtocompileResultMemoryNoSourceInfo(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: []string{googleapisDir},
		}),
		SourceInfoMode: protocompile.SourceInfoNone,
	}
	fds, err := c.Compile(context.Background(), googleapisSources...)
	require.NoError(t, err)
	measure(t, fds)
}

func TestGoogleapisProtocompileASTMemory(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	var asts []*ast.FileNode
	for _, file := range googleapisSources {
		func() {
			f, err := os.OpenFile(filepath.Join(googleapisDir, file), os.O_RDONLY, 0)
			require.NoError(t, err)
			defer func() {
				if err := f.Close(); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", f.Name(), err)
				}
			}()
			h := reporter.NewHandler(nil)
			ast, err := parser.Parse(file, f, h)
			require.NoError(t, err)
			asts = append(asts, ast)
		}()
	}
	measure(t, asts)
}

func TestGoogleapisProtoparseResultMemory(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	p := protoparse.Parser{
		ImportPaths:           []string{googleapisDir},
		IncludeSourceCodeInfo: true,
	}
	fds, err := p.ParseFiles(googleapisSources...)
	require.NoError(t, err)
	measure(t, fds)
}

func TestGoogleapisProtoparseResultMemoryNoSourceInfo(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	p := protoparse.Parser{
		ImportPaths:           []string{googleapisDir},
		IncludeSourceCodeInfo: false,
	}
	fds, err := p.ParseFiles(googleapisSources...)
	require.NoError(t, err)
	measure(t, fds)
}

func TestGoogleapisProtoparseASTMemory(t *testing.T) {
	if skipDownload {
		t.Skip()
	}
	p := protoparse.Parser{
		IncludeSourceCodeInfo: true,
	}
	// NB: ParseToAST fails to respect import paths, so we have to pass full names
	filenames := make([]string, len(googleapisSources))
	for i := range googleapisSources {
		filenames[i] = filepath.Join(googleapisDir, googleapisSources[i])
	}
	asts, err := p.ParseToAST(filenames...)
	require.NoError(t, err)
	measure(t, asts)
}

func measure(t *testing.T, v any) {
	// log heap allocations
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("(heap used: %d bytes)", m.Alloc)

	// and then try to directly measure just the given value
	mt := newMeasuringTape()
	mt.measure(reflect.ValueOf(v))
	t.Logf("memory used: %d bytes", mt.memoryUsed())
}
