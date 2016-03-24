package bugspots

import (
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultCommitPattern is the default regular expression used to match
	// bug-fixing commits.
	DefaultCommitPattern = "\\b(fix(e[sd])?|close[sd]?) (#|gh-)[1-9][0-9]*\\b"
	// DefaultMinCount is the default minimum number of hotspots to return.
	DefaultMinCount = 0
	// DefaultMaxCount is the default maximum number of hotspots to return.
	DefaultMaxCount = math.MaxInt32 // int is either int32 or int64
	// DefaultPercentile is the default upper percentile of hotspots to
	// return.
	DefaultPercentile = 10.0
)

type commandOutputter func(string, ...string) (string, error)

func newCommandOutputter(dir string) commandOutputter {
	return func(name string, args ...string) (out string, err error) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir

		outb, err := cmd.Output()
		if err != nil {
			return
		}

		out = strings.TrimSpace(string(outb[:]))
		return
	}
}

// Repo is a path to a git a repository.
type Repo struct {
	commandOutput commandOutputter
	Path          string
}

// NewRepoByPath returns a pointer to a new Repo.
func NewRepoByPath(path string) *Repo {
	return &Repo{
		newCommandOutputter(path),
		path,
	}
}

func parseLsFiles(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, "\n")
}

// headFiles returns the files at HEAD.
func (r *Repo) headFiles() (headFiles []string, err error) {
	out, err := r.commandOutput("git", "ls-files")
	if err != nil {
		return
	}
	headFiles = parseLsFiles(out)
	return
}

type commit struct {
	t     time.Time
	files []string
}

// assumes `git log --format=format:%ct --name-only'
func parseLog(raw string) ([]commit, error) {
	if raw == "" {
		return []commit{}, nil
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
func (r *Repo) bugFixCommits(pattern string) ([]commit, error) {
	// --diff-filter ignores commits with no files attached
	out, err := r.commandOutput("git", "log", "--diff-filter=ACDMRTUXB",
		"-E", "-i", "--grep", pattern, "--format=format:%ct", "--name-only")
	if err != nil {
		return nil, err
	}
	commits, err := parseLog(out)
	if err != nil {
		return nil, err
	}
	return commits, nil
}

func parseRevList(raw string) (int64, error) {
	lines := strings.Split(raw, "\n")
	if len(lines) != 2 {
		return 0, errors.New("no commits")
	}
	t, err := strconv.Atoi(lines[1])
	return int64(t), err
}

// firstCommitTime returns the timestamp of the first commit in the history.
func (r *Repo) firstCommitTime() (t int64, err error) {
	out, err := r.commandOutput("git", "rev-list", "--max-parents=0", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// lastCommitTime returns the timestamp of the last commit in the history.
func (r *Repo) lastCommitTime() (t int64, err error) {
	out, err := r.commandOutput("git", "rev-list", "--max-count=1", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// Bugspots is the interface to the algorithm.
type Bugspots struct {
	Repo       *Repo
	pattern    string
	minCount   int
	maxCount   int
	percentile float64
}

// NewBugspots returns a pointer to a new Bugspots object.
func NewBugspots(repo *Repo) *Bugspots {
	return &Bugspots{
		Repo:       repo,
		pattern:    DefaultCommitPattern,
		minCount:   DefaultMinCount,
		maxCount:   DefaultMaxCount,
		percentile: DefaultPercentile,
	}
}

// SetPattern sets the pattern parameter.
func (b *Bugspots) SetPattern(pattern string) {
	b.pattern = pattern
}

func normalizeTimestamp(t, lo, hi int64) float64 {
	return float64(t-lo) / float64(hi-lo)
}

func scoreFunc(t float64) float64 {
	return 1 / (1 + math.Exp(-12*t+12))
}

// Hotspot represents a bug-prone file.
type Hotspot struct {
	// File is a path relative to the working directory.
	File string
	// Score is the score of the file according to the ranking function.
	Score float64
}

type hotspotList []Hotspot

func (l hotspotList) Len() int           { return len(l) }
func (l hotspotList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l hotspotList) Less(i, j int) bool { return l[i].Score > l[j].Score } // sic

// Hotspots returns the hotspots ranked by score.
func (b *Bugspots) Hotspots() ([]Hotspot, error) {
	headFiles, err := b.Repo.headFiles()
	tfirst, err := b.Repo.firstCommitTime()
	tlast, err := b.Repo.lastCommitTime()
	commits, err := b.Repo.bugFixCommits(b.pattern)
	if err != nil {
		return nil, err
	}

	hotspots := make(hotspotList, 0, len(headFiles))
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
			hotspots = append(hotspots, Hotspot{headFile, score})
		}
	}
	sort.Sort(hotspots)

	return hotspots, nil
}

// Slicer is a helper class that simplifies extracting a specified upper
// percentile from a slice of Hotspot objects, given a minimum and a maximum
// number of entries to extract.
type Slicer struct {
	minCount   int
	maxCount   int
	percentile float64
}

// NewSlicer returns a pointer to a new Slicer object.
func NewSlicer(percentile float64) *Slicer {
	return &Slicer{
		minCount:   DefaultMinCount,
		maxCount:   DefaultMaxCount,
		percentile: percentile,
	}
}

// SetMinCount sets the minimum number of hotspots to return,
// regardless of the specified upper percentile.
func (s *Slicer) SetMinCount(minCount int) {
	if minCount < 0 {
		panic("minCount must be non-negative")
	}
	s.minCount = minCount
}

// SetMaxCount sets the maximum number of hotspots to return,
// regardless of the specified upper percentile.
func (s *Slicer) SetMaxCount(maxCount int) {
	if maxCount <= 0 {
		panic("maxCount must be over zero")
	}
	s.maxCount = maxCount
}

// Slice returns the slice.
func (s *Slicer) Slice(hotspots []Hotspot) []Hotspot {
	retCount := math.Min(float64(s.maxCount), math.Max(float64(s.minCount), s.percentile/100*float64(len(hotspots))))
	return hotspots[:int(retCount)]
}
