package skillpkg

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

type lockedDirFS struct {
	fstest.MapFS
	locked string
}

func (f lockedDirFS) Open(name string) (fs.File, error) {
	if name == f.locked {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.Open(name)
}

func (f lockedDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.locked {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.ReadDir(name)
}

type unreadableRootFS struct{}

func (unreadableRootFS) Open(string) (fs.File, error) { return nil, errors.New("the archive is gone") }

func treeWithALockedDirectory() fs.FS {
	return lockedDirFS{locked: "locked", MapFS: fstest.MapFS{
		"a.txt":        {Data: []byte("a")},
		"dir/b.txt":    {Data: []byte("b")},
		"locked/c.txt": {Data: []byte("c")},
	}}
}

func TestTheWalkVisitsEachReadableEntryOnceAndNothingBehindALockedDirectory(t *testing.T) {
	var entries []string
	eachReadableEntry(treeWithALockedDirectory(), func(path string, _ fs.DirEntry) {
		entries = append(entries, path)
	})

	want := []string{".", "a.txt", "dir", "dir/b.txt", "locked"}
	if !slices.Equal(entries, want) {
		t.Errorf("visited %v, want %v", entries, want)
	}
}

func TestTheFileWalkLeavesDirectoriesOut(t *testing.T) {
	var files []string
	EachReadableFile(treeWithALockedDirectory(), func(path string, entry fs.DirEntry) {
		if entry.IsDir() {
			t.Errorf("%s is a directory", path)
		}
		files = append(files, path)
	})

	want := []string{"a.txt", "dir/b.txt"}
	if !slices.Equal(files, want) {
		t.Errorf("visited %v, want %v", files, want)
	}
}

func TestAnUnreadableRootIsAnEmptyWalk(t *testing.T) {
	visits := 0
	eachReadableEntry(unreadableRootFS{}, func(string, fs.DirEntry) { visits++ })
	if visits != 0 {
		t.Errorf("visited %d entries of a tree that cannot be opened, want 0", visits)
	}
}
