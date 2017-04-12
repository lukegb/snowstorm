package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/lukegb/snowstorm/ngdp"
	"github.com/lukegb/snowstorm/ngdp/client"
	"github.com/lukegb/snowstorm/ngdp/mndx"
	"gopkg.in/webpack.v0"
)

var (
	trackRegionsStr  = flag.String("track-regions", "eu,us", "comma-separated list of regions to track")
	trackProgramsStr = flag.String("track-programs", "hero,herot", "comma-separated list of programs to track")

	listen  = flag.String("listen", ":8080", "HTTP listen address")
	devMode = flag.Bool("dev", false, "development mode")
)

var (
	ds *datastore
)

type Program struct {
	VersionInfo struct {
		BuildConfig   string `json:"build_config"`
		CDNConfig     string `json:"cdn_config"`
		BuildID       int    `json:"build_id"`
		VersionsName  string `json:"versions_name"`
		ProductConfig string `json:"product_config"`
	} `json:"version_info"`
	CDNInfo struct {
		Path  string   `json:"path"`
		Hosts []string `json:"hosts"`
	} `json:"cdn_info"`
}

func programFromClient(c *client.Client) Program {
	var p Program

	p.VersionInfo.BuildConfig = fmt.Sprintf("%032x", c.VersionInfo.BuildConfig)
	p.VersionInfo.CDNConfig = fmt.Sprintf("%032x", c.VersionInfo.CDNConfig)
	p.VersionInfo.BuildID = c.VersionInfo.BuildID
	p.VersionInfo.VersionsName = c.VersionInfo.VersionsName
	p.VersionInfo.ProductConfig = fmt.Sprintf("%032x", c.VersionInfo.ProductConfig)

	p.CDNInfo.Path = c.CDNInfo.Path
	p.CDNInfo.Hosts = c.CDNInfo.Hosts

	return p
}

func annotateHeadersWithClient(h http.Header, c *client.Client) {
	h.Set("Snowstorm-Build-Config", fmt.Sprintf("%032x", c.VersionInfo.BuildConfig))
	h.Set("Snowstorm-Build-ID", fmt.Sprintf("%d", c.VersionInfo.BuildID))
	h.Set("Snowstorm-Version-Name", c.VersionInfo.VersionsName)
}

func ProgramsHandler(w http.ResponseWriter, r *http.Request) {
	out, err := func() (map[ngdp.ProgramCode]map[ngdp.Region]Program, error) {
		out := make(map[ngdp.ProgramCode]map[ngdp.Region]Program)
		tracking := ds.Tracking()
		for _, t := range tracking {
			if _, ok := out[t.Program]; !ok {
				out[t.Program] = make(map[ngdp.Region]Program)
			}

			c, err := ds.Client(t.Region, t.Program)
			if err != nil {
				return nil, err
			}

			out[t.Program][t.Region] = programFromClient(c)
		}
		return out, nil
	}()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func ProgramHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	program := ngdp.ProgramCode(vars["program"])
	region := ngdp.Region(vars["region"])

	c, err := ds.Client(region, program)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	annotateHeadersWithClient(w.Header(), c)

	out := programFromClient(c)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

type FileDirectory struct {
	Directories map[string]*FileDirectory `json:"directories,omitempty"`
	Files       []string                  `json:"files,omitempty"`
}

func FileHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	program := ngdp.ProgramCode(vars["program"])
	region := ngdp.Region(vars["region"])

	c, err := ds.Client(region, program)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	annotateHeadersWithClient(w.Header(), c)

	ctx := r.Context()

	fp := vars["filePath"]

	glog.Infof("%s/%s: request file %q", program, region, fp)
	tde, err := c.FilenameMapper.(*mndx.TreeDirectory).Get(fp)
	if err != nil {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}

	if tde.File != nil {
		calcetag := fmt.Sprintf("\"%032x\"", tde.File.EncodingKey)
		if etag := r.Header.Get("If-None-Match"); etag == calcetag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		// serving as file
		rc, err := c.Fetch(ctx, tde.File.EncodingKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rc.Body.Close()

		w.Header().Set("Content-Length", fmt.Sprintf("%d", tde.File.Size))
		w.Header().Set("Snowstorm-File-Content-Hash", fmt.Sprintf("%032x", rc.ContentHash))
		w.Header().Set("Snowstorm-File-CDN-Hash", fmt.Sprintf("%032x", rc.CDNHash))
		if !rc.RetrievedCDNHash.Equal(rc.CDNHash) {
			w.Header().Set("Snowstorm-Archive-CDN-Hash", fmt.Sprintf("%032x", rc.RetrievedCDNHash))
		}
		w.Header().Set("ETag", calcetag)
		io.Copy(w, rc.Body)
		return
	}

	recurse := r.FormValue("recurse") == "true"

	// serving as directory
	var makeDirectory func(*mndx.TreeDirectory) (*FileDirectory, error)
	makeDirectory = func(d *mndx.TreeDirectory) (*FileDirectory, error) {
		fd := &FileDirectory{
			Directories: make(map[string]*FileDirectory),
		}
		for _, e := range d.List() {
			if e.Directory != nil {
				if !recurse {
					fd.Directories[e.Name] = &FileDirectory{}
					continue
				}
				var err error
				fd.Directories[e.Name], err = makeDirectory(e.Directory)
				if err != nil {
					return nil, fmt.Errorf("%s: %v", e.Name, err)
				}
			} else if e.File != nil {
				fd.Files = append(fd.Files, e.Name)
			} else {
				return nil, fmt.Errorf("somehow %q is neither a directory nor a file", e.Name)
			}
		}
		return fd, nil
	}
	out, err := makeDirectory(tde.Directory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

func main() {
	flag.Parse()

	webpack.Init(*devMode)

	llc := &client.LowLevelClient{
		Client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	ds = newDatastore(llc)

	trackRegions := strings.Split(*trackRegionsStr, ",")
	trackPrograms := strings.Split(*trackProgramsStr, ",")

	for _, region := range trackRegions {
		for _, program := range trackPrograms {
			ds.Track(ngdp.Region(region), ngdp.ProgramCode(program))
		}
	}

	glog.Info("Performing initial datastore update...")
	ds.Update(context.Background())
	go func() {
		for range time.Tick(30 * time.Minute) {
			glog.Info("Performing datastore update")
			ds.Update(context.Background())
		}
	}()

	rtr := mux.NewRouter()
	http.Handle("/", rtr)

	r := rtr.Methods("GET").Subrouter()
	r.HandleFunc("/programs", ProgramsHandler)
	r.HandleFunc("/programs/{program}/{region}", ProgramHandler)
	r.Handle("/programs/{program}/{region}/files", gziphandler.GzipHandler(http.HandlerFunc(FileHandler)))
	r.Handle("/programs/{program}/{region}/files/{filePath:.+}", gziphandler.GzipHandler(http.HandlerFunc(FileHandler)))

	done := make(chan int)
	http.HandleFunc("/exit", func(w http.ResponseWriter, r *http.Request) {
		close(done)
	})

	go func() {
		glog.Infof("Listening on %q", *listen)
		glog.Exit(http.ListenAndServe(*listen, nil))
	}()

	<-done
}
