// pygo docs — Generates HTML documentation from Go code (stdlib only).
package main

import (
	"flag"
	"fmt"
	"go/doc"
	"go/parser"
	"go/token"
	"html/template"
	"log"
	"os"
	"path/filepath"
)

var docTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}} — PyGo Docs</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/water.css@11/water.min.css">
  <style>.pygo-header{color:#2563EB;border-bottom:3px solid #2563EB;padding-bottom:.5rem}body a{color:#2563EB}</style>
</head>
<body>
  <nav class="pygo-header"><a href="index.html">PyGo Framework Docs</a> · v2.0</nav>
  <main>
    <h1>{{.Title}}</h1>
    <p><code>{{.Package}}</code></p>
    {{if .Doc}}<div>{{.Doc}}</div>{{end}}
    {{if .Types}}<h2>Types</h2>
    {{range .Types}}<h3><code>{{.Name}}</code></h3><p>{{.Doc}}</p>{{end}}{{end}}
    {{if .Funcs}}<h2>Functions</h2>
    {{range .Funcs}}<h3><code>{{.Name}}</code></h3><p>{{.Doc}}</p>{{end}}{{end}}
  </main>
</body>
</html>`

var indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>PyGo Framework Docs</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/water.css@11/water.min.css">
  <style>.pygo-header{color:#2563EB;border-bottom:3px solid #2563EB;padding-bottom:.5rem}</style>
</head>
<body>
  <nav class="pygo-header">PyGo Framework Docs · v2.0</nav>
  <main>
    <h1>PyGo Framework v2.0</h1>
    <ul>{{range .Packages}}<li><a href="{{.Link}}.html">{{.Name}}</a> — {{.Doc}}</li>{{end}}</ul>
  </main>
</body>
</html>`

type pageContext struct {
	Title, Package, Doc string
	Types               []typeDoc
	Funcs               []funcDoc
}
type typeDoc struct{ Name, Doc string }
type funcDoc struct{ Name, Doc string }
type pkgDoc struct{ Name, Doc, Path, Link string }

func runDocs(args []string) error {
	fs := flag.NewFlagSet("docs", flag.ExitOnError)
	output := fs.String("output", "docs", "Output directory")
	help := fs.Bool("help", false, "Show help")
	fs.Parse(args)
	if *help || len(args) > 0 && args[0] == "--help" {
		fmt.Println("pygo docs — Generate HTML docs from Go packages\nUsage: pygo docs [--output DIR]\nDefault: ./docs/")
		return nil
	}
	dir := *output
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	fset := token.NewFileSet()
	dirs, err := findGoDirs(".")
	if err != nil {
		return err
	}
	var pkgs []pkgDoc
	for _, d := range dirs {
		pkgs = append(pkgs, genPkgDocs(fset, d, dir)...)
	}
	renderIndex(dir, pkgs)
	log.Printf("[docs] Generated %d package pages in %s/", len(pkgs), dir)
	return nil
}

func findGoDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		base := info.Name()
		if base == "vendor" || base == "node_modules" || base == "docs" || base == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			matched, _ := filepath.Glob(filepath.Join(path, "*.go"))
			if len(matched) > 0 {
				dirs = append(dirs, path)
			}
		}
		return nil
	})
	return dirs, err
}

func genPkgDocs(fset *token.FileSet, dir, outDir string) []pkgDoc {
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil || len(pkgs) == 0 {
		return nil
	}
	var result []pkgDoc
	tmpl, _ := template.New("page").Parse(docTemplate)
	for name, astPkg := range pkgs {
		if name == "main" || name == "test" {
			name = astPkg.Name
		}
		d := doc.New(astPkg, name, 0)
		link := safeName(name)
		page := pageContext{
			Title:   name,
			Package: name,
			Doc:     trimDoc(d.Doc),
		}
		for _, t := range d.Types {
			page.Types = append(page.Types, typeDoc{Name: t.Name, Doc: trimDocShort(t.Doc)})
		}
		for _, f := range d.Funcs {
			page.Funcs = append(page.Funcs, funcDoc{Name: f.Name, Doc: trimDocShort(f.Doc)})
		}
		f, _ := os.Create(filepath.Join(outDir, link+".html"))
		if f != nil {
			tmpl.Execute(f, page)
			f.Close()
		}
		result = append(result, pkgDoc{
			Name:  name,
			Doc:   trimDocShort(d.Doc),
			Path:  dir,
			Link:  link,
		})
		log.Printf("[docs] %s → %s.html", name, link)
	}
	return result
}

func renderIndex(dir string, pkgs []pkgDoc) {
	tmpl, _ := template.New("index").Parse(indexTemplate)
	f, _ := os.Create(filepath.Join(dir, "index.html"))
	if f != nil {
		tmpl.Execute(f, map[string]interface{}{"Packages": pkgs})
		f.Close()
	}
}

func trimDoc(s string) string {
	if s == "" {
		return "(sin documentación)"
	}
	return s
}

func trimDocShort(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func safeName(s string) string {
	return s
}
