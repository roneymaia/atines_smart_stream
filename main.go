package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"atines_smart_stream/internal/api"
	"atines_smart_stream/internal/ffmpeg"
	"atines_smart_stream/internal/launcher"
	"atines_smart_stream/internal/media"
	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "endereço de escuta da UI")
	dataPath := flag.String("data", defaultDataPath(), "caminho do cameras.json")
	noBrowser := flag.Bool("no-browser", false, "não abrir o navegador ao iniciar")
	flag.Parse()

	ffmpegPath, ffprobePath, err := ffmpeg.Locate()
	if err != nil {
		log.Fatalf("FFmpeg: %v", err)
	}

	st, err := store.Open(*dataPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	caps := media.DetectCapabilities(context.Background(), ffmpegPath)
	log.Printf("encoders de hardware: nvenc=%v qsv=%v amf=%v vaapi=%v", caps.NVENC, caps.QSV, caps.AMF, caps.VAAPI)

	// argsFor probes the stream then builds ffmpeg args for a camera.
	argsFor := func(cam model.Camera) ([]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		in, err := media.BuildInputURL(cam)
		if err != nil {
			return nil, err
		}
		probe, err := media.Probe(ctx, ffprobePath, in)
		if err != nil {
			// Probe failed (camera offline?): assume h264 so we at least try to remux.
			probe = media.ProbeResult{VideoCodec: "h264"}
		}
		return media.BuildArgs(cam, probe, caps)
	}

	sup := supervisor.New(supervisor.SupervisorConfig{
		FFmpegPath: ffmpegPath,
		ArgsFor:    argsFor,
	})

	srv := api.NewServer(st, sup)
	// Feed supervisor status into SSE.
	sup.SetOnStatus(srv.StatusSink())

	// Resume cameras that were enabled.
	for _, c := range st.List() {
		if c.Enabled {
			sup.Start(c)
		}
	}

	url := "http://" + *addr
	fmt.Printf("Atines Smart Stream rodando em %s\n", url)
	if !*noBrowser {
		launcher.OpenBrowser(url)
	}
	log.Fatal(http.ListenAndServe(*addr, srv))
}

func defaultDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "cameras.json"
	}
	return filepath.Join(dir, "atines-smart-stream", "cameras.json")
}
