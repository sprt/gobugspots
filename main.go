package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const regexp = "\\b(fix(e[sd])?|close[sd]?) (#|gh-)[1-9][0-9]*\\b"

type commit struct {
	t     time.Time
	files []string
}

// Hotspot represents a bug-prone file.
type Hotspot struct {
	// File is a path relative to the working directory.
	File string
	// Score is the score of the file according to the ranking function.
	Score float64
}

// HotspotList represents a list of hotspots.
type HotspotList []Hotspot

// Repo represents a git repository.
type Repo struct {
	path string
}

func checkOutput(path, cmd string, args ...string) (out string, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	if err = os.Chdir(path); err != nil {
		return
	}
	defer os.Chdir(wd)

	outb, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return
	}
	out = strings.TrimSpace(string(outb[:]))

	return
}

func (p HotspotList) Len() int           { return len(p) }
func (p HotspotList) Less(i, j int) bool { return p[i].Score < p[j].Score }
func (p HotspotList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func sortMapByValue(m map[string]float64) HotspotList {
	pl := make(HotspotList, len(m))
	i := 0
	for k, v := range m {
		pl[i] = Hotspot{k, v}
		i++
	}
	sort.Sort(sort.Reverse(pl))
	return pl
}

// NewRepo returns a pointer to a new Repo object.
func NewRepo(path string) *Repo {
	return &Repo{"."}
}

func parseLsFiles(raw string) []string {
	return strings.Split(raw, "\n")
}

// HeadFiles returns the files at HEAD.
func (r *Repo) HeadFiles() (headFiles []string, err error) {
	out, err := checkOutput(r.path, "git", "ls-files")
	if err != nil {
		return
	}
	headFiles = parseLsFiles(out)
	return
}

func parseRevList(raw string) (t int, err error) {
	lines := strings.Split(raw, "\n")
	if len(lines) != 2 {
		err = errors.New("no commits")
		return
	}
	t, err = strconv.Atoi(lines[1])
	return
}

// FirstCommitTime returns the timestamp of the first commit in the history.
func (r *Repo) FirstCommitTime() (t int, err error) {
	out, err := checkOutput(r.path, "git", "rev-list", "--max-parents=0", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// LastCommitTime returns the timestamp of the last commit in the history.
func (r *Repo) LastCommitTime() (t int, err error) {
	out, err := checkOutput(r.path, "git", "rev-list", "--max-count=1", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
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
			return []commit{}, fmt.Errorf("invalid timestamp '%v'", lines[0])
		}
		t := time.Unix(int64(timestamp), 0)
		commits = append(commits, commit{t, lines[1:]})
	}
	return commits, nil
}

// BugFixCommits returns the bug-fix commits.
func (r *Repo) BugFixCommits() ([]commit, error) {
	// --diff-filter ignores commits with no files attached
	out, err := checkOutput(r.path, "git", "log", "--diff-filter=ACDMRTUXB", "-E", "-i", "--grep="+regexp, "--format=format:%ct", "--name-only")
	if err != nil {
		return []commit{}, err
	}
	commits, err := parseLog(out)
	if err != nil {
		return []commit{}, err
	}
	return commits, nil
}

func normalizeTimestamp(t, lo, hi int64) float64 {
	return float64((t - lo) / (hi - lo))
}

func scoreFunc(t float64) float64 {
	return 1 / (1 + math.Exp(-12*t+12))
}

// Hotspots returns the top 10% hotspots, ranked by score.
func (r *Repo) Hotspots() (HotspotList, error) {
	commits, err := r.BugFixCommits()
	if err != nil {
		return nil, err
	}

	tfirst, err := r.FirstCommitTime()
	if err != nil {
		return nil, err
	}
	tlast, err := r.LastCommitTime()
	if err != nil {
		return nil, err
	}

	headFiles, err := r.HeadFiles()
	if err != nil {
		return nil, err
	}

	hotspots := map[string]float64{}
	for _, commit := range commits {
		t := normalizeTimestamp(commit.t.Unix(), int64(tfirst), int64(tlast))
		for _, file := range commit.files {
			isHead := false
			for _, headFile := range headFiles {
				if file == headFile {
					isHead = true
					break
				}
			}
			if isHead {
				hotspots[file] += scoreFunc(t)
			}
		}
	}

	sortedHotspots := sortMapByValue(hotspots)
	topHotspots := sortedHotspots[:len(sortedHotspots)/10]

	return topHotspots, nil
}

func main() {
	repo := NewRepo(".")
	hotspots, err := repo.Hotspots()
	if err != nil {
		log.Fatalln(err)
	}

	for _, h := range hotspots {
		fmt.Printf("%.4f %s\n", h.Score, h.File)
	}
}
