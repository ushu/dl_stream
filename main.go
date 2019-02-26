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
	"net/http"
	"net/url"
	"os"
	"path"
)

var input string
var output string
var isCSV bool

type MasterJSON struct {
	ClipID  string  `json:"clip_id"`
	BaseURL string  `json:"base_url"`
	Video   []Video `json:"video"`
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
	flag.StringVar(&output, "o", "download.mp4", "Output file name")
	flag.BoolVar(&isCSV, "csv", false, "Load CSV file")
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

	records, err := csv.NewReader(bytes.NewReader(buf)).ReadAll()
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
	log.Printf("Downloading %s\n", in)

	u, err := url.Parse(in)
	if err != nil {
		return err
	}

	// read input
	mjson, err := readMasterJSON(u)
	if err != nil {
		return err
	}
	// prepare output
	w, done, err := openOutput(out)
	if err != nil {
		return err
	}
	defer done()

	return processMasterJSON(u, mjson, w)
}

func selectVideo(videos []Video) (*Video, error) {
	if len(videos) == 0 {
		return nil, errors.New("No video in stream")
	}
	return &videos[0], nil
}

func processMasterJSON(u *url.URL, mjson *MasterJSON, w io.Writer) error {
	video, err := selectVideo(mjson.Video)
	if err != nil {
		return err
	}

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

func openOutput(out string) (w *bufio.Writer, done func(), err error) {
	var o *os.File
	if o, err = os.Create(out); err == nil {
		w = bufio.NewWriter(o)
		done = func() {
			w.Flush()
			o.Close()
		}
	}
	return
}
