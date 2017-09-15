package main

import (
	"bufio"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var (
	templates = template.Must(template.ParseFiles("package.tmpl", "index.tmpl", "home.tmpl"))
	style     = mustRead("./style.css")

	groups = []struct {
		Name, Desc string
		Projects   []string
		Packages   []packageT
	}{
		{
			"Libraries",
			"Generic libraries that can be used by any project. Both public and private.",
			[]string{
				"cache", "database", "dlm", "flipout", "go-spamc", "httperr",
				"log", "mailaddress", "test", "utils", "must", "reload", "tnef",
				"geoip", "vat", "goamqp", "israce", "validate", "mailcheckerc",
			},
			[]packageT{},
		},
		{
			"Desk",
			"Teamwork Desk-specific projects",
			[]string{
				"desk", "deskactivity", "deskdocs", "deske2e", "deskedge",
				"deskimporter", "desksentiment", "desksockets", "desktwitter",
				"deskwebhooks", "elasticdesk", "mailchecker",
			},
			[]packageT{},
		},
		{
			"Projects",
			"Teamwork Projects-specific projects.",
			[]string{
				"TeamworkAPIInGO", "projects-api", "projects-webhooks", "projectsapigo", "notification-server",
			},
			[]packageT{},
		},
		{
			"Other",
			"Everything not in one of the other groups.",
			[]string{},
			[]packageT{},
		},
		{
			"Deprecated",
			"Old stuff; don't use unless you really know what you're doing",
			[]string{
				"TeamworkDeskTool", "go-modules", "email",
			},
			[]packageT{},
		},
	}
)

type packageT struct {
	Name           string // Top-level name
	Doc            string // Single-line comment
	Depth          int    // Depth to display it as
	FullImportPath string // Full import path
	RelImportPath  string // Relative import path (may be the same as Full)
}

// Config for godocgen.
type Config struct {
	Outdir     string
	Clonedir   string
	Scan       []string
	RelativeTo string
	MainTitle  string // Title to add in the header, <title> tag, and some other places.

	packages []packageT
}

func main() {
	c := Config{
		Outdir:     "./_site",
		Clonedir:   "./_clone",
		Scan:       []string{"github.com/teamwork/..."},
		RelativeTo: "github.com/teamwork",
		MainTitle:  "Teamwork Go doc",
	}

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
		for i, g := range groups {
			for _, project := range g.Projects {
				if strings.HasPrefix(pkg.RelImportPath, project) {
					groups[i].Packages = append(groups[i].Packages, pkg)
					found = true
					break groups
				}
			}
		}

		if !found {
			groups[3].Packages = append(groups[3].Packages, pkg)
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
		"groups":    groups,
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
