package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chromedp/chromedp"
	"github.com/go-pkgz/lgr"
	"github.com/h2non/bimg"
	"github.com/jessevdk/go-flags"
	"log"
	"math/rand"
	"os"
	"time"
)

type WebCam struct {
	Name                    string `json:"name"`
	Url                     string `json:"url"`
	XpathToOpenInFullScreen string `json:"xpath-to-open-in-full-screen"`
	XpathWebcamContainer    string `json:"xpath-webcam-container"`
}

type WebCamList []WebCam

var opts struct {
	ConfigPath       string `long:"config-path" env:"CONFIG_PATH" description:"Config path" default:"./data/config.json"`
	CaptureDelayFrom int    `long:"capture-delay-from" env:"CAPTURE_DELAY_FROM" description:"Capture delay from" default:"280"`
	CaptureDelayTo   int    `long:"capture-delay-to" env:"CAPTURE_DELAY_TO" description:"Capture delay to" default:"300"`

	SavePath string `long:"save-path" env:"SAVE_PATH" description:"Save path" default:"./data/webcam-screenshots"`

	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

func main() {
	fmt.Println("Webcam capture started")
	if _, err := flags.Parse(&opts); err != nil {
		log.Printf("[ERROR] failed to parse flags: %v", err)
		os.Exit(1)
	}

	setupLog(opts.Debug)

	log.Printf("[INFO] opts: %+v", opts)

	config, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		log.Fatalf("[ERROR] failed to read config: %v", err)
	}

	var webCams WebCamList
	err = json.Unmarshal(config, &webCams)
	if err != nil {
		log.Fatalf("[ERROR] failed to parse config: %v", err)
	}

	for {
		for _, cam := range webCams {
			saveWebCamScreenshot(cam)
		}

		from := opts.CaptureDelayFrom
		to := opts.CaptureDelayTo
		randSleep := time.Duration(rand.Intn(to-from)+from) * time.Second
		time.Sleep(randSleep)
	}
}

func saveWebCamScreenshot(cam WebCam) {
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-extensions", false),
	)

	// RemoteAllocatorOptions are the options for the remote allocator.
	// Enable GPU
	//var options []chromedp.RemoteAllocatorOption

	//allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), "ws://browser:9222/", options...)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancel()

	// create context
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// Navigate to the video page
	err := chromedp.Run(ctx,
		chromedp.Navigate(cam.Url),
		chromedp.Sleep(5*time.Second),
		chromedp.DoubleClick(cam.XpathToOpenInFullScreen),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		log.Fatal("[ERROR] Error navigating to the video page: ", err)
	}

	// capture screenshot of the video
	var buf []byte
	if err := chromedp.Run(ctx, elementScreenshot(cam, &buf)); err != nil {
		log.Fatal("[ERROR] Error capturing screenshot: ", err)
	}

	jpgImage, err := bimg.NewImage(buf).Convert(bimg.JPEG)
	if err != nil {
		log.Printf("[ERROR] Error converting image: %s", err)
	}

	compressOptions := bimg.Options{
		Quality:      70,
		Compression:  9,
		NoAutoRotate: true,
	}
	compressedImage, err := bimg.Resize(jpgImage, compressOptions)
	if err != nil {
		log.Printf("[ERROR] Error compressing image: %s", err)
	}

	screenshotName := cam.Name + "_" + time.Now().Format("2006-01-02_15-04-05") + ".jpg"
	path := opts.SavePath + "/" + screenshotName
	err = bimg.Write(path, compressedImage)

	if err != nil {
		log.Printf("[ERROR] Error writing image: %s. %s", screenshotName, err)
	}

	log.Printf("[INFO] Saved screenshot to " + path)

	// Close the context
	cancel()
}

func elementScreenshot(cam WebCam, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Screenshot(cam.XpathWebcamContainer, res),
	}
}

func setupLog(dbg bool) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	lgr.SetupStdLogger(logOpts...)
}