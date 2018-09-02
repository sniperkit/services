/*
Sniperkit-Bot
- Status: analyzed
*/

// Sniperkit - 2018
// Status: Analyzed

package session

import (
	"fmt"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gopherjs/gopherjs/compiler"
	"github.com/sniperkit/snk.fork.services"
	"gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
)

func New(tags []string, root billy.Filesystem, assetsArchives map[string]map[bool]*compiler.Archive, fileserver services.Fileserver, configValidExtensions []string) *Session {
	s := &Session{}
	s.tags = append([]string{"js", "netgo", "purego", "jsgo"}, tags...)
	s.pathfs = memfs.New()
	s.rootfs = root
	s.source = map[string]map[string]string{}
	s.sourcefs = memfs.New()
	s.configValidExtensions = configValidExtensions
	s.Fileserver = fileserver
	s.AssetsArchives = assetsArchives
	return s
}

type Session struct {
	// build tags
	tags []string

	// File system for uploaded source code. Used in preference to rootfs and pathfs (should always be read-only)
	sourcefs billy.Filesystem

	// File system for GOPATH (getter will write to this)
	pathfs billy.Filesystem

	// File system for GOROOT, defaults to assets.Assets (should always be read-only)
	rootfs billy.Filesystem

	// Map of uploaded source files: package path => filename => contents
	source map[string]map[string]string

	// Valid file extensions
	configValidExtensions []string

	// Fileserver for storing and serving generated artifacts
	Fileserver services.Fileserver

	// AssetsArchives are the pre-compiled standard library archives created in the generation process...
	// the format is map[path]map[minified]*compiler.Archive
	AssetsArchives map[string]map[bool]*compiler.Archive
}

func (s *Session) SetSource(source map[string]map[string]string) error {
	s.source = source
	s.sourcefs = memfs.New()
	for path, files := range source {
		if err := s.createPackage(s.sourcefs, filepath.Join("gopath", "src", path), files); err != nil {
			return err
		}
	}
	return nil
}

type BuildType int

const (
	DefaultType BuildType = iota
	JsType
)

func (s *Session) BuildContext(buildType BuildType, suffix string) *build.Context {
	var goarch, goos string
	var cgo bool
	switch buildType {
	case DefaultType:
		goarch = "amd64"
		goos = "darwin"
		cgo = false
	case JsType:
		goarch = "js"
		goos = "darwin"
		cgo = true
	}
	b := &build.Context{
		GOARCH:        goarch,   // Target architecture
		GOOS:          goos,     // Target operating system
		GOROOT:        "goroot", // Go root
		GOPATH:        "gopath", // Go path
		InstallSuffix: suffix,   // Builder only: "min" or "".
		Compiler:      "gc",     // Compiler to assume when computing target paths
		BuildTags:     s.tags,   // Build tags
		CgoEnabled:    cgo,      // Builder only: detect `import "C"` to throw proper error
		ReleaseTags:   build.Default.ReleaseTags,

		// IsDir reports whether the path names a directory.
		// If IsDir is nil, Import calls os.Stat and uses the result's IsDir method.
		IsDir: func(path string) bool {
			fs := s.Filesystem(path)
			fi, err := fs.Stat(path)
			return err == nil && fi.IsDir()
		},

		// HasSubdir reports whether dir is lexically a subdirectory of
		// root, perhaps multiple levels below. It does not try to check
		// whether dir exists.
		// If so, HasSubdir sets rel to a slash-separated path that
		// can be joined to root to produce a path equivalent to dir.
		// If HasSubdir is nil, Import uses an implementation built on
		// filepath.EvalSymlinks.
		HasSubdir: func(root, dir string) (rel string, ok bool) {
			// copied from default implementation to prevent use of filepath.EvalSymlinks
			const sep = string(filepath.Separator)
			root = filepath.Clean(root)
			if !strings.HasSuffix(root, sep) {
				root += sep
			}
			dir = filepath.Clean(dir)
			if !strings.HasPrefix(dir, root) {
				return "", false
			}
			return filepath.ToSlash(dir[len(root):]), true
		},

		// ReadDir returns a slice of os.FileInfo, sorted by Name,
		// describing the content of the named directory.
		// If ReadDir is nil, Import uses ioutil.ReadDir.
		ReadDir: func(path string) ([]os.FileInfo, error) {
			fs := s.Filesystem(path)
			return fs.ReadDir(path)
		},

		// OpenFile opens a file (not a directory) for reading.
		// If OpenFile is nil, Import uses os.Open.
		OpenFile: func(path string) (io.ReadCloser, error) {
			dir, _ := filepath.Split(path)
			fs := s.Filesystem(dir)
			return fs.Open(path)
		},
	}
	return b
}

func (s *Session) HasSource(path string) bool {
	return s.source[path] != nil
}

func (s *Session) GoPath() billy.Filesystem {
	return s.pathfs
}

// Gets either sourcefs, rootfs or pathfs depending on the path, and if the package is part of source
func (s *Session) Filesystem(dir string) billy.Filesystem {

	dir = strings.Trim(filepath.Clean(dir), string(filepath.Separator))
	parts := strings.Split(dir, string(filepath.Separator))

	if len(parts) > 2 {
		// If the package is in the source collection, return sourcefs (for both requests for gopath
		// and also goroot)
		pkg := strings.Join(parts[2:], "/")
		if s.source[pkg] != nil {
			return s.sourcefs
		}
	}

	switch parts[0] {
	case "gopath":
		return s.pathfs
	case "goroot":
		return s.rootfs
	}

	panic(fmt.Sprintf("Top dir should be goroot or gopath, got %s", dir))
}

func (s *Session) createPackage(fs billy.Filesystem, dir string, files map[string]string) error {
	if err := fs.MkdirAll(dir, 0777); err != nil {
		return err
	}
	for name, contents := range files {
		if !s.isValidFile(name) {
			continue
		}
		if err := s.createFile(fs, dir, name, contents); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) isValidFile(name string) bool {
	if len(s.configValidExtensions) == 0 {
		panic("configValidExtensions not specified")
	}
	for _, ext := range s.configValidExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func (s *Session) createFile(fs billy.Filesystem, dir, name, contents string) error {
	file, err := fs.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte(contents)); err != nil {
		return err
	}
	return nil
}
