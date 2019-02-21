package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
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
	Audio []struct {
		ID                 string  `json:"id"`
		BaseURL            string  `json:"base_url"`
		Format             string  `json:"format"`
		MimeType           string  `json:"mime_type"`
		Codecs             string  `json:"codecs"`
		Bitrate            int     `json:"bitrate"`
		AvgBitrate         int     `json:"avg_bitrate"`
		Duration           float64 `json:"duration"`
		Channels           int     `json:"channels"`
		SampleRate         int     `json:"sample_rate"`
		MaxSegmentDuration int     `json:"max_segment_duration"`
		InitSegment        string  `json:"init_segment"`
		Segments           []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			URL   string  `json:"url"`
		} `json:"segments"`
	} `json:"audio"`
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
	if len(mjson.Video) == 0 {
		log.Fatal("No video in stream")
	}
	video := mjson.Video[0]

	// open the output file
	o, err := os.Create(output)
	if err != nil {
		log.Fatalf("Could not create output file: %v", err)
	}
	defer o.Close()
	w := bufio.NewWriter(o)
	defer w.Flush()

	// decode initial segment, if any
	if video.InitSegment != "" {
		b, err := base64.StdEncoding.DecodeString(video.InitSegment)
		if err != nil {
			log.Fatalf("Could not decode initial segment: %v", err)
		}
		if _, err = w.Write(b); err != nil {
			log.Fatalf("Could not write to output file: %v", err)
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
			log.Fatalf("Could not download segment: %v", err)
		}
		_, err = io.Copy(w, res.Body)
		if err != nil {
			_ = res.Body.Close()
			log.Fatalf("Could download URL contents: %v", err)
		}
		if err := res.Body.Close(); err != nil {
			log.Fatalf("Could not write to output file: %v", err)
		}
	}

	// https://skyfire.vimeocdn.com/1550712600-0x3b90c95f6b2c22486d1f7abdc442ca216aacde7b/125872030/sep/audio/359921969/chop/segment-41.m4s

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
