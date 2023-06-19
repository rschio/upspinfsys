package upspinfsys_test

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
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

func TestGlob(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	matches, err := fs.Glob(fsys, root+"/*/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"user@example.com/code/text.txt",
		"user@example.com/documents/doc1.txt",
		"user@example.com/documents/doc2.txt",
	}
	sort.Strings(matches)
	if diff := cmp.Diff(matches, want); diff != "" {
		t.Fatalf("got wrong Glob, diff: %s", diff)
	}

}

func TestDir(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	visited := map[string]bool{
		filepath.Join(root):                          false,
		filepath.Join(root, "rootfile.txt"):          false,
		filepath.Join(root, "documents"):             false,
		filepath.Join(root, "documents", "photos"):   false,
		filepath.Join(root, "documents", "doc1.txt"): false,
		filepath.Join(root, "documents", "doc2.txt"): false,
		filepath.Join(root, "code"):                  false,
		filepath.Join(root, "code", "main.go"):       false,
		filepath.Join(root, "code", "go.mod"):        false,
		filepath.Join(root, "code", "text.txt"):      false,
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

func TestUpspinFSRead(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	fname := filepath.Join(root, "rootfile.txt")
	f, err := fsys.Open(fname)
	if err != nil {
		t.Fatalf("failed to open rootfile.txt on fs: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read rootfile.txt: %v", err)
	}
	if string(data) != "rootfile" {
		t.Fatalf("read wrong content from rootfile.txt, got: %q", data)
	}

	seeker := f.(io.Seeker)
	if _, err := seeker.Seek(1, 0); err != nil {
		t.Fatalf("failed to seek file: %v", err)
	}
	data, err = io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read rootfile.txt: %v", err)
	}
	if string(data) != "ootfile" {
		t.Fatalf("read wrong content from rootfile.txt, got: %q", data)
	}

	if _, err := seeker.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek file: %v", err)
	}

	var buf [4]byte
	ra := f.(io.ReaderAt)
	n, err := ra.ReadAt(buf[:], 4)
	if err != nil {
		t.Fatalf("failed to read at: %v", err)
	}
	if string(buf[:n]) != "file" {
		t.Fatalf("readAt wrong content from rootfile.txt, got: %q", buf[:n])
	}

	_, err = fsys.Open(filepath.Join(root, "noexist.txt"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("should return ErrNotExist, got: %v", err)
	}

	f, err = fsys.Open(root)
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
	if len(des2) != 2 {
		t.Fatalf("should return 2 entry, got %d", len(des2))
	}
	if _, err := dir.ReadDir(-1); err != io.EOF {
		t.Fatalf("should return io.EOF, got: %v", err)
	}

	deNames := make([]string, 0)
	for _, de := range des {
		deNames = append(deNames, de.Name())
	}
	for _, de := range des2 {
		deNames = append(deNames, de.Name())
	}

	want := []string{
		"doc2.txt",
		"doc1.txt",
		"photos",
	}
	trans := cmp.Transformer("Sort", func(in []string) []string {
		out := append([]string(nil), in...)
		sort.Strings(out)
		return out
	})
	if diff := cmp.Diff(deNames, want, trans); diff != "" {
		t.Fatalf("got wrong entry names: %s", diff)
	}

	f, err = fsys.Open(filepath.Join(root, "code"))
	if err != nil {
		t.Fatalf("failed to open code: %v", err)
	}
	dir, ok = f.(fs.ReadDirFile)
	if !ok {
		t.Fatal("should implement fs.ReadDirFile")
	}
	des, err = dir.ReadDir(0)
	if err != nil {
		t.Fatalf("failed to read dir(0): %v", err)
	}
	if len(des) != 3 {
		t.Fatalf("got %d entries, want 3", len(des))
	}
}

func TestFileInfo(t *testing.T) {
	fsys := upspinfsys.UpspinFS(c)
	root := string(cfg.UserName())

	f, err := fsys.Open(filepath.Join(root, "rootfile.txt"))
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}
	if info.Name() != "rootfile.txt" {
		t.Fatalf("info.Name() returned the wrong name. Got: %q, want %q",
			info.Name(), "rootfile.txt")
	}
	if info.Size() != int64(len("rootfile")) {
		t.Fatalf("wrong file size, got %d, want %d", info.Size(), len("rootfile"))
	}
	if info.Mode() != fs.ModeIrregular {
		t.Fatalf("should return Mode() == fs.ModeIrregular")
	}
	if info.IsDir() {
		t.Fatalf("should return IsDir() == false")
	}
	if info.Sys() != nil {
		t.Fatalf("should return Sys() == nil")
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
