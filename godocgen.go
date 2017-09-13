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

type packageT struct {
	Name, Doc, Depth string
}

var (
	templates = template.Must(template.ParseFiles("package.tmpl", "index.tmpl", "home.tmpl"))
	style     = mustRead("./style.css")

	groups = []struct {
		Name, Desc string
		Projects   []string
		Packages   []packageT
	}{
		{
			"libraries",
			"Generic libraries that can be used by any project. Both public and private.",
			[]string{
				"cache", "database", "dlm", "flipout", "go-spamc", "httperr",
				"log", "mailaddress", "test", "utils", "must", "reload", "tnef",
				"geoip", "vat", "goamqp",
			},
			[]packageT{},
		},
		{
			"desk",
			"Teamwork Desk-specific projects",
			[]string{
				"desk", "deskactivity", "deskdocs", "deske2e", "deskedge",
				"deskimporter", "desksentiment", "desksockets", "desktwitter",
				"deskwebhooks", "elasticdesk", "mailchecker",
			},
			[]packageT{},
		},
		{
			"projects",
			"Teamwork Projects-specific projects.",
			[]string{
				"TeamworkAPIInGO", "projects-api", "projects-webhooks", "projectsapigo", "notification-server",
			},
			[]packageT{},
		},
		{
			"other",
			"Everything not in one of the other groups.",
			[]string{},
			[]packageT{},
		},
		{
			"deprecated",
			"Old stuff; don't use unless you really know what you're doing",
			[]string{
				"TeamworkDeskTool", "go-modules", "email",
			},
			[]packageT{},
		},
	}
)

func main() {
	outdir := "./_site"
	scan := []string{"github.com/teamwork/..."}
	if len(os.Args) > 2 {
		scan = os.Args[2:]
	}

	packages, err := list(scan...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot list packages: %v\n", err)
		os.Exit(1)
	}

	for _, pkg := range packages {
		err := writePackage(outdir, pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not write package %v: %v\n", pkg, err)
			os.Exit(1)
		}
	}

	if err := makeIndexes(outdir); err != nil {
		fmt.Fprintf(os.Stderr, "could not generate index.html files: %v\n", err)
		os.Exit(1)
	}

	if err := makeHome(outdir, scan); err != nil {
		fmt.Fprintf(os.Stderr, "could not generate index.html files: %v\n", err)
		os.Exit(1)
	}

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

// list all packages in "dirs", where every entry is an argument to "go list".
func list(dirs ...string) ([]string, error) {
	packages := []string{}

	for _, dir := range dirs {
		cmd := exec.Command("go", "list", dir)

		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		packages = append(packages, strings.Split(strings.TrimSpace(string(out)), "\n")...)
	}

	return packages, nil
}

func listDocs(dirs ...string) ([][]string, error) {
	packages := [][]string{}

	for _, dir := range dirs {
		cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{.Doc}}", dir)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("listDocs error: %v; output: %s", err, out)
		}

		pkgs := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, importPath := range pkgs {
			p := strings.Replace(importPath, "github.com/teamwork/", "", 1)
			space := strings.Index(p, " ")
			if space == -1 {
				packages = append(packages, []string{p, "", "0", importPath})
			} else {
				packages = append(packages, []string{p[:space], p[space:], "0", importPath})
			}
		}
	}

	// go list won't list dirs without Go files, so e.g.
	// github.com/teamwork/TeamworkAPIInGO/app
	// Would show as just "app", since "TeamworkAPIInGO" has no Go files.
	add := [][]string{}
	for _, p := range packages {
		d := filepath.Dir(p[0])
		if d == "." {
			continue
		}

		goFiles, _ := filepath.Glob(filepath.Join("./_clone/src/github.com/teamwork/", d, "/*.go"))
		if len(goFiles) > 0 {
			continue
		}

		found := false
		for _, a := range add {
			if a[0] == d {
				found = true
				break
			}
		}
		if found {
			continue
		}

		add = append(add, []string{d, "",
			// I think I have officially discovered the best way of doing
			// subtraction in Go.
			fmt.Sprintf("%s", []byte{[]byte(p[2])[0] - 1}),
			d})
	}

	packages = append(packages, add...)

	sort.Slice(packages, func(i, j int) bool {
		return packages[i][0] < packages[j][0]
	})

	// Reduce to just package name and record indent level.
	for i := range packages {
		packages[i][2] = fmt.Sprintf("%d", len(strings.Split(packages[i][0], "/")))
		packages[i][0] = filepath.Base(packages[i][0])

		packages[i][3] = strings.Replace(strings.TrimSpace(packages[i][3]), "github.com/teamwork/", "", 1)
	}

	// Indent level 0 for packages with 0 subpackages
	for i := range packages {
		if i+1 == len(packages) {
			break
		}
		if packages[i][2] != "1" {
			continue
		}
		if packages[i+1][2] == "1" {
			packages[i][2] = "0"
		}
	}

	return packages, nil
}

// godoc runs godoc on a package and gets the result.
func godoc(pkg string) (string, error) {
	// https://go.googlesource.com/tools/+/2d19ab38faf14664c76088411c21bf4fafea960b/godoc/static/
	cmd := exec.Command("godoc", "-html", "-templates", "godoc_tpl", pkg)
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

// Write package documentation.
func writePackage(outdir, pkg string) error {
	doc, err := godoc(pkg)
	if err != nil {
		return err
	}

	out := filepath.Join(outdir, filepath.Dir(pkg), filepath.Base(pkg)) + "/index.html"
	relto := "github.com/teamwork/"
	if relto != "" {
		relpkg := pkg[len(relto):]
		if relpkg == relto {
			return nil
		}
		out = filepath.Join(outdir, filepath.Dir(relpkg), filepath.Base(relpkg)) + "/index.html"
	}

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
		"pkgHead":   filepath.Base(pkg),
		"pkgFull":   pkg,
		"mainTitle": "Teamwork Go doc",
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

// Create indexes for packages that don't have one; this happens if there's a
// subdir without any go files.
func makeIndexes(outdir string) error {
	return filepath.Walk(outdir, func(path string, info os.FileInfo, err error) error {
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

		if err := makeIndex(path); err != nil {
			return fmt.Errorf("cannot make index for %v: %v", path, err)
		}

		return nil
	})
}

// Make the homepage.
func makeHome(path string, pkgs []string) error {
	// Get list of all files and dirs.
	contents, err := listDocs(pkgs...)
	if err != nil {
		return err
	}

	// Add to group.
	for _, pkg := range contents {
		found := false
	groups:
		for i, g := range groups {
			for _, project := range g.Projects {
				if strings.HasPrefix(pkg[3], project) {
					groups[i].Packages = append(groups[i].Packages, packageT{
						Name:  pkg[0],
						Doc:   pkg[1],
						Depth: pkg[2],
					})
					found = true
					break groups
				}
			}
		}

		if !found {
			groups[3].Packages = append(groups[3].Packages, packageT{
				Name:  pkg[0],
				Doc:   pkg[1],
				Depth: pkg[2],
			})
		}
	}

	out := filepath.Join(path, "/index.html")
	fp, err := os.Create(out)
	if err != nil {
		return err
	}

	buf := bufio.NewWriter(fp)
	err = templates.ExecuteTemplate(buf, "home.tmpl", map[string]interface{}{
		"style":     template.CSS(style),
		"groups":    groups,
		"mainTitle": "Teamwork Go doc",
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

func makeIndex(path string) error {
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
		"style":    template.CSS(style),
		"contents": contents,
		//"godoc":     template.HTML(doc),
		//"pkgHead":   filepath.Base(pkg),
		//"pkgFull":   pkg,
		"mainTitle": "Teamwork Go doc",
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
