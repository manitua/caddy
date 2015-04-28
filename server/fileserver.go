package server

import (
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/mholt/caddy/middleware"
	"github.com/mholt/caddy/middleware/browse"
)

// This FileServer is adapted from the one in net/http by
// the Go authors. Significant modifications have been made.
//
//
// License:
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
func FileServer(root http.FileSystem, hide []string) middleware.Handler {
	return &fileHandler{root: root, hide: hide}
}

type fileHandler struct {
	root http.FileSystem
	hide []string // list of files to treat as "Not Found"
}

func (f *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) (int, error) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
	return f.serveFile(w, r, path.Clean(upath))
}

// name is '/'-separated, not filepath.Separator.
func (fh *fileHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) (int, error) {
	f, err := fh.root.Open(name)
	if err != nil {
		if os.IsPermission(err) {
			return http.StatusForbidden, err
		}
		return http.StatusNotFound, nil
	}
	defer f.Close()

	d, err1 := f.Stat()
	if err1 != nil {
		if os.IsPermission(err) {
			return http.StatusForbidden, err
		}
		return http.StatusNotFound, nil
	}

	// redirect to canonical path
	url := r.URL.Path
	if d.IsDir() {
		// Ensure / at end of directory url
		if url[len(url)-1] != '/' {
			redirect(w, r, path.Base(url)+"/")
			return http.StatusMovedPermanently, nil
		}
	} else {
		// Ensure no / at end of file url
		if url[len(url)-1] == '/' {
			redirect(w, r, "../"+path.Base(url))
			return http.StatusMovedPermanently, nil
		}
	}

	// use contents of an index file, if present, for directory
	if d.IsDir() {
		for _, indexPage := range browse.IndexPages {
			index := strings.TrimSuffix(name, "/") + "/" + indexPage
			ff, err := fh.root.Open(index)
			if err == nil {
				defer ff.Close()
				dd, err := ff.Stat()
				if err == nil {
					name = index
					d = dd
					f = ff
					break
				}
			}
		}
	}

	// Still a directory? (we didn't find an index file)
	// Return 404 to hide the fact that the folder exists
	if d.IsDir() {
		return http.StatusNotFound, nil
	}

	// If the file is supposed to be hidden, return a 404
	// (TODO: If the slice gets large, a set may be faster)
	for _, hiddenPath := range fh.hide {
		// Case-insensitive file systems may have loaded "CaddyFile" when
		// we think we got "Caddyfile", which poses a security risk if we
		// aren't careful here: case-insensitive comparison is required!
		// TODO: This matches file NAME only, regardless of path. In other
		// words, trying to serve another file with the same name as the
		// active config file will result in a 404 when it shouldn't.
		if strings.EqualFold(d.Name(), path.Base(hiddenPath)) {
			return http.StatusNotFound, nil
		}
	}

	// Note: Errors generated by ServeContent are written immediately
	// to the response. This usually only happens if seeking fails (rare).
	http.ServeContent(w, r, d.Name(), d.ModTime(), f)

	return http.StatusOK, nil
}

// redirect is taken from http.localRedirect of the std lib. It
// sends an HTTP redirect to the client but will preserve the
// query string for the new path.
func redirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	http.Redirect(w, r, newPath, http.StatusMovedPermanently)
}
