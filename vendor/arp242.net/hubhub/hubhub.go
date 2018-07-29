// Copyright 2018 © Martin Tournoij
// See the bottom of this file for the full copyright.

// Package hubhub is a set of utility functions for working with the GitHub API.
//
// It's not a "client library"; but just a few convenience functions that I
// found myself re-creating a bunch of times in different programs.
package hubhub // import "arp242.net/hubhub"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sync"
	"time"
)

// Global configuration.
var (
	User      string                     // GitHub username.
	Token     string                     // GitHub access token or password.
	API       = "https://api.github.com" // API base URL.
	DebugURL  = false                    // Show URLs as they're requested.
	DebugBody = false                    // Show body of requests.
)

// ErrNotOK is used when the status code is not 200 OK.
type ErrNotOK struct {
	Method, URL string
	Status      string
	StatusCode  int
}

func (e ErrNotOK) Error() string {
	return fmt.Sprintf("code %s for %s %s", e.Status, e.Method, e.URL)
}

var client = http.Client{Timeout: 10 * time.Second}

// Request something from the GitHub API.
//
// The response body will be unmarshaled in to scan unless the response code is
// 204 (No Content). A response code higher than 399 will return an ErrNotOK
// error, but won't affect the behaviour of this function.
//
// Body on the returned *http.Response is closed.
//
// This will use the global User and Token, which must be set.
func Request(scan interface{}, method, url string) (*http.Response, error) {
	if User == "" || Token == "" {
		panic("hubhub: must set User and Token")
	}

	if DebugURL {
		fmt.Printf("%v %v\n", method, url)
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	//if args.header != nil {
	//	req.Header = header
	//}

	if User != "" && Token != "" {
		req.SetBasicAuth(User, Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return resp, err
	}
	defer resp.Body.Close() // nolint: errcheck

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}

	if DebugBody {
		fmt.Println(string(data))
	}

	// Some endpoints return 204 when there is no content (e.g. getting
	// information about a repo without any code).
	if resp.StatusCode != http.StatusNoContent {
		err = json.Unmarshal(data, scan)
	}

	if resp.StatusCode >= 400 {
		// Intentionally override the JSON status error; chances are this is the
		// root cause.
		err = ErrNotOK{
			Status:     resp.Status,
			StatusCode: resp.StatusCode,
			Method:     method,
			URL:        url,
		}
	}

	return resp, err
}

// RequestStat is like Request, but retry on 202 response codes.
//
// From the GitHub API docs:
//
// "If the data hasn't been cached when you query a repository's statistics,
// you'll receive a 202 response; a background job is also fired to start
// compiling these statistics. Give the job a few moments to complete, and then
// submit the request again. If the job has completed, that request will receive
// a 200 response with the statistics in the response body."
func RequestStat(scan interface{}, method, url string, maxWait time.Duration) error {
	start := time.Now()
	for {
		if start.Sub(start) > maxWait {
			return errors.New("timed out")
		}

		resp, err := Request(scan, method, url)
		if err != nil {
			if resp.StatusCode == http.StatusAccepted {
				// Ignore json errors on 202; the output is {}, which won't
				// unmarshal in to e.g. an array type.
				if _, ok := err.(*json.UnmarshalTypeError); ok {
					err = nil
				}
			}
			if err != nil {
				return err
			}
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return nil
		case http.StatusAccepted:
			time.Sleep(2 * time.Second)
		case http.StatusNoContent:
			return nil
		}
	}
}

// Repository in GitHub.
type Repository struct {
	Name     string    `json:"name"`
	Language string    `json:"language"`
	PushedAt time.Time `json:"pushed_at"`
	Topics   []string  `json:"topics"`
}

type info struct {
	PublicRepos  int `json:"public_repos"`
	PrivateRepos int `json:"total_private_repos"`
}

// ListRepos lists all repositories for a user or organisation.
//
// The name is in the form of "orgs/OrganisationName" or "user/Username".
//
// This first gets a count of repositories so we can parallelize the pagination,
// which speeds up large organisations/users at the expense of slowing down
// smaller ones.
//
// TODO: try making a more generic pagination function.
func ListRepos(name string) ([]Repository, error) {
	var i info
	_, err := Request(&i, "GET", fmt.Sprintf("%s/%s", API, name))
	if err != nil {
		return nil, err
	}

	var (
		nPages   = int(math.Ceil((float64(i.PublicRepos) + float64(i.PrivateRepos)) / 100.0))
		allRepos []Repository
		errs     []error
		lock     sync.Mutex
		wg       sync.WaitGroup
	)

	wg.Add(nPages)
	for i := 1; i <= nPages; i++ {
		go func(i int) {
			defer wg.Done()

			var repos []Repository
			_, err := Request(&repos, "GET", fmt.Sprintf("%s/%s/repos?per_page=100&page=%d", API, name, i))
			if err != nil {
				lock.Lock()
				errs = append(errs, err)
				lock.Unlock()
				return
			}
			lock.Lock()
			allRepos = append(allRepos, repos...)
			lock.Unlock()
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		return allRepos, fmt.Errorf("%#v", errs)
	}
	return allRepos, err
}

// Copyright 2018 © Martin Tournoij
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
