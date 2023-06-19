package upspinfsys_test

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"

	"github.com/rschio/upspinfsys"
)

func ExampleFS() {
	fsys := upspinfsys.UpspinFS(c)
	home := string(cfg.UserName())
	path := filepath.Join(home, "documents", "doc1.txt")

	f, err := fsys.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s\n", data)
	// Output:
	// doc1
}

func ExampleHTTPServer() {
	fsys := upspinfsys.UpspinFS(c)
	home := string(cfg.UserName())
	path := filepath.Join(home, "documents", "doc2.txt")

	go func() {
		http.Handle("/", http.FileServer(http.FS(fsys)))
		http.ListenAndServe(":8080", nil)
	}()

	resp, err := http.Get("http://localhost:8080/" + path)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s\n", data)
	// Output:
	// doc2
}
