package commands

import (
	"fmt"
	"os"

	"github.com/git-lfs/git-lfs/filepathfilter"
	"github.com/git-lfs/git-lfs/git"
	"github.com/git-lfs/git-lfs/lfs"
	"github.com/git-lfs/git-lfs/tasklog"
	"github.com/git-lfs/git-lfs/tq"
	"github.com/spf13/cobra"
)

var (
	checkoutTo     string
	checkoutBase   bool
	checkoutOurs   bool
	checkoutTheirs bool
)

func checkoutCommand(cmd *cobra.Command, args []string) {
	setupRepository()

	stage, err := whichCheckout()
	if err != nil {
		Exit("Error parsing args: %v", err)
	}

	if checkoutTo != "" && stage != git.IndexStageDefault {
		checkoutConflict(rootedPaths(args)[0], stage)
		return
	} else if checkoutTo != "" || stage != git.IndexStageDefault {
		Exit("--to and exactly one of --theirs, --ours, and --base must be used together")
	}

	ref, err := git.CurrentRef()
	if err != nil {
		Panic(err, "Could not checkout")
	}

	singleCheckout := newSingleCheckout(cfg.Git, "")
	if singleCheckout.Skip() {
		fmt.Println("Cannot checkout LFS objects, Git LFS is not installed.")
		return
	}

	var totalBytes int64
	var pointers []*lfs.WrappedPointer
	logger := tasklog.NewLogger(os.Stdout,
		tasklog.ForceProgress(cfg.ForceProgress()),
	)
	meter := tq.NewMeter(cfg)
	meter.Direction = tq.Checkout
	meter.Logger = meter.LoggerFromEnv(cfg.Os)
	logger.Enqueue(meter)
	chgitscanner := lfs.NewGitScanner(cfg, func(p *lfs.WrappedPointer, err error) {
		if err != nil {
			LoggedError(err, "Scanner error: %s", err)
			return
		}

		totalBytes += p.Size
		meter.Add(p.Size)
		meter.StartTransfer(p.Name)
		pointers = append(pointers, p)
	})

	chgitscanner.Filter = filepathfilter.New(rootedPaths(args), nil)

	if err := chgitscanner.ScanTree(ref.Sha); err != nil {
		ExitWithError(err)
	}
	chgitscanner.Close()

	meter.Start()
	for _, p := range pointers {
		singleCheckout.Run(p)

		// not strictly correct (parallel) but we don't have a callback & it's just local
		// plus only 1 slot in channel so it'll block & be close
		meter.TransferBytes("checkout", p.Name, p.Size, totalBytes, int(p.Size))
		meter.FinishTransfer(p.Name)
	}

	meter.Finish()
	singleCheckout.Close()
}

func checkoutConflict(file string, stage git.IndexStage) {
	singleCheckout := newSingleCheckout(cfg.Git, "")
	if singleCheckout.Skip() {
		fmt.Println("Cannot checkout LFS objects, Git LFS is not installed.")
		return
	}

	ref, err := git.ResolveRef(fmt.Sprintf(":%d:%s", stage, file))
	if err != nil {
		Exit("Could not checkout (are you not in the middle of a merge?): %v", err)
	}

	scanner, err := git.NewObjectScanner(cfg.GitEnv(), cfg.OSEnv())
	if err != nil {
		Exit("Could not create object scanner: %v", err)
	}

	if !scanner.Scan(ref.Sha) {
		Exit("Could not find object %q", ref.Sha)
	}

	ptr, err := lfs.DecodePointer(scanner.Contents())
	if err != nil {
		Exit("Could not find decoder pointer for object %q: %v", ref.Sha, err)
	}

	p := &lfs.WrappedPointer{Name: file, Pointer: ptr}

	if err := singleCheckout.RunToPath(p, checkoutTo); err != nil {
		Exit("Error checking out %v to %q: %v", ref.Sha, checkoutTo, err)
	}
	singleCheckout.Close()
}

func whichCheckout() (stage git.IndexStage, err error) {
	seen := 0
	stage = git.IndexStageDefault

	if checkoutBase {
		seen++
		stage = git.IndexStageBase
	}
	if checkoutOurs {
		seen++
		stage = git.IndexStageOurs
	}
	if checkoutTheirs {
		seen++
		stage = git.IndexStageTheirs
	}

	if seen > 1 {
		return 0, fmt.Errorf("at most one of --base, --theirs, and --ours is allowed")
	}
	return stage, nil
}

// Parameters are filters
// firstly convert any pathspecs to patterns relative to the root of the repo,
// in case this is being executed in a sub-folder
func rootedPaths(args []string) []string {
	pathConverter, err := lfs.NewCurrentToRepoPatternConverter(cfg)
	if err != nil {
		Panic(err, "Could not checkout")
	}

	rootedpaths := make([]string, 0, len(args))
	for _, arg := range args {
		rootedpaths = append(rootedpaths, pathConverter.Convert(arg))
	}
	return rootedpaths
}

func init() {
	RegisterCommand("checkout", checkoutCommand, func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&checkoutTo, "to", "", "Checkout a conflicted file to this path")
		cmd.Flags().BoolVar(&checkoutOurs, "ours", false, "Checkout our version of a conflicted file")
		cmd.Flags().BoolVar(&checkoutTheirs, "theirs", false, "Checkout their version of a conflicted file")
		cmd.Flags().BoolVar(&checkoutBase, "base", false, "Checkout the base version of a conflicted file")
	})
}
