package fileutil

import (
	"fmt"
	"os"
)

// SameFileError is used when the source and destination file are the same file.
type SameFileError struct {
	Src string
	Dst string
}

func (e SameFileError) Error() string {
	return fmt.Sprintf("%s and %s are the same file", e.Src, e.Dst)
}

// SpecialFileError is used when the source or destination file is a special
// file, and not something we can operate on.
type SpecialFileError struct {
	File     string
	FileInfo os.FileInfo
}

func (e SpecialFileError) Error() string {
	return fmt.Sprintf("`%s` is a named pipe", e.File)
}

// NotADirectoryError is used when attempting to copy a directory tree that is
// not a directory.
type NotADirectoryError struct {
	Src string
}

func (e NotADirectoryError) Error() string {
	return fmt.Sprintf("`%s` is not a directory", e.Src)
}

// AlreadyExistsError is used when the destination already exists.
type AlreadyExistsError struct {
	Dst string
}

func (e AlreadyExistsError) Error() string {
	return fmt.Sprintf("`%s` already exists", e.Dst)
}
