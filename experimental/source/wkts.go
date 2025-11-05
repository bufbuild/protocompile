package source

import "github.com/bufbuild/protocompile/wellknownimports"

var wktFS = FS{FS: wellknownimports.FS()}

// WKTs returns an opener that yields protocompile's built-in WKT sources.
func WKTs() Opener {
	// All openers returned by this function compare equal.
	return wkts{}
}

type wkts struct{}

func (wkts) Open(path string) (*File, error) {
	file, err := wktFS.Open(path)
	if err != nil {
		return nil, err
	}

	file.path = "<built-in>/" + path
	return file, nil
}
