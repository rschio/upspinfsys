# upspinfsys
fs.FS implementation for Upspin.

[![Go Reference](https://pkg.go.dev/badge/github.com/rschio/upspinfsys.svg)](https://pkg.go.dev/github.com/rschio/upspinfsys)

```go
package main

import (
	"log"
	"net/http"

	"github.com/rschio/upspinfsys"
	"upspin.io/client"
	"upspin.io/config"
	"upspin.io/transports"
)

func main() {
	cfg, err := config.FromFile("config")
	if err != nil {
		log.Fatal(err)
	}
	transports.Init(cfg)
	c := client.New(cfg)

	fsys := upspinfsys.UpspinFS(c)
	http.Handle("/", http.FileServer(http.FS(fsys)))
	http.ListenAndServe(":8080", nil)
}
```
