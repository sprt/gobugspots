package bugspots

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/btree"
)

const (
	// DefaultCommitRegexp is the default regular expression used to match
	// bug-fixing commits.
	DefaultCommitRegexp = "\\b(fix(e[sd])?|close[sd]?) (#|gh-)[1-9][0-9]*\\b"
	// DefaultMinCount is the default minimum number of hotspots to return.
	DefaultMinCount = 0
	// DefaultMaxCount is the default maximum number of hotspots to return.
	DefaultMaxCount = math.MaxInt32 // int is either int32 or int64
	// DefaultPercentile is the default upper percentile of hotspots to
	// return.
	DefaultPercentile = 10.0
)

type slicerOptions struct {
	minCount   int
	maxCount   int
	percentile float64
}

func newSlicerOptions() *slicerOptions {
	return &slicerOptions{
		minCount:   DefaultMinCount,
		maxCount:   DefaultMaxCount,
		percentile: DefaultPercentile,
	}
}

// SetMinCount sets the minimum number of hotspots returned by Hotspots.
func (so *slicerOptions) SetMinCount(minCount int) {
	if minCount < 0 {
		panic("minCount cannot be negative")
	}
	so.minCount = minCount
}

// SetMaxCount sets the maximum number of hotspots returned by Hotspots.
func (so *slicerOptions) SetMaxCount(maxCount int) {
	if maxCount <= 0 {
		panic("maxCount must be over zero")
	}
	so.maxCount = maxCount
}

// SetPercentile sets the upper percentile of hotspots returned by Hotspots.
func (so *slicerOptions) SetPercentile(percentile float64) {
	if percentile <= 0 || percentile > 100 {
		panic("percentile must be in range (0, 100]")
	}
	so.percentile = percentile
}

type slicer struct {
	*slicerOptions
	tree  *btree.BTree
	slice []*Hotspot
}

func newSlicer(options *slicerOptions, tree *btree.BTree) *slicer {
	return &slicer{
		slicerOptions: options,
		tree:          tree,
		slice:         []*Hotspot{},
	}
}

// return true to continue iterating, or false to stop
func (s *slicer) Iterator(item btree.Item) bool {
	s.slice = append(s.slice, item.(*Hotspot))

	if reachedMin := len(s.slice) >= s.minCount; !reachedMin {
		return true
	}

	if reachedMax := len(s.slice) == s.maxCount; reachedMax {
		return false
	}

	reachedPercentile := len(s.slice) == int(s.percentile/100*float64(s.tree.Len()))
	return !reachedPercentile
}

type commit struct {
	t     time.Time
	files []string
}

// Repo is a path to a git a repository.
type Repo struct {
	Path string
}

// NewRepoByPath returns a pointer to a new Repo.
func NewRepoByPath(path string) *Repo {
	return &Repo{path}
}

func (r *Repo) cmdOutput(cmd string, args ...string) (out string, err error) {
	// TODO: make thread-safe
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	if err = os.Chdir(r.Path); err != nil {
		return
	}
	defer func() {
		if err = os.Chdir(wd); err != nil {
			return
		}
	}()

	outb, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return
	}
	out = strings.TrimSpace(string(outb[:]))

	return
}

func parseLsFiles(raw string) []string {
	return strings.Split(raw, "\n")
}

// headFiles returns the files at HEAD.
func (r *Repo) headFiles() (headFiles []string, err error) {
	out, err := r.cmdOutput("git", "ls-files")
	if err != nil {
		return
	}
	headFiles = parseLsFiles(out)
	return
}

// assumes `git log --format=format:%ct --name-only'
func parseLog(raw string) ([]commit, error) {
	if raw == "" {
		return nil, nil
	}
	commits := []commit{}
	for _, commitRaw := range strings.Split(raw, "\n\n") {
		lines := strings.Split(commitRaw, "\n")
		timestamp, err := strconv.Atoi(lines[0])
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp '%v'", lines[0])
		}
		t := time.Unix(int64(timestamp), 0)
		commits = append(commits, commit{t, lines[1:]})
	}
	return commits, nil
}

// bugFixCommits returns the bug-fix commits.
func (r *Repo) bugFixCommits(regexp string) ([]commit, error) {
	// --diff-filter ignores commits with no files attached
	out, err := r.cmdOutput("git", "log", "--diff-filter=ACDMRTUXB",
		"-E", "-i", "--grep", regexp, "--format=format:%ct", "--name-only")
	if err != nil {
		return nil, err
	}
	commits, err := parseLog(out)
	if err != nil {
		return nil, err
	}
	return commits, nil
}

func parseRevList(raw string) (t64 int64, err error) {
	lines := strings.Split(raw, "\n")
	if len(lines) != 2 {
		err = errors.New("no commits")
		return
	}
	t, err := strconv.Atoi(lines[1])
	t64 = int64(t)
	return
}

// firstCommitTime returns the timestamp of the first commit in the history.
func (r *Repo) firstCommitTime() (t int64, err error) {
	out, err := r.cmdOutput("git", "rev-list", "--max-parents=0", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// lastCommitTime returns the timestamp of the last commit in the history.
func (r *Repo) lastCommitTime() (t int64, err error) {
	out, err := r.cmdOutput("git", "rev-list", "--max-count=1", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// Hotspot represents a bug-prone file.
type Hotspot struct {
	// File is a path relative to the working directory.
	File string
	// Score is the score of the file according to the ranking function.
	Score float64
}

// Less returns true if a.Score > b.Score (sic).
func (a Hotspot) Less(b btree.Item) bool {
	return a.Score > b.(*Hotspot).Score
}

// Bugspots is the interface to the algorithm.
type Bugspots struct {
	*slicerOptions
	Repo   *Repo
	regexp string
}

// NewBugspots returns a pointer to a new Bugspots object.
func NewBugspots(repo *Repo) *Bugspots {
	return &Bugspots{
		slicerOptions: newSlicerOptions(),
		Repo:          repo,
		regexp:        DefaultCommitRegexp,
	}
}

// SetRegexp sets the regexp parameter.
func (b *Bugspots) SetRegexp(regexp string) {
	b.regexp = regexp
}

func normalizeTimestamp(t, lo, hi int64) float64 {
	return float64(t-lo) / float64(hi-lo)
}

func scoreFunc(t float64) float64 {
	return 1 / (1 + math.Exp(-12*t+12))
}

// Hotspots returns the top hotspots, ranked by score.
func (b *Bugspots) Hotspots() ([]*Hotspot, error) {
	headFiles, err := b.Repo.headFiles()
	tfirst, err := b.Repo.firstCommitTime()
	tlast, err := b.Repo.lastCommitTime()
	commits, err := b.Repo.bugFixCommits(b.regexp)
	if err != nil {
		return nil, err
	}

	tree := btree.New(2)
	for _, headFile := range headFiles {
		score := 0.0
		for _, commit := range commits {
			t := normalizeTimestamp(commit.t.Unix(), tfirst, tlast)
			for _, file := range commit.files {
				if file == headFile {
					score += scoreFunc(t)
				}
			}
		}
		if score != 0 {
			tree.ReplaceOrInsert(&Hotspot{headFile, score})
		}
	}

	slicer := newSlicer(b.slicerOptions, tree)
	tree.Ascend(slicer.Iterator)

	return slicer.slice, nil
}
