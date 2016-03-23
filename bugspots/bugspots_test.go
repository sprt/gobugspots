package bugspots

import (
	"reflect"
	"testing"
	"time"
)

func mockCommandOutputter(out string) commandOutputter {
	return func(_ string, _ ...string) (string, error) {
		return out, nil
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	var tests = []struct {
		in  []int64
		out float64
	}{
		{[]int64{50, 50, 100}, 0},
		{[]int64{75, 50, 100}, 0.5},
		{[]int64{100, 50, 100}, 1},
	}

	for _, tt := range tests {
		actual := normalizeTimestamp(tt.in[0], tt.in[1], tt.in[2])
		if actual != tt.out {
			t.Errorf("got %v, expected %v", actual, tt.out)
		}
	}
}

func TestRepoHeadFiles(t *testing.T) {
	var tests = []struct {
		in  string
		out []string
	}{
		{"", []string{}},
		{"foo", []string{"foo"}},
		{"foo\nbar", []string{"foo", "bar"}},
	}

	repo := &Repo{}
	for _, tt := range tests {
		repo.commandOutput = mockCommandOutputter(tt.in)
		actual, _ := repo.headFiles()
		if !reflect.DeepEqual(actual, tt.out) {
			t.Errorf("got %#v, expected %#v", actual, tt.out)
		}
	}
}

func TestRepoBugFixCommits(t *testing.T) {
	var tests = []struct {
		in  string
		out []commit
	}{
		{"", []commit{}},
		{"1\nfoo\nbar\n\n2\nbaz", []commit{
			commit{t: time.Unix(1, 0), files: []string{"foo", "bar"}},
			commit{t: time.Unix(2, 0), files: []string{"baz"}},
		}},
	}

	repo := &Repo{}
	for _, tt := range tests {
		repo.commandOutput = mockCommandOutputter(tt.in)
		actual, _ := repo.bugFixCommits(DefaultCommitPattern)
		if !reflect.DeepEqual(actual, tt.out) {
			t.Errorf("got %#v, expected %#v", actual, tt.out)
		}
	}
}

func TestFirstCommitTime(t *testing.T) {
	repo := &Repo{commandOutput: mockCommandOutputter("hash\n1")}
	actual, err := repo.firstCommitTime()
	if actual != 1 {
		t.Errorf("got (%v, %v), expected (%v, <nil>)", actual, err, 1)
	}
}

func TestLastCommitTime(t *testing.T) {
	repo := &Repo{commandOutput: mockCommandOutputter("hash\n2")}
	actual, err := repo.lastCommitTime()
	if actual != 2 {
		t.Errorf("got (%v, %v), expected (%v, <nil>)", actual, err, 2)
	}
}
