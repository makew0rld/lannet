package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

var httpFs http.Handler
var fsRoot string
var fsMu = &sync.RWMutex{} // Use when changing

var dirListTmpl = template.Must(template.ParseFS(templates, "templates/dirlist.html"))
var homepageTmpl = template.Must(template.ParseFS(templates, "templates/homepage.html"))

// startWebserver starts the webserver in a goroutine. It returns the port the
// webserver runs on, which is randomized.
func startWebserver() string {
	// https://stackoverflow.com/a/43425461/

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		daemonDie(err)
	}

	// Setup handlers
	httpFs = http.FileServer(dotFileHidingFileSystem{http.Dir(defaultRoot)})
	fsRoot = defaultRoot
	http.Handle("/", customDirListing(rootHandler))
	http.HandleFunc("/.homepage", homeHandler)
	http.HandleFunc("/.api/", apiHandler)

	go func() { daemonDie(http.Serve(listener, nil)) }()

	return strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
}

type fileInfo struct {
	Name    string
	ModTime time.Time
	Size    string
}

// dirlistData is the data for the dirlist template
type dirlistData struct {
	Name          string
	ChildrenDirs  []fileInfo
	ChildrenFiles []fileInfo
}

func customDirListing(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			// It's a directory
			fsMu.RLock()
			defer fsMu.RUnlock()

			fi, err := os.Stat(filepath.Join(fsRoot, r.URL.Path, "index.html"))
			if err != nil || fi.IsDir() {
				// No index or some other error, generate dir listing instead
				entries, err := os.ReadDir(filepath.Join(fsRoot, r.URL.Path))
				if err != nil {
					// Just let the other handler deal with it
					next.ServeHTTP(w, r)
					return
				}
				data := dirlistData{
					Name:          path.Dir(r.URL.Path),
					ChildrenDirs:  make([]fileInfo, 0),
					ChildrenFiles: make([]fileInfo, 0),
				}
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), ".") {
						continue
					}

					var modTime time.Time
					var size string
					fi, err := entry.Info()
					if err == nil {
						modTime = fi.ModTime()
						size = humanize.IBytes(uint64(fi.Size()))
					}
					if entry.IsDir() {
						data.ChildrenDirs = append(data.ChildrenDirs, fileInfo{
							Name:    entry.Name(),
							ModTime: modTime,
							Size:    size,
						})
					} else {
						data.ChildrenFiles = append(data.ChildrenFiles, fileInfo{
							Name:    entry.Name(),
							ModTime: modTime,
							Size:    size,
						})
					}
				}
				dirListTmpl.Execute(w, data)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fsMu.RLock()
	defer fsMu.RUnlock()
	httpFs.ServeHTTP(w, r)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	peersMu.RLock()
	defer peersMu.RUnlock()

	homepageTmpl.Execute(w, peers)
}

// loopbackOnly writes a 403 response if the request is not from the local machine,
// returning true as well.
func loopbackOnly(w http.ResponseWriter, r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !net.ParseIP(ip).IsLoopback() {
		// A different machine made this API request - not allowed!
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "403 Forbidden")
		return true
	}
	return false
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/.api/")
	switch path {
	case "setRoot":
		if loopbackOnly(w, r) {
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "403 Method Not Allowed\nUse POST")
			return
		}

		newRoot, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "500 Internal Server Error\nCouldn't read request body: "+err.Error())
			return
		}
		if !filepath.IsAbs(string(newRoot)) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "400 Bad Request\nProvided root path is not an absolute path")
			return
		}
		// Change file server handler
		fsMu.Lock()
		httpFs = http.FileServer(dotFileHidingFileSystem{http.Dir(string(newRoot))})
		fsRoot = string(newRoot)
		fsMu.Unlock()
		w.WriteHeader(http.StatusOK)

	case "getName":
		if name == "" {
			// Tell peer to use IP address as name
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, "404 Not Found")
		}
		fmt.Fprint(w, name)

	case "setName":
		if loopbackOnly(w, r) {
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "403 Method Not Allowed\nUse POST")
			return
		}

		// Only read name up to 64 bytes
		var newName strings.Builder
		_, err := io.CopyN(&newName, r.Body, 65)
		if !errors.Is(err, io.EOF) {
			if err == nil {
				// Too long
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintln(w, "Name was longer than 64 bytes, rejected")
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintln(w, "500 Internal Server Error\nCouldn't read request body: "+err.Error())
			}
			return
		}
		name = strings.ToValidUTF8(newName.String(), "\uFFFD")
		w.WriteHeader(http.StatusOK)

	default:
		// Unknown path
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "404 Not Found")
	}
}

// Fileserver with dot-file hiding.
// Taken from https://golang.org/pkg/net/http/#example_FileServer_dotFileHiding

// containsDotFile reports whether name contains a path element starting with a period.
// The name is assumed to be a delimited by forward slashes, as guaranteed
// by the http.FileSystem interface.
func containsDotFile(name string) bool {
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// dotFileHidingFile is the http.File use in dotFileHidingFileSystem.
// It is used to wrap the Readdir method of http.File so that we can
// remove files and directories that start with a period from its output.
type dotFileHidingFile struct {
	http.File
}

// Readdir is a wrapper around the Readdir method of the embedded File
// that filters out all files that start with a period in their name.
func (f dotFileHidingFile) Readdir(n int) (fis []fs.FileInfo, err error) {
	files, err := f.File.Readdir(n)
	for _, file := range files { // Filters out the dot files
		if !strings.HasPrefix(file.Name(), ".") {
			fis = append(fis, file)
		}
	}
	return
}

// dotFileHidingFileSystem is an http.FileSystem that hides
// hidden "dot files" from being served.
type dotFileHidingFileSystem struct {
	http.FileSystem
}

// Open is a wrapper around the Open method of the embedded FileSystem
// that serves a 403 permission error when name has a file or directory
// with whose name starts with a period in its path.
func (fsys dotFileHidingFileSystem) Open(name string) (http.File, error) {
	if containsDotFile(name) { // If dot file, return 403 response
		return nil, fs.ErrPermission
	}

	file, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return dotFileHidingFile{file}, err
}
