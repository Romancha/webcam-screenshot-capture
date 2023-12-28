package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chromedp/chromedp"
	"github.com/fogleman/gg"
	"github.com/go-pkgz/lgr"
	"github.com/h2non/bimg"
	"github.com/jessevdk/go-flags"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
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

	SavePath          string `long:"save-path" env:"SAVE_PATH" description:"Save path" default:"./data/webcam-screenshots"`
	FontPath          string `long:"font-path" env:"FONT_PATH" description:"Font path" default:"./data/Roboto-Bold.ttf"`
	WaterMarkTimezone string `long:"watermark-timezone" env:"WATERMARK_TIMEZONE" description:"Watermark timezone" default:"Europe/Moscow"`

	Debug   bool `long:"debug" env:"DEBUG" description:"debug mode"`
	Profile bool `long:"profile" env:"PROFILE" description:"profile mode"`
}

func main() {
	fmt.Println("Webcam capture started")
	if _, err := flags.Parse(&opts); err != nil {
		log.Printf("[ERROR] failed to parse flags: %v", err)
		os.Exit(1)
	}

	setupLog(opts.Debug)

	log.Printf("[INFO] opts: %+v", opts)

	if opts.Profile {
		go func() {
			err := http.ListenAndServe(":8080", nil)
			if err != nil {
				log.Fatalf("[ERROR] failed to start pprof: %v", err)
			}
		}()
	}

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
	ctxCancel, cancelContext := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancelContext()

	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-extensions", false),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctxCancel, options...)
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
		log.Printf("[ERROR] Error navigating to the video page: %s", err)
		return
	}

	// capture screenshot of the video
	var screenshotBuf []byte
	if err := chromedp.Run(ctx, elementScreenshot(cam, &screenshotBuf)); err != nil {
		log.Printf("[ERROR] Error capturing screenshot: %s", err)
		return
	}

	screenshotName := cam.Name + "_" + time.Now().Format("2006-01-02_15-04-05") + ".png"
	pngImagePath := opts.SavePath + "/" + screenshotName
	err = bimg.Write(pngImagePath, screenshotBuf)

	if err != nil {
		log.Printf("[ERROR] Error writing image: %s. %s", screenshotName, err)
		return
	}

	// Add the date watermark
	err = addDateWatermark(pngImagePath)
	if err != nil {
		log.Printf("[ERROR] Error adding watermark: %s", err)
		return
	}

	imageWithWatermark, err := bimg.Read(pngImagePath)
	if err != nil {
		log.Printf("[ERROR] Error reading image with watermark: %s", err)
		return
	}

	jpgImage, err := bimg.NewImage(imageWithWatermark).Convert(bimg.JPEG)
	if err != nil {
		log.Printf("[ERROR] Error converting image: %s", err)
		return
	}

	// Compress the image after adding the watermark
	compressOptions := bimg.Options{
		Quality:      70,
		Compression:  9,
		NoAutoRotate: true,
	}
	compressedImage, err := bimg.Resize(jpgImage, compressOptions)
	if err != nil {
		log.Printf("[ERROR] Error compressing image: %s", err)
		return
	}

	// Save the compressed image with .jpg extension
	jpgImagePath := pngImagePath[:len(pngImagePath)-4] + ".jpg"

	err = bimg.Write(jpgImagePath, compressedImage)
	if err != nil {
		log.Printf("[ERROR] Error writing compressed image: %s. %s", screenshotName, err)
		return
	}

	log.Printf("[INFO] Saved screenshot to " + jpgImagePath)

	//remove the original .png file
	err = os.Remove(pngImagePath)
	if err != nil {
		log.Printf("[ERROR] Error removing original .png file: %s. %s", pngImagePath, err)
	}

	// Close the context
	cancel()
}

func elementScreenshot(cam WebCam, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Screenshot(cam.XpathWebcamContainer, res),
	}
}

func addDateWatermark(imagePath string) error {
	// Load the image
	img, err := gg.LoadImage(imagePath)
	if err != nil {
		return err
	}

	// Create a new context with the same size as the image
	var W = float64(img.Bounds().Size().X)
	var H = float64(img.Bounds().Size().Y)
	dc := gg.NewContext(int(W), int(H))

	// Draw the image onto the context
	dc.DrawImage(img, 0, 0)

	// Set the font style, size, and color for the watermark text
	if err := dc.LoadFontFace(opts.FontPath, 35); err != nil {
		log.Printf("[ERROR] Error loading font: %s", err)
	}
	dc.SetRGB(1, 0, 0)

	// Write the current date as a string onto the context
	location, err := time.LoadLocation(opts.WaterMarkTimezone)
	if err != nil {
		log.Printf("[ERROR] Error loading location: %s", err)
	}
	dc.DrawStringAnchored(time.Now().In(location).Format("2006-01-02 15:04"), W*0.2, H*0.03, 0.5, 0.5)

	// Save the context as a new image
	dc.Stroke()
	return dc.SavePNG(imagePath)
}

func setupLog(dbg bool) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	lgr.SetupStdLogger(logOpts...)
}
