package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/btree"
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

// Less returns true if a.Score > b.Score (sic).
func (a Hotspot) Less(b btree.Item) bool {
	return a.Score > b.(*Hotspot).Score
}

func parseLsFiles(raw string) []string {
	return strings.Split(raw, "\n")
}

// headFiles returns the files at HEAD.
func (r *Repo) headFiles() (headFiles []string, err error) {
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

// firstCommitTime returns the timestamp of the first commit in the history.
func (r *Repo) firstCommitTime() (t int, err error) {
	out, err := checkOutput(r.path, "git", "rev-list", "--max-parents=0", "--format=%ct", "HEAD")
	if err != nil {
		return
	}
	return parseRevList(out)
}

// lastCommitTime returns the timestamp of the last commit in the history.
func (r *Repo) lastCommitTime() (t int, err error) {
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

// bugFixCommits returns the bug-fix commits.
func (r *Repo) bugFixCommits() ([]commit, error) {
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
	return float64(t-lo) / float64(hi-lo)
}

func scoreFunc(t float64) float64 {
	return 1 / (1 + math.Exp(-12*t+12))
}

// Hotspots returns the top 10% hotspots, ranked by score.
func (r *Repo) Hotspots() ([]Hotspot, error) {
	commits, err := r.bugFixCommits()
	if err != nil {
		return nil, err
	}

	tfirst, err := r.firstCommitTime()
	if err != nil {
		return nil, err
	}
	tlast, err := r.lastCommitTime()
	if err != nil {
		return nil, err
	}

	headFiles, err := r.headFiles()
	if err != nil {
		return nil, err
	}

	tree := btree.New(2)
	for _, headFile := range headFiles {
		score := 0.0
		for _, commit := range commits {
			t := normalizeTimestamp(commit.t.Unix(), int64(tfirst), int64(tlast))
			for _, file := range commit.files {
				if file == headFile {
					score += scoreFunc(t)
				}
			}
		}
		tree.ReplaceOrInsert(&Hotspot{headFile, score})
	}

	hotspots := []Hotspot{}
	tree.Ascend(func(item btree.Item) bool {
		hotspots = append(hotspots, *item.(*Hotspot))
		return len(hotspots) < tree.Len()/10
	})

	return hotspots, nil
}

func main() {
	repo := &Repo{"."}
	hotspots, err := repo.Hotspots()
	if err != nil {
		log.Fatalln(err)
	}

	for _, h := range hotspots {
		fmt.Printf("%.4f %s\n", h.Score, h.File)
	}
}
