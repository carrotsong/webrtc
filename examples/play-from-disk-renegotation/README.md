# play-from-disk-renegotiation
play-from-disk-renegotiation demonstrates Pion WebRTC's renegotiation abilities.

For a simpler example of playing a file from disk we also have [examples/play-from-disk](/examples/play-from-disk)

## Instructions

### Download play-from-disk-renegotiation
This example requires you to clone the repo since it is serving static HTML.

```
mkdir -p $GOPATH/src/github.com/carrotsong
cd $GOPATH/src/github.com/carrotsong
git clone https://github.com/carrotsong/webrtc.git
cd webrtc/examples/play-from-disk-renegotiation
```

### Create IVF named `output.ivf` that contains a VP8 track
```
ffmpeg -i $INPUT_FILE -g 30 output.ivf
```

### Run play-from-disk-renegotiation
The `output.ivf` you created should be in the same directory as `play-from-disk-renegotiation`. Execute `go run *.go`

### Open the Web UI
Open [http://localhost:8080](http://localhost:8080) and you should have a `Add Track` and `Remove Track` button.  Press these to add as many tracks as you want, or to remove as many as you wish.

Congrats, you have used Pion WebRTC! Now start building something cool
