// Package godocgen generate self-contained HTML documentation with godoc.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"arp242.net/sconfig"
	"arp242.net/singlepage/singlepage"
	"github.com/PuerkitoBio/goquery"
	"github.com/teamwork/utils/fileutil"
	"github.com/teamwork/utils/sliceutil"
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
	Organisation  []string
	Outdir        string
	Clonedir      string
	Scan          []string
	RelativeTo    string
	MainTitle     string
	User          string
	Pass          string
	Groups        []group
	Exclude       []string
	SkipClone     bool
	HomeText      string
	Bundle        bool
	RewriteSource string
	ShallowClone  bool
}

type options struct {
	config    string
	skipClone bool
}

var templates = template.Must(template.ParseFiles("package.tmpl", "index.tmpl", "home.tmpl"))

func main() {
	// Parse commandline.
	var opts options
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godocgen [flags]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}

	flag.StringVar(&opts.config, "config", "./config", "path to config file")
	flag.BoolVar(&opts.skipClone, "s", false, "skip git clone/pull")
	flag.Parse()

	// Parse config.
	var c Config
	sconfig.RegisterType("[]main.group", func(v []string) (interface{}, error) {
		g := group{Name: v[0]}

		for i := range v[1:] {
			if v[i+1] == "---" {
				g.Projects = v[i+2:]
				break
			}
			g.Desc += v[i+1] + " "
		}
		return []group{g}, nil
	})
	if err := sconfig.Parse(&c, opts.config, nil); err != nil {
		fmt.Fprintf(os.Stderr, "cannot load config: %v\n", err)
		os.Exit(1)
	}
	c.SkipClone = c.SkipClone || opts.skipClone

	if !c.SkipClone {
		if c.Pass == "" {
			c.Pass = os.Getenv("GITHUB_PASS")
			if c.Pass == "" {
				fmt.Fprintf(os.Stderr,
					"No password set; please set 'pass' in config or use the GITHUB_PASS env variable\n")
				os.Exit(1)
			}
		}

		// Load GitHub repos.
		repos, err := getRepos(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot get repo list: %v\n", err)
			os.Exit(1)
		}

		if err := updateRepos(c, repos); err != nil {
			fmt.Fprintf(os.Stderr, "cannot update repo: %v\n", err)
			os.Exit(1)
		}
	}

	// Setup _site
	abs, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	err = os.Setenv("GOPATH", filepath.Join(abs, "/", c.Clonedir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// TODO: exclude .git
	//rm, _ := filepath.Glob(filepath.Join(c.Outdir, "/*"))
	//for _, p := range rm {
	//	os.RemoveAll(p)
	//}
	err = fileutil.CopyTree("./_static", c.Outdir+"/_static", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	packages, err := list(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list packages: %v\n", err)
		os.Exit(1)
	}

	for _, pkg := range packages {
		err := writePackage(c, packages, pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not write package %v: %v\n", pkg.Name, err)
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
	orig, err := os.Getwd()
	if err != nil {
		return err
	}

	orig, err = filepath.Abs(orig)
	if err != nil {
		return err
	}

	var rErr error
	defer func() { rErr = os.Chdir(orig) }()

	root := filepath.Join(c.Clonedir, "/src/github.com/", c.Organisation[0])
	err = os.MkdirAll(root, 0700)
	if err != nil {
		return err
	}

	for i, r := range repos {
		fmt.Printf(" %v/%v ", i+1, len(repos))

		if sliceutil.InStringSlice(c.Exclude, r.Name) {
			fmt.Printf("excluding %v                 \r", r.Name)
			time.Sleep(3 * time.Second)
			continue
		}

		d := filepath.Join(root, "/", r.Name)
		if s, err := os.Stat(d); err == nil && s.IsDir() {
			fmt.Printf("updating %v                 \r", r.Name)
			err = os.Chdir(d)
			if err != nil {
				return err
			}

			_, _, err := run("git", "pull", "--quiet")
			if err != nil {
				return err
			}

			err = os.Chdir(orig)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("cloning %v                  \r", r.Name)
			err = os.Chdir(root)
			if err != nil {
				return err
			}

			cmd := []string{"git", "clone"}
			if c.ShallowClone {
				cmd = append(cmd, "--depth=1")
			}
			_, _, err := run(append(cmd, "--quiet",
				fmt.Sprintf("https://github.com/%v/%v", c.Organisation[0], r.Name))...)
			if err != nil {
				return err
			}

			err = os.Chdir(orig)
			if err != nil {
				return err
			}
		}

	}

	fmt.Printf("\n")
	return rErr
}

// Rewrite source links from:
//  <a href="/src/target/redis.go?s=1187:1246#L39">
//to:
//  <a href="https://github.com/Teamwork/cache/blob/master/redis.go#L39">
var reRewriteSourceGH = regexp.MustCompile(`<a href="/src/target/(.*?\.go)\?s=[0-9:]+#(L\d+)">`)

var reRewriteFileSource = regexp.MustCompile(`<a href="source://(.*?.go)">`)

// Write package documentation.
func writePackage(c Config, packages []packageT, pkg packageT) error {
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

	// Fix source links. By default they're offset by 10; from srcPosLinkFunc()
	// in the godoc source:
	//
	//   if low < high {
	//       fmt.Fprintf(&buf, "?s=%d:%d", low, high) // no need for URL escaping
	//       // if we have a selection, position the page
	//       // such that the selection is a bit below the top
	//       line -= 10
	//       if line < 1 {
	//           line = 1
	//       }
	//   }
	//
	// This looks really confusing on GitHub.
	if c.RewriteSource == "github" {
		doc = reRewriteSourceGH.ReplaceAllStringFunc(doc, func(v string) string {
			match := reRewriteSourceGH.FindAllStringSubmatch(v, -1)[0]
			line, _ := strconv.ParseInt(match[2][1:], 10, 64)
			dir := strings.Replace(pkg.RelImportPath, pkg.Name, "", 1)
			return fmt.Sprintf(`<a href="https://github.com/%v/%v/blob/master/%v%v#L%v">`,
				c.Organisation[0], pkg.RelImportPath, dir, match[1], line+10)
		})

		// Rewrite links to source files
		doc = reRewriteFileSource.ReplaceAllStringFunc(doc, func(v string) string {
			match := reRewriteFileSource.FindAllStringSubmatch(v, -1)[0]
			s := strings.Split(match[1], "/")
			return fmt.Sprintf(`<a href="https://github.com/%v/%v/blob/master/%v">`,
				c.Organisation[0], s[2], strings.Join(s[3:], "/"))
		})
	}

	buf := bufio.NewWriter(fp)
	err = templates.ExecuteTemplate(buf, "package.tmpl", map[string]interface{}{
		"godoc":     template.HTML(doc),
		"mainTitle": c.MainTitle,
		"pkg":       pkg,
		"commit":    gitCommit(c.Clonedir + "/src/" + pkg.FullImportPath),
		"now":       time.Now().Format(time.UnixDate),
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

	b, err := ioutil.ReadFile(out)
	if err != nil {
		return err
	}

	// Remove empty subdirs.
	qdoc, err := goquery.NewDocumentFromReader(bytes.NewReader(b))
	if err != nil {
		return err
	}

	qdoc.Find(".pkg-dir tr").Each(func(i int, s *goquery.Selection) {
		n := s.Find(".pkg-name")
		if n.Length() == 0 {
			return
		}
		name := strings.TrimSpace(n.Text())
		if name == "bin" {
			n.Remove()
		}
		if name == ".." || name == "Name" {
			return
		}

		name = pkg.RelImportPath + "/" + name
		//fmt.Println(name)
		syn := ""
		for _, p := range packages {
			if p.RelImportPath == name {
				syn = p.Doc
				break
			}
		}
		if syn != "" {
			s.Find(".pkg-synopsis").SetText(syn)
		}
	})

	if qdoc.Find(".pkg-dir tr").Length() <= 3 {
		qdoc.Find(".pkg-dir").Remove()
		qdoc.Find(`a[href="#pkg-subdirectories"]`).Parent().Remove()
		qdoc.Find("#pkg-subdirectories").Remove()
	}

	html, err := qdoc.Html()
	if err != nil {
		return err
	}

	// Bundle
	if c.Bundle {
		html, err = singlepage.Bundle(html, singlepage.Everything)
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(out, []byte(html), 0)
}

func gitCommit(path string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// godoc runs godoc on a package and gets the result.
func godoc(path string) (string, error) {
	// https://go.googlesource.com/tools/+/2d19ab38faf14664c76088411c21bf4fafea960b/godoc/static/
	//cmd := exec.Command("godoc", "-url", "/pkg/"+path)
	cmd := exec.Command("godoc", "-html", "-templates", "godoc_tpl", "-analysis", "type,pointer", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not run godoc: %v: %s", err, bytes.Split(out, []byte("\n"))[0])
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

		if path == "./_site" { // TODO: config
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

		// TODO: config value instead of hardcoded "Groups[3]"
		if !found && len(c.Groups) > 3 {
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
		"groups":    c.Groups,
		"mainTitle": c.MainTitle,
		"homeText":  template.HTML(c.HomeText),
	})
	if err != nil {
		return err
	}

	if err := buf.Flush(); err != nil {
		return err
	}

	return fp.Close()
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
		"contents":  contents,
		"mainTitle": c.MainTitle,
	})
	if err != nil {
		return err
	}

	if err := buf.Flush(); err != nil {
		return err
	}

	return fp.Close()
}

func run(cmd ...string) (stdout []string, stderr []string, err error) {
	r := exec.Command(cmd[0], cmd[1:]...)

	outPipe, _ := r.StdoutPipe()
	errPipe, _ := r.StderrPipe()
	defer outPipe.Close() // nolint: errcheck
	defer errPipe.Close() // nolint: errcheck

	err = r.Start()

	out, _ := ioutil.ReadAll(outPipe)
	outerr, _ := ioutil.ReadAll(errPipe)
	return strings.Split(strings.Trim(string(out), "\n"), "\n"),
		strings.Split(strings.Trim(string(outerr), "\n"), "\n"),
		err
}

func removePathPrefix(full, remove string) string {
	return strings.Trim(strings.Replace(full, remove, "", 1), "/")
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
	defer resp.Body.Close() // nolint: errcheck

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

	fmt.Printf("fetching repos from GitHub ... ")

	page := 1
	for {
		fmt.Printf("%v ", page)
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

				// TODO: Don't do this on initial clone
				// TODO: config!
				//if r.PushedAt.After(time.Now().Add(-48 * time.Hour)) {
				allRepos = append(allRepos, r)
				//}
			}
		}

		if len(repos) < 100 || len(repos) == 0 {
			break
		}

		page++
	}

	fmt.Println(" done")
	sort.Slice(allRepos, func(i int, j int) bool {
		return allRepos[i].Name < allRepos[j].Name
	})

	return allRepos, nil
}
