package main

import (
	"time"

	"github.com/arp242/hubhub"
)

// repository is a Github repository.
type repository struct {
	Name     string    `json:"name"`
	Archived bool      `json:"archived"`
	Language string    `json:"language"`
	PushedAt time.Time `json:"pushed_at"`
	Topics   []string  `json:"topics"`
}

type info struct {
	PublicRepos  int `json:"public_repos"`
	PrivateRepos int `json:"total_private_repos"`
}

func listRepos(org string) ([]repository, error) {
	var repos []repository
	err := hubhub.Paginate(&repos, "/orgs/"+org+"/repos", 0)
	return repos, err
}
