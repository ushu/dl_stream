# dl_stream

A tool to download videos from a JSON (`master.json`) descriptor.

## Getting Started

You need a valid `go` install to use this tool.

The tool itself has no external dependencies: it only depends on standard library.

### Installing

```
$ go get -u github.com/ushu/dl_stream
```

### Usage

You can download a single stream:

```sh
# Download the the stream at provided URL into "my_output_file.mp4"
$ dl_stream -o my_output_file.mp4 https://.../master.json
```

Or use semicolon-separated CSV file a an input structured as such:

```csv
https://.../master.json;/path/to/file
https://.../master.json;/path/to/next_file
```

```sh
$ dl_stream -csv list.csv
```

### Other options

#### re-download

By default, the tool will not remove existing files, to force a re-download of existing files, one must provide the `-r` flag:

```sh
$ dl_stream -r -csv list.csv
```

#### resolution

By default, the tool will download the videos using the highest resolution available.
One can provide the `-res` flag to decide which format to download:

```sh
# Download 720p video
$ dl_stream -res 720 http://.../master.json

# Download 1080p video
$ dl_stream -res 1080 http://.../master.json
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details

## TODO

* [ ] Support for m3u8 (HLS) file descriptors
* [ ] Better docs

