package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
)

type fileList struct {
	next  *fileList
	isdir bool
	path  string
	size  int64
	file  *os.File
}

var lastFile = &fileList{}

func (firstFile *fileList) String() string {
	s := ""
	for f := firstFile; f != lastFile; f = f.next {
		if f.isdir {
			continue
		}
		s += fmt.Sprintf("\n%s (%d bytes)", f.path, f.size)
	}
	return s
}

func (firstFile *fileList) build(length uint32, total int64, hashes [][20]byte) ([]bool, error) {
	piecesSet := make([]bool, len(hashes))
	i := 0

	buf := make([]byte, length)
	p := buf

	for f := firstFile; f != lastFile; f = f.next {
		if f.isdir {
			err := os.MkdirAll(f.path, 0700)
			if err != nil {
				return []bool{}, fmt.Errorf("creating directory (perm=0700) %s: %s", f.path, err)
			}
			continue
		}

		var err error
		f.file, err = os.OpenFile(f.path, os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			return []bool{}, fmt.Errorf("opening (mode=O_RDWR|O_CREATE,perm=0600) %s: %s", f.path, err)
		}
		err = f.file.Truncate(f.size)
		if err != nil {
			return []bool{}, fmt.Errorf("truncating %s to %d bytes: %s", f.path, f.size, err)
		}

		for {
			n, err := f.file.Read(p)
			if n < len(p) {
				if err != nil && err != io.EOF {
					return []bool{}, fmt.Errorf("reading %s: %s", f.path, err)
				}
				p = p[n:]
				// If err == nil, assume at EOF
				break
			}
			p = buf
			if sha1.Sum(p) == hashes[i] {
				piecesSet[i] = true
			}
			i++
		}
		continue
	}

	p = buf[:len(buf)-len(p)]
	if len(p) > 0 && sha1.Sum(p) == hashes[i] {
		piecesSet[i] = true
	}

	return piecesSet, nil
}
