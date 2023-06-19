# upspinfsys
fs.FS implementation for Upspin.

[![Go Reference](https://pkg.go.dev/badge/github.com/rschio/upspinfsys.svg)](https://pkg.go.dev/github.com/rschio/upspinfsys)

```go
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
```
