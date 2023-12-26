// Package upspinfsys provides an implementation of fs.FS interface for Upspin.
// It implements the fs.FS interface and the necessary methods to serve it using
// the http.FileServer too.
//
// Limitations:
//   - The FileMode does not represent the Access file correctly.
package upspinfsys

import (
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"

	"upspin.io/errors"
	"upspin.io/path"
	"upspin.io/upspin"
)

type uFS struct {
	client upspin.Client
}

// UpspinFS returns a fs.FS implementation.
// To use the Open function is necessary to pass the full path of the file
// (the file system is not rooted at client's home).
func UpspinFS(c upspin.Client) fs.FS {
	return uFS{client: c}
}

func (u uFS) Open(name string) (fs.File, error) {
	const op = "open"

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}

	de, err := u.client.Lookup(upspin.PathName(name), true)
	if err != nil {
		switch {
		case errors.Is(errors.NotExist, err):
			err = fs.ErrNotExist
		case errors.Is(errors.Permission, err):
			err = fs.ErrPermission
		default:
			err = fmt.Errorf("failed to lookup file %s: %w", name, err)
		}
		return nil, &fs.PathError{
			Op:   op,
			Path: name,
			Err:  err,
		}
	}

	if de.IsDir() {
		return &dir{de: de, client: u.client}, nil
	}

	if !de.IsRegular() {
		return nil, &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fmt.Errorf("not implemented for non dir or regular files"),
		}
	}

	f, err := u.client.Open(upspin.PathName(name))
	if err != nil {
		return nil, &fs.PathError{
			Op:   op,
			Path: name,
			Err:  fmt.Errorf("failed to open file %s: %w", name, err),
		}
	}
	return file{file: f, de: de}, nil
}

func (u uFS) ReadDir(name string) ([]fs.DirEntry, error) {
	pattern := string(path.Join(upspin.PathName(name), "*"))
	des, err := glob(u.client, pattern)
	if err != nil {
		return nil, fmt.Errorf("readdir: %s: %w", name, err)
	}
	sort.Slice(des, func(i, j int) bool { return des[i].Name() < des[j].Name() })
	return des, nil
}

func (u uFS) Glob(pattern string) ([]string, error) {
	entries, err := u.client.Glob(pattern)
	if err != nil {
		return nil, err
	}
	// Check if len == 0 to return nil, not an empty slice,
	// it's defined in the fs.Glob that should return nil.
	if len(entries) == 0 {
		return nil, nil
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = string(e.Name)
	}
	return names, nil
}

type file struct {
	file upspin.File
	de   *upspin.DirEntry
}

func (f file) Close() error {
	return f.file.Close()
}

func (f file) Read(b []byte) (n int, err error) {
	return f.file.Read(b)
}

func (f file) ReadAt(b []byte, off int64) (n int, err error) {
	return f.file.ReadAt(b, off)
}

func (f file) Seek(offset int64, whence int) (ret int64, err error) {
	return f.file.Seek(offset, whence)
}

func (f file) Stat() (fs.FileInfo, error) {
	return fileInfo(f.de), nil
}

type dir struct {
	client        upspin.Client
	de            *upspin.DirEntry
	entries       []fs.DirEntry
	entriesOffset int
}

func (*dir) Close() error {
	return nil
}

func (d *dir) Read(b []byte) (n int, err error) {
	return 0, fmt.Errorf("read %s: is a directory", d.de.Name)
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return fileInfo(d.de), nil
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.entries != nil {
		if d.entriesOffset == len(d.entries) {
			if n <= 0 {
				return nil, nil
			}
			return nil, io.EOF
		}
		start := d.entriesOffset
		end := start + n

		if n <= 0 || end > len(d.entries) {
			end = len(d.entries)
		}

		out := d.entries[start:end]
		d.entriesOffset = end

		return out, nil
	}

	pattern := string(path.Join(d.de.Name, "*"))
	des, err := glob(d.client, pattern)
	if err != nil {
		return nil, fmt.Errorf("reddir %s: %w", d.de.Name, err)
	}
	d.entries = des

	if n <= 0 || n >= len(d.entries) {
		d.entriesOffset = len(d.entries)
		return d.entries, nil
	}
	d.entriesOffset = n
	return d.entries[:n], nil
}

func glob(c upspin.Client, pattern string) ([]fs.DirEntry, error) {
	entries, err := c.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get dir entries: %w", err)
	}
	des := make([]fs.DirEntry, len(entries))
	for i, e := range entries {
		des[i] = dirEntry(e)
	}
	return des, nil
}

func dirEntry(de *upspin.DirEntry) fs.DirEntry {
	info := fileInfo(de)
	return fs.FileInfoToDirEntry(info)
}

func fileInfo(de *upspin.DirEntry) info {
	size, _ := de.Size()

	fpath := path.DropPath(de.Name, 1)
	name := string(de.Name[len(fpath):])
	if fpath == de.Name {
		name = string(fpath)
		name, _ = strings.CutSuffix(name, "/")
	}
	name, _ = strings.CutPrefix(name, "/")

	// TODO: Think in a way to reflect the actual Access file permissions.
	// Using 0700 gives the owner read and execute permissions, write is not
	// possible because the fs.FS interface is read only.
	var mode fs.FileMode = 0700
	switch {
	case de.IsDir():
		mode |= fs.ModeDir
	case de.IsLink():
		mode |= fs.ModeSymlink
	}

	return info{
		name:    name,
		size:    size,
		mode:    mode,
		modTime: de.Time.Go(),
		isDir:   de.IsDir(),
	}
}

type info struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return i.size }
func (i info) Mode() fs.FileMode  { return i.mode }
func (i info) ModTime() time.Time { return i.modTime }
func (i info) IsDir() bool        { return i.isDir }
func (i info) Sys() any           { return nil }
