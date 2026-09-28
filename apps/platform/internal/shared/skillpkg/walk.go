package skillpkg

import "io/fs"

func EachReadableFile(fsys fs.FS, visit func(path string, entry fs.DirEntry)) {
	eachReadableEntry(fsys, func(path string, entry fs.DirEntry) {
		if !entry.IsDir() {
			visit(path, entry)
		}
	})
}

func eachReadableEntry(fsys fs.FS, visit func(path string, entry fs.DirEntry)) {
	_ = fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil {
			visit(path, entry)
		}
		return nil
	})
}
