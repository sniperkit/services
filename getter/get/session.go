/*
Sniperkit-Bot
- Status: analyzed
*/

// Sniperkit - 2018
// Status: Analyzed

package get

import (
	"context"
	"fmt"
	"go/build"
	"io"
	"sync"

	"github.com/sniperkit/snk.fork.services"
	"github.com/sniperkit/snk.fork.services/getter/cache"
	"github.com/sniperkit/snk.fork.services/session"
	"golang.org/x/sync/singleflight"
)

type Getter struct {
	session           *session.Session
	gitreq            *cache.Request
	log               io.Writer
	send              func(services.Message)
	packageCache      map[string]*Package
	buildContext      *build.Context
	foldPath          map[string]string
	downloadCache     map[string]bool
	downloadRootCache map[string]*repoRoot
	repoPackages      map[string]*repoRoot
	fetchGroup        singleflight.Group
	fetchCacheMu      sync.Mutex
	fetchCache        map[string]fetchResult // key is metaImportsForPrefix's importPrefix
	Callback          func(path string, files map[string]string, standard bool) error
}

func New(sess *session.Session, send func(services.Message), cache *cache.Request) *Getter {
	g := &Getter{}
	g.gitreq = cache
	g.session = sess
	g.send = send
	g.packageCache = make(map[string]*Package)
	g.foldPath = make(map[string]string)
	g.downloadCache = make(map[string]bool)
	g.downloadRootCache = make(map[string]*repoRoot) // key is the root dir of the repo
	g.repoPackages = make(map[string]*repoRoot)      // key is the path of the package. NOTE: not all packages are included, but the ones we're interested in should be.
	g.fetchCache = make(map[string]fetchResult)
	g.buildContext = g.session.BuildContext(session.DefaultType, "")
	return g
}

func (g *Getter) Get(ctx context.Context, path string, update bool, insecure, single bool) error {
	var stk ImportStack
	if err := g.download(ctx, path, nil, &stk, update, insecure, single); err != nil {
		return err
	}
	if single {
		// don't build hints in single mode
		return nil
	}
	// after download, build a list of package path => dependency repo URLs for the gitcache hints
	hints := map[string][]string{}
	var processPath func(path string) []string
	processPath = func(path string) []string {

		if path == "C" {
			// Special case for "C" pseudo-import
			return nil
		}

		urls := map[string]bool{}

		p := g.packageCache[path]

		if p == nil {
			panic(fmt.Sprintf("package %s not in cache", path))
		}

		if p.Standard {
			return nil
		}

		if p != nil {
			for _, imp := range p.Imports {
				urlsForImport := processPath(imp)
				for _, url := range urlsForImport {
					urls[url] = true
				}
			}
		}

		repoForThisPath, ok := g.repoPackages[path]
		if ok {
			urls[repoForThisPath.repo] = true
		} else if p != nil {
			root, _ := g.vcsFromDir(p.Dir, p.Internal.Build.SrcRoot)
			// ignore error
			if root != nil {
				urls[root.repo] = true
			}
		}

		var urlsArray []string
		for url := range urls {
			urlsArray = append(urlsArray, url)
		}
		hints[path] = urlsArray
		return urlsArray
	}
	processPath(path)
	g.gitreq.SetHints(hints)
	return nil
}

// WithCancel executes the provided function, but returns early with true if the context cancellation
// signal was recieved.
func WithCancel(ctx context.Context, f func()) bool {
	finished := make(chan struct{})
	go func() {
		f()
		close(finished)
	}()
	select {
	case <-finished:
		return false
	case <-ctx.Done():
		return true
	}
}
