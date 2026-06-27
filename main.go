package main

import (
	_ "embed"
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

//go:embed templates/mapper.tmpl
var mapperTmpl string

type FieldMapping struct {
	From string
	To   string
}

type MapperData struct {
	Package     string
	FromPkgPath string
	FromPkgName string
	FromType    string
	ToType      string
	Fields      []FieldMapping
}

func main() {
	fromFlag := flag.String("from", "", "source type as full import path with type: github.com/user/pkg.TypeName")
	toFlag := flag.String("to", "", "destination type name in current package")
	flag.Parse()

	if *fromFlag == "" || *toFlag == "" {
		log.Fatal("flags -from and -to are required")
	}

	fromPkgPath, fromTypeName, err := splitPkgType(*fromFlag)
	if err != nil {
		log.Fatalf("-from: %v", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports,
		Dir:  workDir(),
	}

	fromPkgs, err := packages.Load(cfg, fromPkgPath)
	if err != nil {
		log.Fatalf("load source package %q: %v", fromPkgPath, err)
	}
	if packages.PrintErrors(fromPkgs) > 0 {
		log.Fatalf("errors in source package %q", fromPkgPath)
	}

	currentPkgs, err := packages.Load(cfg, ".")
	if err != nil {
		log.Fatalf("load current package: %v", err)
	}
	if packages.PrintErrors(currentPkgs) > 0 {
		log.Fatal("errors in current package")
	}

	fromStruct, err := lookupStruct(fromPkgs[0], fromTypeName)
	if err != nil {
		log.Fatalf("source type: %v", err)
	}

	toStruct, err := lookupStruct(currentPkgs[0], *toFlag)
	if err != nil {
		log.Fatalf("destination type: %v", err)
	}

	fields := matchFields(fromStruct, toStruct)
	if len(fields) == 0 {
		log.Fatal("no matching fields found between structs")
	}

	data := MapperData{
		Package:     currentPkgs[0].Name,
		FromPkgPath: fromPkgPath,
		FromPkgName: fromPkgs[0].Name,
		FromType:    fromTypeName,
		ToType:      *toFlag,
		Fields:      fields,
	}

	outFile := "map_" + strings.ToLower(fromTypeName) + "_to_" + strings.ToLower(*toFlag) + ".go"
	if err = generate(data, outFile); err != nil {
		log.Fatalf("generate: %v", err)
	}
}

func workDir() string {
	if f := os.Getenv("GOFILE"); f != "" {
		if abs, err := filepath.Abs(f); err == nil {
			return filepath.Dir(abs)
		}
	}
	dir, _ := os.Getwd()
	return dir
}

func splitPkgType(s string) (pkgPath, typeName string, err error) {
	i := strings.LastIndex(s, ".")
	if i == -1 {
		return "", "", fmt.Errorf("%q must be in format pkg/path.TypeName", s)
	}
	return s[:i], s[i+1:], nil
}

func lookupStruct(pkg *packages.Package, name string) (*types.Struct, error) {
	obj := pkg.Types.Scope().Lookup(name)
	if obj == nil {
		return nil, fmt.Errorf("type %q not found in package %q", name, pkg.PkgPath)
	}
	s, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%q is not a struct", name)
	}
	return s, nil
}

func matchFields(from, to *types.Struct) []FieldMapping {
	toFields := make(map[string]bool, to.NumFields())
	for i := range to.NumFields() {
		toFields[to.Field(i).Name()] = true
	}

	var mappings []FieldMapping
	for i := range from.NumFields() {
		f := from.Field(i)
		if toFields[f.Name()] {
			mappings = append(mappings, FieldMapping{From: f.Name(), To: f.Name()})
		}
	}
	return mappings
}

func generate(data MapperData, outFile string) error {
	tmpl, err := template.New("mapper").Parse(mapperTmpl)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if err = tmpl.Execute(f, data); err != nil {
		f.Close()
		return fmt.Errorf("execute template: %w", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	src, err := os.ReadFile(outFile)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	formatted, err := imports.Process(outFile, src, nil)
	if err != nil {
		return fmt.Errorf("format imports: %w", err)
	}

	if err = os.WriteFile(outFile, formatted, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
