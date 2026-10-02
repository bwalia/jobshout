package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// specBase is the servers[].url path every spec path is relative to.
const specBase = "/api/v1"

var chiMethods = map[string]string{
	"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE",
}

// TestOpenAPIPathsAreRouted fails when api/openapi.yaml documents an operation
// the router does not register, including a path parameter whose name differs
// from the chi one. It reads the routes from this file's source rather than
// building the router, which needs a database and every service.
func TestOpenAPIPathsAreRouted(t *testing.T) {
	routes := routesFromSource(t, "main.go")
	if len(routes) < 100 {
		t.Fatalf("parsed only %d routes from main.go; the route walker is probably broken", len(routes))
	}

	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing openapi.yaml: %v", err)
	}
	if len(spec.Paths) == 0 {
		t.Fatal("openapi.yaml has no paths")
	}

	var missing []string
	for path, ops := range spec.Paths {
		for method := range ops {
			m := strings.ToUpper(method)
			if _, ok := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}[m]; !ok {
				continue
			}
			key := m + " " + normalizePath(specBase+path)
			if !routes[key] {
				missing = append(missing, key)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("openapi.yaml documents operations the router does not register:\n  %s", strings.Join(missing, "\n  "))
	}
}

func routesFromSource(t *testing.T, file string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	var walk func(n ast.Node, prefix string)
	walk = func(n ast.Node, prefix string) {
		ast.Inspect(n, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Route":
				if len(call.Args) == 2 {
					if p, ok := stringLit(call.Args[0]); ok {
						if fn, ok := call.Args[1].(*ast.FuncLit); ok {
							walk(fn.Body, prefix+p)
							return false
						}
					}
				}
			case "Group":
				if len(call.Args) == 1 {
					if fn, ok := call.Args[0].(*ast.FuncLit); ok {
						walk(fn.Body, prefix)
						return false
					}
				}
			default:
				if m, ok := chiMethods[sel.Sel.Name]; ok && len(call.Args) == 2 {
					if p, ok := stringLit(call.Args[0]); ok {
						out[m+" "+normalizePath(prefix+p)] = true
					}
				}
			}
			return true
		})
	}
	walk(f, "")
	return out
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// normalizePath folds chi's "/x" + "/" mounting into "/x".
func normalizePath(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
