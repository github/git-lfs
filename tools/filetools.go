// Package tools contains other helper functions too small to justify their own package
// NOTE: Subject to change, do not rely on this package from outside git-lfs source
package tools

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/git-lfs/git-lfs/errors"
	"github.com/git-lfs/git-lfs/filepathfilter"
)

// FileOrDirExists determines if a file/dir exists, returns IsDir() results too.
func FileOrDirExists(path string) (exists bool, isDir bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, false
	} else {
		return true, fi.IsDir()
	}
}

// FileExists determines if a file (NOT dir) exists.
func FileExists(path string) bool {
	ret, isDir := FileOrDirExists(path)
	return ret && !isDir
}

// DirExists determines if a dir (NOT file) exists.
func DirExists(path string) bool {
	ret, isDir := FileOrDirExists(path)
	return ret && isDir
}

// FileExistsOfSize determines if a file exists and is of a specific size.
func FileExistsOfSize(path string, sz int64) bool {
	fi, err := os.Stat(path)

	if err != nil {
		return false
	}

	return !fi.IsDir() && fi.Size() == sz
}

// ResolveSymlinks ensures that if the path supplied is a symlink, it is
// resolved to the actual concrete path
func ResolveSymlinks(path string) string {
	if len(path) == 0 {
		return path
	}

	if resolved, err := CanonicalizeSystemPath(path); err == nil {
		return resolved
	}
	return path
}

// RenameFileCopyPermissions moves srcfile to destfile, replacing destfile if
// necessary and also copying the permissions of destfile if it already exists
func RenameFileCopyPermissions(srcfile, destfile string) error {
	info, err := os.Stat(destfile)
	if os.IsNotExist(err) {
		// no original file
	} else if err != nil {
		return err
	} else {
		if err := os.Chmod(srcfile, info.Mode()); err != nil {
			return fmt.Errorf("can't set filemode on file %q: %v", srcfile, err)
		}
	}

	if err := RobustRename(srcfile, destfile); err != nil {
		return fmt.Errorf("cannot replace %q with %q: %v", destfile, srcfile, err)
	}
	return nil
}

// CleanPaths splits the given `paths` argument by the delimiter argument, and
// then "cleans" that path according to the path.Clean function (see
// https://golang.org/pkg/path#Clean).
// Note always cleans to '/' path separators regardless of platform (git friendly)
func CleanPaths(paths, delim string) (cleaned []string) {
	// If paths is an empty string, splitting it will yield [""], which will
	// become the path ".". To avoid this, bail out if trimmed paths
	// argument is empty.
	if paths = strings.TrimSpace(paths); len(paths) == 0 {
		return
	}

	for _, part := range strings.Split(paths, delim) {
		part = strings.TrimSpace(part)

		// Remove trailing `/` or `\`, but only the first one.
		for _, sep := range []string{`/`, `\`} {
			if strings.HasSuffix(part, sep) {
				part = strings.TrimSuffix(part, sep)
				break
			}
		}

		cleaned = append(cleaned, part)
	}

	return cleaned
}

// repositoryPermissionFetcher is an interface that matches the configuration
// object and can be used to fetch repository permissions.
type repositoryPermissionFetcher interface {
	RepositoryPermissions(executable bool) os.FileMode
}

// MkdirAll makes a directory and any intervening directories with the
// permissions specified by the core.sharedRepository setting.
func MkdirAll(path string, config repositoryPermissionFetcher) error {
	umask := 0777 & ^config.RepositoryPermissions(true)
	return doWithUmask(int(umask), func() error {
		return os.MkdirAll(path, config.RepositoryPermissions(true))
	})
}

var (
	// currentUser is a wrapper over user.Current(), but instead uses the
	// value of os.Getenv("HOME") for the returned *user.User's "HomeDir"
	// member.
	currentUser func() (*user.User, error) = func() (*user.User, error) {
		u, err := user.Current()
		if err != nil {
			return nil, err
		}
		u.HomeDir = os.Getenv("HOME")
		return u, nil
	}
	lookupUser       func(who string) (*user.User, error) = user.Lookup
	lookupConfigHome func() string                        = func() string {
		return os.Getenv("XDG_CONFIG_HOME")
	}
)

// ExpandPath returns a copy of path with any references to the current user's
// home directory (spelled "~"), or a named user's home directory (spelled
// "~user") in the path, sanitized to the calling filesystem's path separator
// preference.
//
// If the "expand" argument is given as true, the resolved path to the named
// user's home directory will expanded with filepath.EvalSymlinks.
//
// If either the current or named user does not have a home directory, an error
// will be returned.
//
// Otherwise, the error returned will be nil, and the string returned will be
// the expanded path.
func ExpandPath(path string, expand bool) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	var username string
	if slash := strings.Index(path[1:], "/"); slash > -1 {
		username = path[1 : slash+1]
	} else {
		username = path[1:]
	}

	var (
		who *user.User
		err error
	)
	if len(username) == 0 {
		who, err = currentUser()
	} else {
		who, err = lookupUser(username)
	}

	if err != nil {
		return "", errors.Wrapf(err, "could not find user %s", username)
	}

	homedir := who.HomeDir
	if expand {
		homedir, err = filepath.EvalSymlinks(homedir)
		if err != nil {
			return "", errors.Wrapf(err, "cannot eval symlinks for %s", homedir)
		}
	}
	return filepath.Join(homedir, path[len(username)+1:]), nil
}

// ExpandConfigPath returns a copy of path expanded as with ExpandPath.  If the
// path is empty, the default path is looked up inside $XDG_CONFIG_HOME, or
// ~/.config if that is not set.
func ExpandConfigPath(path, defaultPath string) (string, error) {
	if path != "" {
		return ExpandPath(path, false)
	}

	configHome := lookupConfigHome()
	if configHome != "" {
		return filepath.Join(configHome, defaultPath), nil
	}

	return ExpandPath(fmt.Sprintf("~/.config/%s", defaultPath), false)
}

// VerifyFileHash reads a file and verifies whether the SHA is correct
// Returns an error if there is a problem
func VerifyFileHash(oid, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := NewLfsContentHash()
	_, err = io.Copy(h, f)
	if err != nil {
		return err
	}

	calcOid := hex.EncodeToString(h.Sum(nil))
	if calcOid != oid {
		return fmt.Errorf("file %q has an invalid hash %s, expected %s", path, calcOid, oid)
	}

	return nil
}

// FastWalkCallback is the signature for the callback given to FastWalkGitRepo()
type FastWalkCallback func(parentDir string, info os.FileInfo, err error)

// FastWalkDir is a more optimal implementation of filepath.Walk for a Git
// repo. The callback guaranteed to be called sequentially. The function returns
// once all files and errors have triggered callbacks.
// It differs in the following ways:
//  * Uses goroutines to parallelise large dirs and descent into subdirs
//  * Does not provide sorted output; parents will always be before children but
//    there are no other guarantees. Use parentDir argument in the callback to
//    determine absolute path rather than tracking it yourself
//  * Automatically ignores any .git directories
//
// rootDir - Absolute path to the top of the repository working directory
func FastWalkDir(rootDir string, cb FastWalkCallback) {
	fastWalkCallback(fastWalkWithExcludeFiles(rootDir), cb)
}

// fastWalkCallback calls the FastWalkCallback "cb" for all files found by the
// given *fastWalker, "walker".
func fastWalkCallback(walker *fastWalker, cb FastWalkCallback) {
	for file := range walker.ch {
		cb(file.ParentDir, file.Info, file.Err)
	}
}

// Returned from FastWalk with parent directory context
// This is needed because FastWalk can provide paths out of order so the
// parent dir cannot be implied
type fastWalkInfo struct {
	ParentDir string
	Info      os.FileInfo
	Err       error
}

type fastWalker struct {
	rootDir string
	ch      chan fastWalkInfo
	limit   int32
	cur     *int32
	wg      *sync.WaitGroup
}

// fastWalkWithExcludeFiles walks the contents of a dir, respecting
// include/exclude patterns.
//
// rootDir - Absolute path to the top of the repository working directory
func fastWalkWithExcludeFiles(rootDir string) *fastWalker {
	excludePaths := []filepathfilter.Pattern{
		filepathfilter.NewPattern(".git"),
		filepathfilter.NewPattern("**/.git"),
	}

	limit, _ := strconv.Atoi(os.Getenv("LFS_FASTWALK_LIMIT"))
	if limit < 1 {
		limit = runtime.GOMAXPROCS(-1) * 20
	}

	c := int32(0)
	w := &fastWalker{
		rootDir: rootDir,
		limit:   int32(limit),
		cur:     &c,
		ch:      make(chan fastWalkInfo, 256),
		wg:      &sync.WaitGroup{},
	}

	go func() {
		defer w.Wait()

		dirFi, err := os.Stat(w.rootDir)
		if err != nil {
			w.ch <- fastWalkInfo{Err: err}
			return
		}

		w.Walk(true, "", dirFi, excludePaths)
	}()
	return w
}

// Walk is the main recursive implementation of fast walk.  Sends the file/dir
// and any contents to the channel so long as it passes the include/exclude
// filter.  Increments waitg.Add(1) for each new goroutine launched internally
//
// workDir - Relative path inside the repository
func (w *fastWalker) Walk(isRoot bool, workDir string, itemFi os.FileInfo,
	excludePaths []filepathfilter.Pattern) {

	var fullPath string      // Absolute path to the current file or dir
	var parentWorkDir string // Absolute path to the workDir inside the repository
	if isRoot {
		fullPath = w.rootDir
	} else {
		parentWorkDir = join(w.rootDir, workDir)
		fullPath = join(parentWorkDir, itemFi.Name())
	}

	if !isRoot && itemFi.IsDir() {
		// If this directory has a .git directory or file in it, then
		// this is a submodule, and we should not recurse into it.
		_, err := os.Stat(filepath.Join(fullPath, ".git"))
		if err == nil {
			return
		}
	}

	workPath := join(workDir, itemFi.Name())
	if !filepathfilter.NewFromPatterns(nil, excludePaths).Allows(workPath) {
		return
	}

	w.ch <- fastWalkInfo{ParentDir: parentWorkDir, Info: itemFi}

	if !itemFi.IsDir() {
		// Nothing more to do if this is not a dir
		return
	}

	var childWorkDir string
	if !isRoot {
		childWorkDir = join(workDir, itemFi.Name())
	}

	// The absolute optimal way to scan would be File.Readdirnames but we
	// still need the Stat() to know whether something is a dir, so use
	// File.Readdir instead. Means we can provide os.FileInfo to callers like
	// filepath.Walk as a bonus.
	df, err := os.Open(fullPath)
	if err != nil {
		w.ch <- fastWalkInfo{Err: err}
		return
	}

	// The number of items in a dir we process in each goroutine
	jobSize := 100
	for children, err := df.Readdir(jobSize); err == nil; children, err = df.Readdir(jobSize) {
		// Parallelise all dirs, and chop large dirs into batches
		w.walk(children, func(subitems []os.FileInfo) {
			for _, childFi := range subitems {
				w.Walk(false, childWorkDir, childFi, excludePaths)
			}
		})
	}

	df.Close()
	if err != nil && err != io.EOF {
		w.ch <- fastWalkInfo{Err: err}
	}
}

func (w *fastWalker) walk(children []os.FileInfo, fn func([]os.FileInfo)) {
	cur := atomic.AddInt32(w.cur, 1)
	if cur > w.limit {
		fn(children)
		atomic.AddInt32(w.cur, -1)
		return
	}

	w.wg.Add(1)
	go func() {
		fn(children)
		w.wg.Done()
		atomic.AddInt32(w.cur, -1)
	}()
}

func (w *fastWalker) Wait() {
	w.wg.Wait()
	close(w.ch)
}

func join(paths ...string) string {
	ne := make([]string, 0, len(paths))

	for _, p := range paths {
		if len(p) > 0 {
			ne = append(ne, p)
		}
	}
	return strings.Join(ne, "/")
}

// SetFileWriteFlag changes write permissions on a file
// Used to make a file read-only or not. When writeEnabled = false, the write
// bit is removed for all roles. When writeEnabled = true, the behaviour is
// different per platform:
// On Mac & Linux, the write bit is set only on the owner as per default umask.
// All other bits are unaffected.
// On Windows, all the write bits are set since Windows doesn't support Unix permissions.
func SetFileWriteFlag(path string, writeEnabled bool) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := uint32(stat.Mode())

	if (writeEnabled && (mode&0200) > 0) ||
		(!writeEnabled && (mode&0222) == 0) {
		// no change needed
		return nil
	}

	if writeEnabled {
		mode = mode | 0200 // set owner write only
		// Go's own Chmod makes Windows set all though
	} else {
		mode = mode &^ 0222 // disable all write
	}
	return os.Chmod(path, os.FileMode(mode))
}

// TempFile creates a temporary file in specified directory with proper permissions for the repository.
// On success, it returns an open, non-nil *os.File, and the caller is responsible
// for closing and/or removing it.  On failure, the temporary file is
// automatically cleaned up and an error returned.
//
// This function is designed to handle only temporary files that will be renamed
// into place later somewhere within the Git repository.
func TempFile(dir, pattern string, cfg repositoryPermissionFetcher) (*os.File, error) {
	tmp, err := ioutil.TempFile(dir, pattern)
	if err != nil {
		return nil, err
	}

	perms := cfg.RepositoryPermissions(false)
	err = os.Chmod(tmp.Name(), perms)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	return tmp, nil
}

// ExecutablePermissions takes a set of Unix permissions (which may or may not
// have the executable bits set) and maps them into a set of permissions in
// which the executable bits are set, using the same technique as Git does.
func ExecutablePermissions(perms os.FileMode) os.FileMode {
	// Copy read bits to executable bits.
	return perms | ((perms & 0444) >> 2)
}

// CanonicalizePath takes a path and produces a canonical absolute path,
// performing any OS- or environment-specific path transformations (within the
// limitations of the Go standard library).  If the path is empty, it returns
// the empty path with no error.  If missingOk is true, then if the
// canonicalized path does not exist, an absolute path is given instead.
func CanonicalizePath(path string, missingOk bool) (string, error) {
	path, err := TranslateCygwinPath(path)
	if err != nil {
		return "", err
	}
	if len(path) > 0 {
		path, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		result, err := CanonicalizeSystemPath(path)
		if err != nil && os.IsNotExist(err) && missingOk {
			return path, nil
		}
		if err != nil && runtime.GOOS == "windows" {
			// As EvalSymlinks is broken on windows
			// it may fail even if the path is valid, so if the file exists we should use it
			if _, err1 := os.Stat(path); !os.IsNotExist(err1) {
				return path, nil
			}
		}
		return result, err
	}
	return "", nil
}
