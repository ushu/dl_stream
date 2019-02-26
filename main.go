package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
)

var input string
var output string

type MasterJSON struct {
	ClipID  string `json:"clip_id"`
	BaseURL string `json:"base_url"`
	Video   []struct {
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
	} `json:"video"`
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("")
	flag.StringVar(&output, "o", "download.mp4", "Output file name")
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

	// now we load the data
	u, err := url.Parse(input)
	if err != nil {
		log.Fatalf("Expect a URL: %v", err)
	}
	buf, err := readURL(u)
	if err != nil {
		log.Fatalf("Could not read data: %v", err)
	}

	// is it a master.json file ?
	var mjson MasterJSON
	if err = json.Unmarshal(buf, &mjson); err != nil {
		log.Fatalf("Could not decode data: %v", err)
	}
	if err = processMasterJSON(u, &mjson); err != nil {
		log.Fatal(err)
	}
}

func processMasterJSON(u *url.URL, mjson *MasterJSON) error {
	if len(mjson.Video) == 0 {
		return errors.New("No video in stream")
	}
	video := mjson.Video[0]

	// open the output file
	w, done, err := openOutput()
	if err != nil {
		return fmt.Errorf("Could not create output file: %v", err)
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

func readURL(u *url.URL) ([]byte, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ioutil.ReadFile(u.String())
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

func openOutput() (w *bufio.Writer, done func(), err error) {
	var o *os.File
	if o, err = os.Create(output); err == nil {
		w = bufio.NewWriter(o)
		done = func() {
			w.Flush()
			o.Close()
		}
	}
	return
}
