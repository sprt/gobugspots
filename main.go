package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/sprt/gobugspots/bugspots"
)

const (
	defaultPath = "."
)

var (
	minCount   int
	maxCount   int
	percentile float64
	pattern    string
	path       = defaultPath
)

func init() {
	flag.IntVar(&minCount, "min-count", bugspots.DefaultMinCount, "minimum number of hotspots to show")
	flag.IntVar(&maxCount, "max-count", bugspots.DefaultMaxCount, "maxium number of hotspots to show")
	flag.Float64Var(&percentile, "percentile", bugspots.DefaultPercentile, "upper percentile of hotspots to show")
	flag.StringVar(&pattern, "pattern", bugspots.DefaultCommitPattern, "regular expression used to match bug-fixing commits")
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [options] [path]\n\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if len(flag.Args()) > 0 {
		path = flag.Args()[0]
	}

	repo := bugspots.NewRepoByPath(path)
	b := bugspots.NewBugspots(repo)
	b.SetPattern(pattern)

	hotspots, err := b.Hotspots()
	if err != nil {
		log.Fatalln(err)
	}

	s := bugspots.NewSlicer(percentile)
	s.SetMinCount(minCount)
	s.SetMaxCount(maxCount)
	hotspots = s.Slice(hotspots)

	for _, h := range hotspots {
		fmt.Printf("%.4f %s\n", h.Score, h.File)
	}

	if len(hotspots) == 0 {
		fmt.Fprintln(os.Stderr, "no hotspots")
	}
}
