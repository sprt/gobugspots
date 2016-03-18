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
	regexp string
	path   = defaultPath
)

func init() {
	flag.StringVar(&regexp, "regexp", bugspots.DefaultCommitRegexp, "regular expression used to match bug-fixing commits")
	if len(os.Args) >= 2 {
		path = os.Args[1]
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [options] [path]\n\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Println()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	repo := bugspots.NewRepoByPath(path)
	b := bugspots.NewBugspots(repo)
	b.SetRegexp(regexp)

	hotspots, err := b.Hotspots()
	if err != nil {
		log.Fatalln(err)
	}

	for _, h := range hotspots {
		fmt.Printf("%.4f %s\n", h.Score, h.File)
	}

	if len(hotspots) == 0 {
		fmt.Fprintln(os.Stderr, "no hotspots")
	}
}
