package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/teamwork/utils/fileutil"

	"arp242.net/sconfig"
)

var (
	templates = template.Must(template.ParseFiles("package.tmpl", "index.tmpl", "home.tmpl"))
	style     = mustRead("./style.css")
)

type packageT struct {
	Name           string // Top-level name
	Doc            string // Single-line comment
	Depth          int    // Depth to display it as
	FullImportPath string // Full import path
	RelImportPath  string // Relative import path (may be the same as Full)
}

type group struct {
	Name, Desc string
	Projects   []string
	Packages   []packageT
}

// Config for godocgen.
type Config struct {
	Organisation []string
	Outdir       string
	Clonedir     string
	Scan         []string
	RelativeTo   string
	MainTitle    string
	User         string
	Pass         string
	Groups       []group
	packages     []packageT
}

func main() {
	// Parse config.
	var c Config
	sconfig.RegisterType("[]main.group", func(v []string) (interface{}, error) {
		g := group{Name: v[0]}

		for i := range v {
			if v[i] == "---" {
				g.Projects = v[i+1:]
				break
			}
			g.Desc += v[i] + " "
		}
		return []group{g}, nil
	})

	if err := sconfig.Parse(&c, "./config", nil); err != nil {
		fmt.Fprintf(os.Stderr, "cannot load config: %v\n", err)
		os.Exit(1)
	}

	// Load GitHub repos.
	repos, err := getRepos(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot get repo list: %v\n", err)
		os.Exit(1)
	}

	repos = repos[0:3] // XXX
	if err := updateRepos(c, repos); err != nil {
		fmt.Fprintf(os.Stderr, "cannot update repo: %v\n", err)
		os.Exit(1)
	}

	// Setup _site
	abs, _ := os.Getwd()
	os.Setenv("GOPATH", filepath.Join(abs, "/", c.Clonedir))
	os.RemoveAll(c.Outdir)
	fileutil.CopyTree("./_static", c.Outdir+"/_static", nil)

	packages, err := list(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list packages: %v\n", err)
		os.Exit(1)
	}

	for _, pkg := range packages {
		err := writePackage(c, pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not write package %v: %v\n", pkg, err)
			os.Exit(1)
		}
	}

	if err := makeIndexes(c); err != nil {
		fmt.Fprintf(os.Stderr, "could not generate index.html files: %v\n", err)
		os.Exit(1)
	}
	if err := makeHome(c, packages); err != nil {
		fmt.Fprintf(os.Stderr, "could not generate index.html files: %v\n", err)
		os.Exit(1)
	}
}

// Clone/update repos.
func updateRepos(c Config, repos []Repository) error {
	orig, _ := os.Getwd()
	defer func() { os.Chdir(orig) }()

	root := filepath.Join(c.Clonedir, "/src/", c.Organisation[0])
	os.MkdirAll(root, 0700)

	for i, r := range repos {
		fmt.Printf(" %v/%v ", i+1, len(repos)+1)

		d := filepath.Join(root, "/", r.Name)
		if s, err := os.Stat(d); err == nil && s.IsDir() {
			fmt.Printf("updating %v                 \r", r.Name)
			os.Chdir(d)
			_, _, err := run("git", "pull", "--quiet")
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("cloning %v                  \r", r.Name)
			os.Chdir(root)
			_, _, err := run("git", "clone", "--depth=1", "--quiet", "git@github.com:Teamwork/"+r.Name)
			if err != nil {
				return err
			}
		}

	}

	fmt.Printf("\n")
	return nil
}

// Write package documentation.
func writePackage(c Config, pkg packageT) error {
	doc, err := godoc(pkg.FullImportPath)
	if err != nil {
		return err
	}

	out := filepath.Join(c.Outdir, filepath.Dir(pkg.RelImportPath), pkg.Name) + "/index.html"
	if err := os.MkdirAll(filepath.Dir(out), 0700); err != nil {
		return err
	}
	fp, err := os.Create(out)
	if err != nil {
		return err
	}

	buf := bufio.NewWriter(fp)
	err = templates.ExecuteTemplate(buf, "package.tmpl", map[string]interface{}{
		"style":     template.CSS(style),
		"godoc":     template.HTML(doc),
		"mainTitle": c.MainTitle,
		"pkg":       pkg,
	})
	if err != nil {
		return err
	}

	if err := buf.Flush(); err != nil {
		return err
	}
	if err := fp.Close(); err != nil {
		return err
	}

	return nil
}

// godoc runs godoc on a package and gets the result.
func godoc(path string) (string, error) {
	// https://go.googlesource.com/tools/+/2d19ab38faf14664c76088411c21bf4fafea960b/godoc/static/
	cmd := exec.Command("godoc", "-html", "-templates", "godoc_tpl", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Remove the first line, which is always:
	//    use 'godoc cmd/fmt' for documentation on the fmt command
	// and always unwanted.
	doc := string(out)
	return doc[strings.Index(doc, "\n"):], nil
}

// Create indexes for packages that don't have one; this happens if there's a
// subdir without any go files.
func makeIndexes(c Config) error {
	return filepath.Walk(c.Outdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if _, stat := os.Stat(filepath.Join(path, "/index.html")); stat == nil {
			return nil
		}

		if path == "./_site" {
			return nil
		}

		path, err = filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("cannot get Abs of %v: %v", path, err)
		}

		if err := makeIndex(c, path); err != nil {
			return fmt.Errorf("cannot make index for %v: %v", path, err)
		}

		return nil
	})
}

// Make the homepage.
func makeHome(c Config, packages []packageT) error {
	// Add to group.
	for _, pkg := range packages {
		found := false
	groups:
		for i, g := range c.Groups {
			for _, project := range g.Projects {
				if strings.HasPrefix(pkg.RelImportPath, project) {
					c.Groups[i].Packages = append(c.Groups[i].Packages, pkg)
					found = true
					break groups
				}
			}
		}

		// TODO: config value
		if !found {
			c.Groups[3].Packages = append(c.Groups[3].Packages, pkg)
		}
	}

	out := filepath.Join(c.Outdir, "/index.html")
	fp, err := os.Create(out)
	if err != nil {
		return err
	}

	buf := bufio.NewWriter(fp)
	err = templates.ExecuteTemplate(buf, "home.tmpl", map[string]interface{}{
		"style":     template.CSS(style),
		"groups":    c.Groups,
		"mainTitle": c.MainTitle,
	})
	if err != nil {
		return err
	}

	if err := buf.Flush(); err != nil {
		return err
	}
	if err := fp.Close(); err != nil {
		return err
	}

	return nil
}

func makeIndex(c Config, path string) error {
	// Get list of all files and dirs.
	contents, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	out := filepath.Join(path, "/index.html")
	fp, err := os.Create(out)
	if err != nil {
		return err
	}

	buf := bufio.NewWriter(fp)
	err = templates.ExecuteTemplate(buf, "index.tmpl", map[string]interface{}{
		"style":     template.CSS(style),
		"contents":  contents,
		"mainTitle": c.MainTitle,
	})
	if err != nil {
		return err
	}

	if err := buf.Flush(); err != nil {
		return err
	}
	if err := fp.Close(); err != nil {
		return err
	}

	return nil
}

func run(cmd ...string) (stdout []string, stderr []string, err error) {
	r := exec.Command(cmd[0], cmd[1:]...)

	// TODO: Read stderr too
	out, err := r.Output()
	if err != nil {
		return nil, nil, err
	}

	return strings.Split(strings.Trim(string(out), "\n"), "\n"), nil, nil
}

func removePathPrefix(full, remove string) string {
	return strings.Trim(strings.Replace(full, remove, "", 1), "/")
}

func mustRead(path string) string {
	fp, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	out, err := ioutil.ReadAll(fp)
	if err != nil {
		panic(err)
	}

	return string(out)
}

func list(c Config) ([]packageT, error) {
	packages := []packageT{}

	for _, dir := range c.Scan {
		stdout, stderr, err := run("go", "list", "-f", "{{.ImportPath}} {{.Doc}}", dir)
		if err != nil {
			return nil, fmt.Errorf("go list error: %v; stderr: %v", err, stderr)
		}

		for _, line := range stdout {
			space := strings.Index(line, " ")
			pkg := packageT{}

			if space == -1 {
				pkg.FullImportPath = strings.TrimSpace(line)
			} else {
				pkg.FullImportPath = line[:space]
				pkg.Doc = line[space+1:]
			}
			pkg.RelImportPath = pkg.FullImportPath
			if c.RelativeTo != "" {
				pkg.RelImportPath = removePathPrefix(pkg.FullImportPath, c.RelativeTo)
			}
			pkg.Name = filepath.Base(pkg.RelImportPath)
			pkg.Depth = len(strings.Split(pkg.RelImportPath, "/"))

			packages = append(packages, pkg)
		}
	}

	// The "go list" tool doesn't list directories without any *.go files; we
	// need to add them since we need it for the UI.
	add := []packageT{}
	for _, p := range packages {
	loopstart:
		up := filepath.Dir(p.FullImportPath)
		if up == "." || up == c.RelativeTo {
			continue
		}

		path := filepath.Join(c.Clonedir, "/src/", up, "/*.go")
		goFiles, _ := filepath.Glob(path)
		if len(goFiles) > 0 {
			continue
		}

		// Don't add duplicates.
		found := false
		for _, a := range add {
			if a.FullImportPath == up {
				found = true
				break
			}
		}
		if found {
			continue
		}
		p = packageT{
			Name:           filepath.Base(up),
			FullImportPath: up,
			RelImportPath:  removePathPrefix(up, c.RelativeTo),
			Depth:          p.Depth - 1,
		}

		add = append(add, p)
		// To-do the loop with "p" set to the newly added package to make sure
		// that all dirs are added.
		goto loopstart
	}
	packages = append(packages, add...)

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].FullImportPath < packages[j].FullImportPath
	})

	// Packages without any subpackages are indent level 0.
	for i := range packages {
		if i+1 == len(packages) {
			break
		}
		//fmt.Println(packages[i].Name, packages[i].Depth, " -> ", packages[i+1].Depth, packages[i+1].Name)
		if packages[i].Depth != 1 {
			continue
		}
		if packages[i+1].Depth == 1 {
			packages[i].Depth = 0
		}
	}

	return packages, nil
}

// Repository in git.
type Repository struct {
	Name     string    `json:"name"`
	Language string    `json:"language"`
	PushedAt time.Time `json:"pushed_at"`
	Topics   []string  `json:"topics"`
}

type requestArgs struct {
	method, url string
	header      http.Header
}

func request(c Config, scan interface{}, args requestArgs) error {
	client := http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest(args.method, args.url, nil)
	if err != nil {
		return err
	}
	if args.header != nil {
		req.Header = args.header
	}

	req.SetBasicAuth(c.User, c.Pass)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	//fmt.Printf("%v\n", string(data))
	return json.Unmarshal(data, scan)
}

// InStringSlice reports whether str is within list
func InStringSlice(list []string, str string) bool {
	for _, item := range list {
		if item == str {
			return true
		}
	}
	return false
}

// Get all Go repos.
func getRepos(c Config) ([]Repository, error) {
	var allRepos []Repository

	page := 1
	for {
		fmt.Println(page)
		var repos []Repository
		err := request(c, &repos, requestArgs{
			method: http.MethodGet,
			url:    fmt.Sprintf("https://api.github.com/organizations/"+c.Organisation[1]+"/repos?per_page=100&page=%v", page),
			header: map[string][]string{
				"Accept": {"application/vnd.github.mercy-preview+json"},
			},
		})
		if err != nil {
			return nil, err
		}

		for _, r := range repos {
			if r.Language == "Go" || InStringSlice(r.Topics, "go") || InStringSlice(r.Topics, "golang") {
				allRepos = append(allRepos, r)
			}
		}

		// XXX
		if true { // len(repos) < 100 || len(repos) == 0 {
			break
		}

		page += 1
	}

	sort.Slice(allRepos, func(i int, j int) bool {
		return allRepos[i].Name < allRepos[j].Name
	})

	return allRepos, nil
}
