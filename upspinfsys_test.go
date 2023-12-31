package upspinfsys_test

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/rschio/upspinfsys"
	"upspin.io/client"
	"upspin.io/config"
	"upspin.io/path"
	"upspin.io/transports"
	"upspin.io/upbox"
	"upspin.io/upspin"
)

var (
	c   upspin.Client
	cfg upspin.Config
)

func TestMain(m *testing.M) {
	var cleanup func()
	c, cfg, cleanup = newClient()
	defer cleanup()
	createDirTree(c, cfg)

	m.Run()
}

func TestStd(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)

	var (
		documents = "documents"
		photos    = filepath.Join(documents, "photos")
		code      = "code"
	)
	expected := []string{
		documents, photos, code,
		"rootfile.txt",
		filepath.Join(documents, "doc1.txt"),
		filepath.Join(documents, "doc2.txt"),
		filepath.Join(documents, "link_text"),
		filepath.Join(documents, "link_link_text"),
		filepath.Join(code, "main.go"),
		filepath.Join(code, "go.mod"),
		filepath.Join(code, "text.txt"),
	}

	// To use fstest.TestFS is necessary that "." dir works.
	// Upspin does not have a root dir, each user has its own home dir
	// and it's impossible or at least not practical to list all Upspin users
	// so a ReadDir(".") would not be possible.
	// One option to make "." is to root it at client's home dir
	// e.g. "user@example.com", but this solution does not make possible to
	// list other users content.
	// The solution found is: root at "" and use
	// fs.Sub(fsys, "user@example.com") to test, so we can test against a good
	// test lib and use full Upspin power.
	fsys, _ = fs.Sub(fsys, "user@example.com")

	if err := fstest.TestFS(fsys, expected...); err != nil {
		t.Fatalf("fstest: %v", err)
	}
}

func TestDir(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	visited := map[string]bool{
		filepath.Join(root):                                false,
		filepath.Join(root, "rootfile.txt"):                false,
		filepath.Join(root, "documents"):                   false,
		filepath.Join(root, "documents", "photos"):         false,
		filepath.Join(root, "documents", "doc1.txt"):       false,
		filepath.Join(root, "documents", "doc2.txt"):       false,
		filepath.Join(root, "documents", "link_text"):      false,
		filepath.Join(root, "documents", "link_link_text"): false,
		filepath.Join(root, "code"):                        false,
		filepath.Join(root, "code", "main.go"):             false,
		filepath.Join(root, "code", "go.mod"):              false,
		filepath.Join(root, "code", "text.txt"):            false,
	}
	wantLen := len(visited)

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited[path] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walkdir: %v", err)
	}

	if len(visited) != wantLen {
		t.Errorf("got wrong number of files, got %d want %d", len(visited), wantLen)
	}
	for path, ok := range visited {
		if !ok {
			t.Errorf("path %s was not visited", path)
		}
	}
}

func TestUpspinFSDirRead(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	f, err := fsys.Open(root)
	if err != nil {
		t.Fatalf("failed to open root: %v", err)
	}
	if _, err := io.ReadAll(f); err == nil {
		t.Fatal("should return error when reading a dir")
	}
}

func TestReadDirFile(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	f, err := fsys.Open(filepath.Join(root, "documents"))
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	dir, ok := f.(fs.ReadDirFile)
	if !ok {
		t.Fatal("should implement fs.ReadDirFile")
	}

	des, err := dir.ReadDir(1)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(des) != 1 {
		t.Fatalf("should return 1 entry, got %d", len(des))
	}
	des2, err := dir.ReadDir(-1)
	if err != nil {
		t.Fatalf("read dir(-1): %v", err)
	}
	if len(des2) != 4 {
		t.Fatalf("should return 4 entry, got %d", len(des2))
	}
}

func TestLink(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	target, err := fsys.Open(filepath.Join(root, "code", "text.txt"))
	if err != nil {
		t.Fatalf("Open link: %v", err)
	}
	tContent, err := io.ReadAll(target)
	target.Close()
	if err != nil {
		t.Fatalf("ReadAll target: %v", err)
	}

	l, err := fsys.Open(filepath.Join(root, "documents", "link_text"))
	if err != nil {
		t.Fatalf("Open link: %v", err)
	}

	got, err := io.ReadAll(l)
	if err != nil {
		t.Fatalf("link ReadAll: %v", err)
	}
	if !bytes.Equal(got, tContent) {
		t.Fatalf("got %s want %s", got, tContent)
	}

	linkOfLink, err := fsys.Open(filepath.Join(root, "documents", "link_link_text"))
	if err != nil {
		t.Fatalf("Open linkOfLink: %v", err)
	}

	got, err = io.ReadAll(linkOfLink)
	if err != nil {
		t.Fatalf("linkOfLink ReadAll: %v", err)
	}
	if !bytes.Equal(got, tContent) {
		t.Fatalf("got %s want %s", got, tContent)
	}

}

func TestLinkDir(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	// Link to dir
	linkDir := filepath.Join(root, "documents", "link_dir")
	codeDir := filepath.Join(root, "code")
	// Create here to avoid fstest.TestFS:
	// Ignore link_dir: https://github.com/golang/go/issues/50401
	if _, err := c.PutLink(upspin.PathName(codeDir), upspin.PathName(linkDir)); err != nil {
		log.Fatalf("failed to create link_dir: %v", err)
	}

	want, err := fs.ReadDir(fsys, codeDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	got, err := fs.ReadDir(fsys, linkDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := 0; i < len(got); i++ {
		if got[i].Name() != want[i].Name() {
			t.Errorf("got %q want %q", got[i].Name(), want[i].Name())
		}
	}

}

func createDirTree(c upspin.Client, cfg upspin.Config) {
	var (
		root      = upspin.PathName(cfg.UserName())
		documents = path.Join(root, "documents")
		photos    = path.Join(documents, "photos")
		code      = path.Join(root, "code")
	)
	if _, err := c.MakeDirectory(root); err != nil {
		log.Fatalf("failed to create root dir: %v", err)
	}
	if _, err := c.MakeDirectory(documents); err != nil {
		log.Fatalf("failed to create documents dir: %v", err)
	}
	if _, err := c.MakeDirectory(photos); err != nil {
		log.Fatalf("failed to create photos dir: %v", err)
	}
	if _, err := c.MakeDirectory(code); err != nil {
		log.Fatalf("failed to create code dir: %v", err)
	}

	if _, err := c.Put(path.Join(root, "rootfile.txt"), []byte("rootfile")); err != nil {
		log.Fatalf("failed to create rootfile.txt: %v", err)
	}
	if _, err := c.Put(path.Join(documents, "doc1.txt"), []byte("doc1")); err != nil {
		log.Fatalf("failed to create doc1.txt: %v", err)
	}
	if _, err := c.Put(path.Join(documents, "doc2.txt"), []byte("doc2")); err != nil {
		log.Fatalf("failed to create doc2.txt: %v", err)
	}
	if _, err := c.Put(path.Join(code, "main.go"), []byte("package main")); err != nil {
		log.Fatalf("failed to create main.go: %v", err)
	}
	if _, err := c.Put(path.Join(code, "go.mod"), []byte("module fake")); err != nil {
		log.Fatalf("failed to create go.mod: %v", err)
	}
	if _, err := c.Put(path.Join(code, "text.txt"), []byte("text")); err != nil {
		log.Fatalf("failed to create text.txt: %v", err)
	}

	// Link
	if _, err := c.PutLink(path.Join(code, "text.txt"), path.Join(documents, "link_text")); err != nil {
		log.Fatalf("failed to create link_text: %v", err)
	}
	// Link to link
	if _, err := c.PutLink(path.Join(documents, "link_text"), path.Join(documents, "link_link_text")); err != nil {
		log.Fatalf("failed to create link_link_text: %v", err)
	}
}

const schema = `
users:
  - name: user

servers:
  - name: keyserver
  - name: storeserver
  - name: dirserver

domain: example.com
`

func newClient() (upspin.Client, upspin.Config, func()) {
	sc, err := upbox.SchemaFromYAML(schema)
	if err != nil {
		log.Fatalf("failed to parse schema: %v", err)
	}

	if err := sc.Start(); err != nil {
		log.Fatalf("failed to start schema: %v", err)
	}
	cleanup := func() { sc.Stop() }

	cfg, err := config.FromFile(sc.Config(sc.Users[0].Name))
	if err != nil {
		cleanup()
		log.Fatalf("failed to parse config: %v", err)
	}

	transports.Init(cfg)
	c := client.New(cfg)

	return c, cfg, cleanup
}
