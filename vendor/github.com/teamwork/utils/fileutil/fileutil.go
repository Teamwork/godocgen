// Package fileutil provides high-level file operations.
//
// This code is based on:
// https://github.com/termie/go-shutil
package fileutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// IsSymlink reports if this file is a symbolic link.
func IsSymlink(fi os.FileInfo) bool {
	return (fi.Mode() & os.ModeSymlink) == os.ModeSymlink
}

// CopyFile copies the file data from the file in src to the path in dst.
//
// If followSymlinks is not set and src is a symbolic link, a new symlink will
// be created instead of copying the file it points to.
func CopyFile(src, dst string, followSymlinks bool) error {
	if samefile(src, dst) {
		return &SameFileError{src, dst}
	}

	// Make sure src exists and neither are special files.
	srcStat, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if specialfile(srcStat) {
		return &SpecialFileError{src, srcStat}
	}

	dstStat, err := os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		if specialfile(dstStat) {
			return &SpecialFileError{dst, dstStat}
		}
	}

	// If we don't follow symlinks and it's a symlink, just link it and be done.
	if !followSymlinks && IsSymlink(srcStat) {
		return os.Symlink(src, dst)
	}

	// If we are a symlink, follow it.
	if IsSymlink(srcStat) {
		src, err = os.Readlink(src)
		if err != nil {
			return err
		}
		srcStat, err = os.Stat(src)
		if err != nil {
			return err
		}
	}

	// Do the actual copy.
	fsrc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fsrc.Close()

	fdst, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fdst.Close()

	size, err := io.Copy(fdst, fsrc)
	if err != nil {
		return err
	}

	if size != srcStat.Size() {
		return fmt.Errorf("%s: %d/%d copied", src, size, srcStat.Size())
	}

	return fdst.Close()
}

// CopyMode copies the permission from src to dst.
//
// If followSymlinks is false, symlinks aren't followed if and only if both
// `src` and `dst` are symlinks. If `lchmod` isn't available and both are
// symlinks this does nothing. (I don't think lchmod is available in Go)
func CopyMode(src, dst string, followSymlinks bool) error {
	srcStat, err := os.Lstat(src)
	if err != nil {
		return err
	}

	dstStat, err := os.Lstat(dst)
	if err != nil {
		return err
	}

	// They are both symlinks and we can't change mode on symlinks.
	if !followSymlinks && IsSymlink(srcStat) && IsSymlink(dstStat) {
		return nil
	}

	// Atleast one is not a symlink, get the actual file stats
	srcStat, _ = os.Stat(src)
	return os.Chmod(dst, srcStat.Mode())
}

// Copy data and mode bits ("cp src dst"). Return the file's destination.
//
// The destination may be a directory.
//
// If followSymlinks is false, symlinks won't be followed. This
// resembles GNU's "cp -P src dst".
//
// If source and destination are the same file, a SameFileError will be
// rased.
func Copy(src, dst string, followSymlinks bool) (string, error) {
	dstInfo, err := os.Stat(dst)

	if err == nil && dstInfo.Mode().IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}

	if err != nil && !os.IsNotExist(err) {
		return dst, err
	}

	if err = CopyFile(src, dst, followSymlinks); err != nil {
		return dst, err
	}

	if err = CopyMode(src, dst, followSymlinks); err != nil {
		return dst, err
	}

	return dst, nil
}

// CopyTreeOptions are flags for the CopyTree function.
type CopyTreeOptions struct {
	Symlinks               bool
	IgnoreDanglingSymlinks bool
	CopyFunction           func(string, string, bool) (string, error)
	Ignore                 func(string, []os.FileInfo) []string
}

// CopyTree recursively copies a directory tree.
//
// The destination directory must not already exist.
//
// If the optional Symlinks flag is true, symbolic links in the source tree
// result in symbolic links in the destination tree; if it is false, the
// contents of the files pointed to by symbolic links are copied. If the file
// pointed by the symlink doesn't exist, an error will be returned.
//
// You can set the optional IgnoreDanglingSymlinks flag to true if you want to
// silence this error. Notice that this has no effect on platforms that don't
// support os.Symlink.
//
// The optional ignore argument is a callable. If given, it is called with the
// `src` parameter, which is the directory being visited by CopyTree(), and
// `names` which is the list of `src` contents, as returned by ioutil.ReadDir():
//
//   callable(src, entries) -> ignoredNames
//
// Since CopyTree() is called recursively, the callable will be called once for
// each directory that is copied. It returns a list of names relative to the
// `src` directory that should not be copied.
//
// The optional copyFunction argument is a callable that will be used to copy
// each file. It will be called with the source path and the destination path as
// arguments. By default, Copy() is used, but any function that supports the
// same signature (like Copy2() when it exists) can be used.
func CopyTree(src, dst string, options *CopyTreeOptions) error {
	if options == nil {
		options = &CopyTreeOptions{Symlinks: false,
			Ignore:                 nil,
			CopyFunction:           Copy,
			IgnoreDanglingSymlinks: false}
	}

	srcFileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !srcFileInfo.IsDir() {
		return &NotADirectoryError{src}
	}

	_, err = os.Open(dst)
	if !os.IsNotExist(err) {
		return &AlreadyExistsError{dst}
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcFileInfo.Mode()); err != nil {
		return err
	}

	ignoredNames := []string{}
	if options.Ignore != nil {
		ignoredNames = options.Ignore(src, entries)
	}

	for _, entry := range entries {
		if stringInSlice(entry.Name(), ignoredNames) {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		entryFileInfo, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		// Deal with symlinks
		if IsSymlink(entryFileInfo) {
			linkTo, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if options.Symlinks {
				os.Symlink(linkTo, dstPath)
				//CopyStat(srcPath, dstPath, false)
			} else {
				// ignore dangling symlink if flag is on
				_, err = os.Stat(linkTo)
				if os.IsNotExist(err) && options.IgnoreDanglingSymlinks {
					continue
				}
				_, err = options.CopyFunction(srcPath, dstPath, false)
				if err != nil {
					return err
				}
			}
		} else if entryFileInfo.IsDir() {
			err = CopyTree(srcPath, dstPath, options)
			if err != nil {
				return err
			}
		} else {
			_, err = options.CopyFunction(srcPath, dstPath, false)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
