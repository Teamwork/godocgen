package fileutil

import "os"

func samefile(src string, dst string) bool {
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	return os.SameFile(srcInfo, dstInfo)
}

func specialfile(fi os.FileInfo) bool {
	return (fi.Mode() & os.ModeNamedPipe) == os.ModeNamedPipe
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
