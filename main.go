package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/sprt/gobugspots/bugspots"
)

func main() {
	var regexp string
	flag.StringVar(&regexp, "regexp", bugspots.DefaultCommitRegexp, "regular expression used to match bug-fixing commits")
	flag.Parse()

	repo := bugspots.NewRepoByPath(".")
	b := bugspots.NewBugspots(repo)
	b.SetRegexp(regexp)

	hotspots, err := b.Hotspots()
	if err != nil {
		log.Fatalln(err)
	}

	for _, h := range hotspots {
		fmt.Printf("%.4f %s\n", h.Score, h.File)
	}
}
