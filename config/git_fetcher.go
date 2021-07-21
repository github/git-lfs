package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/git-lfs/git-lfs/git"
)

type GitFetcher struct {
	vmu  sync.RWMutex
	vals map[string][]string
}

func readGitConfig(configs ...*git.ConfigurationSource) (gf *GitFetcher, extensions map[string]Extension, uniqRemotes map[string]bool) {
	vals := make(map[string][]string)
	ignored := make([]string, 0)

	extensions = make(map[string]Extension)
	uniqRemotes = make(map[string]bool)

	for _, gc := range configs {
		uniqKeys := make(map[string]string)

		for _, line := range gc.Lines {
			pieces := strings.SplitN(line, "=", 2)
			if len(pieces) < 2 {
				continue
			}

			allowed := !gc.OnlySafeKeys

			// We don't need to change the case of the key here,
			// since Git will already have canonicalized it for us.
			key, val := pieces[0], pieces[1]

			if origKey, ok := uniqKeys[key]; ok {
				if ShowConfigWarnings && len(vals[key]) > 0 && vals[key][len(vals[key])-1] != val && strings.HasPrefix(key, gitConfigWarningPrefix) {
					fmt.Fprintf(os.Stderr, "WARNING: These git config values clash:\n")
					fmt.Fprintf(os.Stderr, "  git config %q = %q\n", origKey, vals[key])
					fmt.Fprintf(os.Stderr, "  git config %q = %q\n", pieces[0], val)
				}
			} else {
				uniqKeys[key] = pieces[0]
			}

			parts := strings.Split(key, ".")
			if len(parts) == 4 && parts[0] == "lfs" && parts[1] == "extension" {
				// prop: lfs.extension.<name>.<prop>
				name := parts[2]
				prop := parts[3]

				ext := extensions[name]
				ext.Name = name

				switch prop {
				case "clean":
					if gc.OnlySafeKeys {
						ignored = append(ignored, key)
						continue
					}
					ext.Clean = val
				case "smudge":
					if gc.OnlySafeKeys {
						ignored = append(ignored, key)
						continue
					}
					ext.Smudge = val
				case "priority":
					allowed = true
					p, err := strconv.Atoi(val)
					if err == nil && p >= 0 {
						ext.Priority = p
					}
				}

				extensions[name] = ext
			} else if len(parts) > 1 && parts[0] == "remote" {
				if gc.OnlySafeKeys && (len(parts) == 3 && parts[2] != "lfsurl") {
					ignored = append(ignored, key)
					continue
				}

				allowed = true
				remote := strings.Join(parts[1:len(parts)-1], ".")
				uniqRemotes[remote] = remote == "origin"
			} else if len(parts) > 2 && parts[len(parts)-1] == "access" {
				allowed = true
			}

			if !allowed && keyIsUnsafe(key) {
				ignored = append(ignored, key)
				continue
			}

			vals[key] = append(vals[key], val)
		}
	}

	if len(ignored) > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: These unsafe lfsconfig keys were ignored:\n\n")
		for _, key := range ignored {
			fmt.Fprintf(os.Stderr, "  %s\n", key)
		}
	}

	gf = &GitFetcher{vals: vals}

	return
}

// Get implements the Fetcher interface, and returns the value associated with
// a given key and true, signaling that the value was present. Otherwise, an
// empty string and false will be returned, signaling that the value was
// absent.
//
// Map lookup by key is case-insensitive, except for the middle part of a
// three-part key, as per the .gitconfig specification.
//
// Get is safe to call across multiple goroutines.
func (g *GitFetcher) Get(key string) (val string, ok bool) {
	all := g.GetAll(key)

	if len(all) == 0 {
		return "", false
	}
	return all[len(all)-1], true
}

func (g *GitFetcher) GetAll(key string) []string {
	g.vmu.RLock()
	defer g.vmu.RUnlock()

	return g.vals[g.caseFoldKey(key)]
}

func (g *GitFetcher) All() map[string][]string {
	newmap := make(map[string][]string)

	g.vmu.RLock()
	defer g.vmu.RUnlock()

	for key, values := range g.vals {
		for _, value := range values {
			newmap[key] = append(newmap[key], value)
		}
	}

	return newmap
}

func (g *GitFetcher) caseFoldKey(key string) string {
	parts := strings.Split(key, ".")
	last := len(parts) - 1

	// We check for 3 or more parts here because if the middle part is a
	// URL, it may have dots in it. We'll downcase the part before the first
	// dot and after the last dot, but preserve the piece in the middle,
	// which may be a branch name, remote, or URL, all of which are
	// case-sensitive. This is the algorithm Git uses to canonicalize its
	// keys.
	if len(parts) < 3 {
		return strings.ToLower(key)
	}

	return strings.Join([]string{
		strings.ToLower(parts[0]),
		strings.Join(parts[1:last], "."),
		strings.ToLower(parts[last]),
	}, ".")
}

func keyIsUnsafe(key string) bool {
	for _, safe := range safeKeys {
		if safe == key {
			return false
		}
	}
	return true
}

var safeKeys = []string{
	"lfs.allowincompletepush",
	"lfs.fetchexclude",
	"lfs.fetchinclude",
	"lfs.fetchrecentrefsdays",
	"lfs.fetchrecentremoterefs",
	"lfs.fetchrecentcommitsdays",
	"lfs.fetchrecentalways",
	"lfs.pruneoffsetdays",
	"lfs.gitprotocol",
	"lfs.locksverify",
	"lfs.pushurl",
	"lfs.skipdownloaderrors",
	"lfs.url",
}
