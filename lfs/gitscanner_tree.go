package lfs

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"

	"github.com/git-lfs/git-lfs/config"
	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/filepathfilter"
	"github.com/git-lfs/git-lfs/git"
	"github.com/git-lfs/git-lfs/git/gitattr"
)

func runScanTree(cb GitScannerFoundPointer, ref string, filter *filepathfilter.Filter, gitEnv, osEnv config.Environment) error {
	// We don't use the nameMap approach here since that's imprecise when >1 file
	// can be using the same content
	treeShas, err := lsTreeBlobs(ref, func(t *git.TreeBlob) bool {
		return t != nil && t.Size < blobSizeCutoff && filter.Allows(t.Filename)
	})
	if err != nil {
		return err
	}

	pcw, err := catFileBatchTree(treeShas, gitEnv, osEnv)
	if err != nil {
		return err
	}

	for p := range pcw.Results {
		cb(p, nil)
	}

	if err := pcw.Wait(); err != nil {
		cb(nil, err)
	}
	return nil
}

// catFileBatchTree uses git cat-file --batch to get the object contents
// of a git object, given its sha1. The contents will be decoded into
// a Git LFS pointer. treeblobs is a channel over which blob entries
// will be sent. It returns a channel from which point.Pointers can be read.
func catFileBatchTree(treeblobs *TreeBlobChannelWrapper, gitEnv, osEnv config.Environment) (*PointerChannelWrapper, error) {
	scanner, err := NewPointerScanner(gitEnv, osEnv)
	if err != nil {
		return nil, err
	}

	pointers := make(chan *WrappedPointer, chanBufSize)
	errchan := make(chan error, 10) // Multiple errors possible

	go func() {
		hasNext := true
		for t := range treeblobs.Results {
			hasNext = scanner.Scan(t.Oid)

			if p := scanner.Pointer(); p != nil {
				p.Name = t.Filename
				pointers <- p
			}

			if err := scanner.Err(); err != nil {
				errchan <- err
			}

			if !hasNext {
				break
			}
		}

		// If the scanner quit early, we may still have treeblobs to
		// read, so waiting for it to close will cause a deadlock.
		if hasNext {
			// Deal with nested error from incoming treeblobs
			err := treeblobs.Wait()
			if err != nil {
				errchan <- err
			}
		}

		if err = scanner.Close(); err != nil {
			errchan <- err
		}

		close(pointers)
		close(errchan)
	}()

	return NewPointerChannelWrapper(pointers, errchan), nil
}

// Use ls-tree at ref to find a list of candidate tree blobs which might be lfs files
// The returned channel will be sent these blobs which should be sent to catFileBatchTree
// for final check & conversion to Pointer
func lsTreeBlobs(ref string, predicate func(*git.TreeBlob) bool) (*TreeBlobChannelWrapper, error) {
	cmd, err := git.LsTree(ref)
	if err != nil {
		return nil, err
	}

	cmd.Stdin.Close()

	blobs := make(chan git.TreeBlob, chanBufSize)
	errchan := make(chan error, 1)

	go func() {
		scanner := git.NewLsTreeScanner(cmd.Stdout)
		for scanner.Scan() {
			if t := scanner.TreeBlob(); predicate(t) {
				blobs <- *t
			}
		}

		stderr, _ := ioutil.ReadAll(cmd.Stderr)
		err := cmd.Wait()
		if err != nil {
			errchan <- fmt.Errorf("error in git ls-tree: %v %v", err, string(stderr))
		}
		close(blobs)
		close(errchan)
	}()

	return NewTreeBlobChannelWrapper(blobs, errchan), nil
}

func catFileBatchTreeForPointers(treeblobs *TreeBlobChannelWrapper, gitEnv, osEnv config.Environment) (map[string]*WrappedPointer, *filepathfilter.Filter, error) {
	pscanner, err := NewPointerScanner(gitEnv, osEnv)
	if err != nil {
		return nil, nil, err
	}
	oscanner, err := git.NewObjectScanner(gitEnv, osEnv)
	if err != nil {
		return nil, nil, err
	}

	pointers := make(map[string]*WrappedPointer)

	paths := make([]git.AttributePath, 0)
	processor := gitattr.NewMacroProcessor()

	hasNext := true
	for t := range treeblobs.Results {
		if path.Base(t.Filename) == ".gitattributes" {
			hasNext = oscanner.Scan(t.Oid)

			if rdr := oscanner.Contents(); rdr != nil {
				paths = append(paths, git.AttrPathsFromReader(
					processor,
					t.Filename,
					"",
					rdr,
					t.Filename == ".gitattributes", // Read macros from the top-level attributes
				)...)
			}

			if err := oscanner.Err(); err != nil {
				return nil, nil, err
			}
		} else if t.Size < blobSizeCutoff {
			hasNext = pscanner.Scan(t.Oid)

			// It's intentional that we insert nil for
			// non-pointers; we want to keep track of them
			// as well as pointers.
			p := pscanner.Pointer()
			if p != nil {
				p.Name = t.Filename
			}
			pointers[t.Filename] = p

			if err := pscanner.Err(); err != nil {
				return nil, nil, err
			}
		} else {
			pointers[t.Filename] = nil
		}

		if !hasNext {
			break
		}
	}

	// If the scanner quit early, we may still have treeblobs to
	// read, so waiting for it to close will cause a deadlock.
	if hasNext {
		// Deal with nested error from incoming treeblobs
		err := treeblobs.Wait()
		if err != nil {
			return nil, nil, err
		}
	}

	if err = pscanner.Close(); err != nil {
		return nil, nil, err
	}
	if err = oscanner.Close(); err != nil {
		return nil, nil, err
	}

	patterns := make([]filepathfilter.Pattern, 0, len(paths))
	for _, path := range paths {
		// Convert all separators to `/` before creating a pattern to
		// avoid characters being escaped in situations like `subtree\*.md`
		patterns = append(patterns, filepathfilter.NewPattern(filepath.ToSlash(path.Path)))
	}

	return pointers, filepathfilter.NewFromPatterns(patterns, nil), nil
}

func runScanTreeForPointers(cb GitScannerFoundPointer, tree string, gitEnv, osEnv config.Environment) error {
	treeShas, err := lsTreeBlobs(tree, func(t *git.TreeBlob) bool {
		return t != nil
	})
	if err != nil {
		return err
	}

	pointers, filter, err := catFileBatchTreeForPointers(treeShas, gitEnv, osEnv)
	if err != nil {
		return err
	}

	for name, p := range pointers {
		// This file matches the patterns in .gitattributes, so it
		// should be a pointer.  If it is not, then it is a plain Git
		// blob, which we report as an error.
		if filter.Allows(name) {
			if p == nil {
				cb(nil, errors.NewPointerScanError(errors.NewNotAPointerError(nil), tree, name))
			} else {
				cb(p, nil)
			}
		}
	}
	return nil
}
