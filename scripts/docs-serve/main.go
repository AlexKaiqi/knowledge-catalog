package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8766", "address")
	flag.Parse()
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "docs-serve: %v\n", err)
		os.Exit(1)
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docs-serve: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("docs: http://%s/docs/product.html\n", ln.Addr().String())
	log.Fatal(http.Serve(ln, newMux(root)))
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
	}
}
