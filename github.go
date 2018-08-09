package main

import (
	"time"

	"arp242.net/hubhub"
)

// repository is a Github repository.
type repository struct {
	Name     string    `json:"name"`
	Archived bool      `json:"archived"`
	Language string    `json:"language"`
	PushedAt time.Time `json:"pushed_at"`
	Topics   []string  `json:"topics"`
}

func listRepos(org string) ([]repository, error) {
	var repos []repository
	err := hubhub.Paginate(&repos, "GET", "/orgs/"+org+"/repos", 0)
	return repos, err
}
