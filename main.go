package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

var input string
var output string
var isCSV bool
var resolution int
var redownload bool

type MasterJSON struct {
	ClipID  string   `json:"clip_id"`
	BaseURL string   `json:"base_url"`
	Video   []*Video `json:"video"`
}

type Video struct {
	ID                 string  `json:"id"`
	BaseURL            string  `json:"base_url"`
	Format             string  `json:"format"`
	MimeType           string  `json:"mime_type"`
	Codecs             string  `json:"codecs"`
	Bitrate            int     `json:"bitrate"`
	AvgBitrate         int     `json:"avg_bitrate"`
	Duration           float64 `json:"duration"`
	Framerate          int     `json:"framerate"`
	Width              int     `json:"width"`
	Height             int     `json:"height"`
	MaxSegmentDuration int     `json:"max_segment_duration"`
	InitSegment        string  `json:"init_segment"`
	Segments           []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		URL   string  `json:"url"`
	} `json:"segments"`
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("")
	flag.StringVar(&output, "o", "download.mp4", "output file name")
	flag.BoolVar(&isCSV, "csv", false, "load CSV file")
	flag.IntVar(&resolution, "res", 0, "expected resolution (defaults to max available)")
	flag.BoolVar(&redownload, "r", false, "restart all downloads")
	flag.Usage = func() {
		log.Println("Usage: dl_stream FILE_OR_URL")
		flag.PrintDefaults()
	}
}

func main() {
	// grab input/output files
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	input = flag.Arg(0)

	if !isCSV {
		if err := download(input, output); err != nil {
			log.Fatalln(err)
		}
	} else {
		if err := downloadCSV(input); err != nil {
			log.Fatalln(err)
		}
	}
}

func downloadCSV(in string) error {
	// first read de CSV file
	buf, err := ioutil.ReadFile(in)
	if err != nil {
		return err
	}

	r := csv.NewReader(bytes.NewReader(buf))
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		return err
	}

	for i, record := range records {
		if len(record) < 2 {
			break
		}
		if err := download(record[0], record[1]); err != nil {
			return fmt.Errorf("Error on line %d, %v", i+1, err)
		}
	}

	return nil
}

func download(in, out string) error {
	u, err := url.Parse(in)
	if err != nil {
		return err
	}

	// read input
	mjson, err := readMasterJSON(u)
	if err != nil {
		return err
	}

	return processMasterJSON(u, mjson, out)
}

func selectVideo(videos []*Video) (*Video, error) {
	var video *Video
	highestRes := 0
	for _, v := range videos {
		if resolution != 0 && v.Height == resolution {
			video = v
			break
		}
		resPx := v.Width * v.Height
		if resPx > highestRes {
			highestRes = resPx
			video = v
		}
	}
	if video == nil {
		return nil, errors.New("No video in stream")
	}
	return video, nil
}

func processMasterJSON(u *url.URL, mjson *MasterJSON, out string) error {
	video, err := selectVideo(mjson.Video)
	if err != nil {
		return err
	}

	// prepare output
	// -> the extensions depends on the video/mime type
	if out, err = pathWithExtension(out, video.MimeType, ".mp4"); err != nil {
		return err
	}
	if !redownload && fileExists(out) {
		log.Printf("File %q already exists: skipping...\n", path.Base(out))
		return nil
	}
	log.Printf("Downloading %q (%dx%d)\n", path.Base(out), video.Width, video.Height)
	// -> now we can open the destination file
	w, done, err := openOutput(out)
	if err != nil {
		return err
	}
	defer done()

	// decode initial segment, if any
	if video.InitSegment != "" {
		b, err := base64.StdEncoding.DecodeString(video.InitSegment)
		if err != nil {
			return fmt.Errorf("Could not decode initial segment: %v", err)
		}
		if _, err = w.Write(b); err != nil {
			return fmt.Errorf("Could not write to output file: %v", err)
		}
	}

	// iterate and download all the segments
	basePath := path.Clean(path.Join(path.Dir(u.Path), mjson.BaseURL, video.BaseURL))
	for _, s := range video.Segments {
		p := path.Join(basePath, s.URL)
		su := &url.URL{
			Scheme: u.Scheme,
			Host:   u.Host,
			Path:   p,
		}
		res, err := http.Get(su.String())
		if err != nil {
			return fmt.Errorf("Could not download segment: %v", err)
		}
		_, err = io.Copy(w, res.Body)
		if err != nil {
			_ = res.Body.Close()
			return fmt.Errorf("Could download URL contents: %v", err)
		}
		if err = res.Body.Close(); err != nil {
			return fmt.Errorf("Could not write to output file: %v", err)
		}
	}

	return nil
}

func readMasterJSON(u *url.URL) (mjson *MasterJSON, err error) {
	var buf []byte
	if buf, err = readURL(u); err == nil {
		mjson, err = decodeMasterJSON(buf)
	}
	return
}

func decodeMasterJSON(buf []byte) (mjson *MasterJSON, err error) {
	err = json.Unmarshal(buf, &mjson)
	return
}

func readURL(u *url.URL) ([]byte, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ioutil.ReadFile(u.Path)
	}
	res, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	buf, err := ioutil.ReadAll(res.Body)
	if err != nil {
		_ = res.Body.Close()
		return nil, err
	}
	return buf, res.Body.Close()
}

func pathWithExtension(out, mimeType, defaultExt string) (string, error) {
	if ext := path.Ext(out); ext == "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		} else {
			ext = defaultExt
		}
		out = out + ext
	}
	return filepath.Abs(out)
}

func openOutput(out string) (w *bufio.Writer, done func(), err error) {
	// Create the containing directory
	if dir := path.Dir(out); dir != "." {
		if err = os.MkdirAll(dir, 0770); err != nil && !os.IsExist(err) {
			return
		}
	}

	var o *os.File
	if o, err = os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660); err == nil {
		w = bufio.NewWriter(o)
		done = func() {
			w.Flush()
			o.Close()
		}
	}
	return
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return !os.IsNotExist(err)
}
