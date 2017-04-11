package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lukegb/snowstorm/blte"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/client"
	"github.com/lukegb/snowstorm/ngdp/mndx"
)

var (
	cm         sync.RWMutex // guards all below
	c          *client.Client
	fileList   map[string]*mndx.File
	fileSystem *RubbishFilesystem = new(RubbishFilesystem)
)

const (
	checkForNewVersionInterval = 1 * time.Hour

	modeDir  = os.ModeDir | 0555
	modeFile = 0555
)

type RubbishFile struct {
	isDir bool
	name  string
	size  int64

	f *mndx.File
}

func (f *RubbishFile) updateFrom(mf *mndx.File) {
	f.f = mf
	f.isDir = false
	f.size = int64(mf.Size)
}

func (f *RubbishFile) Stat() (os.FileInfo, error) { return f, nil }

func (f *RubbishFile) Name() string { return f.name }
func (f *RubbishFile) Size() int64  { return f.size }
func (f *RubbishFile) Mode() os.FileMode {
	if f.isDir {
		return modeDir
	}
	return modeFile
}
func (f *RubbishFile) ModTime() time.Time { return time.Time{} }
func (f *RubbishFile) IsDir() bool        { return f.isDir }
func (f *RubbishFile) Sys() interface{}   { return nil }

func (f *RubbishFile) toHTTPFile() (*RubbishHTTPFile, error) {
	hf := &RubbishHTTPFile{
		RubbishFile: *f,
	}
	err := hf.open()
	if err != nil {
		return nil, err
	}
	return hf, nil
}

type RubbishHTTPFile struct {
	RubbishFile

	r io.Reader
	c io.Closer

	psn int64
}

func (f *RubbishHTTPFile) Read(b []byte) (n int, err error) {
	n, err = f.r.Read(b)
	if err == nil {
		f.psn = f.psn + int64(n)
	}
	return n, err
}

func (f *RubbishHTTPFile) Close() error {
	return f.c.Close()
}

func (f *RubbishHTTPFile) open() error {
	cm.RLock()
	defer cm.RUnlock()

	contentHash := ngdp.EncodingKeyToContentHash(f.f.EncodingKey)

	fEncBody, err := c.Fetch(contentHash)
	if err != nil {
		return err
	}

	fBody := blte.NewReader(fEncBody)

	f.psn = 0
	f.r = fBody
	f.c = fEncBody

	return nil
}

func (f *RubbishHTTPFile) seekTo(offset int64) (int64, error) {
	if f.psn > offset {
		if err := f.Close(); err != nil {
			return 0, err
		}
		if err := f.open(); err != nil {
			return 0, err
		}
	}

	mustDitch := offset - f.psn
	_, err := io.CopyN(ioutil.Discard, f.r, mustDitch)
	if err != nil {
		return 0, err
	}

	return offset, nil
}

func (f *RubbishHTTPFile) Seek(offset int64, whence int) (int64, error) {
	var seekTo int64
	switch whence {
	case io.SeekCurrent:
		seekTo = f.psn + offset
	case io.SeekEnd:
		seekTo = int64(f.f.Size) + offset
	case io.SeekStart:
		seekTo = offset
	default:
		return 0, fmt.Errorf("invalid seek whence %d", whence)
	}
	return f.seekTo(seekTo)
}

func (f *RubbishHTTPFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, fmt.Errorf("can't readdir a file")
}

type RubbishHTTPDirWrapper struct {
	RubbishHTTPDir

	children []os.FileInfo
}

func (d *RubbishHTTPDirWrapper) Readdir(count int) ([]os.FileInfo, error) {
	if len(d.children) == 0 {
		return nil, io.EOF
	}

	if count <= 0 {
		ch := d.children
		d.children = nil
		return ch, nil
	}

	ch := d.children[:count]
	d.children = d.children[count:]
	return ch, nil
}

type RubbishHTTPDir struct {
	name string

	children    []os.FileInfo
	childrenMap map[string]os.FileInfo
}

func makeRubbishHTTPDir() *RubbishHTTPDir {
	return &RubbishHTTPDir{childrenMap: make(map[string]os.FileInfo)}
}

func (d *RubbishHTTPDir) Close() error             { return fmt.Errorf("can't close a directory") }
func (d *RubbishHTTPDir) Read([]byte) (int, error) { return 0, fmt.Errorf("can't read a directory") }
func (d *RubbishHTTPDir) Seek(int64, int) (int64, error) {
	return 0, fmt.Errorf("can't seek a directory")
}

func (d *RubbishHTTPDir) Stat() (os.FileInfo, error) { return d, nil }

func (d *RubbishHTTPDir) Name() string       { return d.name }
func (d *RubbishHTTPDir) Size() int64        { return 0 }
func (d *RubbishHTTPDir) Mode() os.FileMode  { return modeDir }
func (d *RubbishHTTPDir) ModTime() time.Time { return time.Time{} }
func (d *RubbishHTTPDir) IsDir() bool        { return true }
func (d *RubbishHTTPDir) Sys() interface{}   { return nil }

func (d *RubbishHTTPDir) navigate(path string) (http.File, error) {
	sepIdx := strings.Index(path, "/")
	if sepIdx == -1 {
		// last bit
		key := strings.ToLower(path)
		c, ok := d.childrenMap[key]
		if !ok {
			return nil, os.ErrNotExist
		}
		if crf, ok := c.(*RubbishFile); ok {
			return crf.toHTTPFile()
		}
		if crd, ok := c.(*RubbishHTTPDir); ok {
			return &RubbishHTTPDirWrapper{*crd, crd.children[:]}, nil
		}
		return nil, fmt.Errorf("wtf? %s is a %T", path, c)
	}

	thisLevel := path[:sepIdx]
	nextName := path[sepIdx+1:]

	key := strings.ToLower(thisLevel)

	c, ok := d.childrenMap[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	crd, ok := c.(*RubbishHTTPDir)
	if !ok {
		return nil, fmt.Errorf("can't cd into a file %q: %q", key, path)
	}
	return crd.navigate(nextName)
}

func (d *RubbishHTTPDir) dump(n int) []string {
	prefix := strings.Repeat("  ", n)
	var out []string
	for _, c := range d.children {
		out = append(out, fmt.Sprintf("%s%s", prefix, c.Name()))
		if crd, ok := c.(*RubbishHTTPDir); ok {
			out = append(out, crd.dump(n+1)...)
		}
	}
	return out
}

func (d *RubbishHTTPDir) file(path string) (*RubbishFile, error) {
	sepIdx := strings.Index(path, "/")
	if sepIdx == -1 {
		// last bit
		key := strings.ToLower(path)
		if c, ok := d.childrenMap[key]; ok {
			crf, ok := c.(*RubbishFile)
			if !ok {
				return nil, fmt.Errorf("file with same name as directory %q", path)
			}
			return crf, nil
		}

		rf := &RubbishFile{
			name: path,
		}
		d.childrenMap[key] = rf
		d.children = append(d.children, rf)
		return rf, nil
	}

	thisLevel := path[:sepIdx]
	nextName := path[sepIdx+1:]

	key := strings.ToLower(thisLevel)

	if c, ok := d.childrenMap[key]; ok {
		crd, ok := c.(*RubbishHTTPDir)
		if !ok {
			return nil, fmt.Errorf("directory with same name as file %q", path)
		}
		return crd.file(nextName)
	}

	dir := makeRubbishHTTPDir()
	dir.name = thisLevel
	d.childrenMap[key] = dir
	d.children = append(d.children, dir)
	return dir.file(nextName)
}

type RubbishFilesystem struct {
	rootDir *RubbishHTTPDir
}

// Open must only be called while holding cm.
func (rfs *RubbishFilesystem) Open(name string) (http.File, error) {
	name = strings.TrimLeft(name, "/")

	if name == "" {
		return &RubbishHTTPDirWrapper{*rfs.rootDir, rfs.rootDir.children[:]}, nil
	}

	return rfs.rootDir.navigate(name)
}

func refreshClient() error {
	cli := client.New(ngdp.ProgramHotSTest)
	if err := cli.Init(); err != nil {
		return err
	}

	buildConfig, err := cli.BuildConfig()
	if err != nil {
		return err
	}

	rootEncBody, err := cli.Fetch(buildConfig.Root)
	if err != nil {
		return err
	}
	defer rootEncBody.Close()

	rootBody := blte.NewReader(rootEncBody)

	newFileList, err := mndx.FileList(rootBody)
	if err != nil {
		return err
	}

	rootDir := makeRubbishHTTPDir()

	for fn, f := range newFileList {
		rf, err := rootDir.file(fn)
		if err != nil {
			return err
		}

		rf.updateFrom(f)
	}

	cm.Lock()
	c = cli
	fileList = newFileList
	fileSystem.rootDir = rootDir
	cm.Unlock()

	return nil
}

func main() {
	log.Println("starting up!")

	if err := refreshClient(); err != nil {
		log.Fatal(err)
	}

	go func() {
		// starting spinner
		t := time.Tick(checkForNewVersionInterval)
		for {
			<-t
			cm.RLock()
			isNewVersion, newVer, err := c.VersionHasChanged()
			cm.RUnlock()

			if err != nil {
				log.Printf("update spinner: VersionHasChanged: %v", err)
				continue
			}
			if isNewVersion {
				oldVer, err := c.Version()
				if err != nil {
					log.Printf("update spinner: Version: %v", err)
				}
				log.Printf("update spinner: new version available!\nold version: %#v\nnew version: %#v", oldVer, newVer)
				if err := refreshClient(); err != nil {
					log.Printf("update spinner: refreshClient: %v - will try again later", err)
				}
			}
		}
	}()

	log.Println("ready!")

	fs := http.FileServer(fileSystem)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cm.RLock()
		defer cm.RUnlock()

		log.Println(r.URL)

		fs.ServeHTTP(w, r)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
