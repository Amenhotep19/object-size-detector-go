/*
* Copyright (c) 2018 Intel Corporation.
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gocv.io/x/gocv"
)

const (
	// name is a program name
	name = "object-size-detector"
	// topic is MQTT topic
	topic = "defects/counter"
)

var (
	// deviceID is camera device ID
	deviceID int
	// input is path to image or video file
	input string
	// min is minimum part area of assembly objec
	min int
	// max is maximum part area of assembly object
	max int
	// backend is inference backend
	backend int
	// target is inference target
	target int
	// publish is a flag which instructs the program to publish data analytics
	publish bool
	// rate is number of seconds between analytics are collected and sent to a remote server
	rate int
	// delay is video play delay
	delay float64
)

func init() {
	flag.IntVar(&deviceID, "device", -1, "Camera device ID")
	flag.StringVar(&input, "input", "", "Path to image or video file")
	flag.IntVar(&backend, "backend", 0, "Inference backend. 0: Auto, 1: Halide language, 2: Intel DL Inference Engine")
	flag.IntVar(&min, "min", 20000, "Minimum part area of assembly object")
	flag.IntVar(&max, "max", 30000, "Maximum part area of assembly object")
	flag.IntVar(&target, "target", 0, "Target device. 0: CPU, 1: OpenCL, 2: OpenCL half precision, 3: VPU")
	flag.BoolVar(&publish, "publish", false, "Publish data analytics to a remote server")
	flag.IntVar(&rate, "rate", 1, "Number of seconds between analytics are sent to a remote server")
	flag.Float64Var(&delay, "delay", 5.0, "Video playback delay")
}

// Status stores assembly line part status
type Status struct {
	// Seen means part was detected
	Seen bool
	// Defect means part has a defect
	Defect bool
}

// Part is assembly line object
type Part struct {
	// now is current status of Part
	now *Status
	// prev is previous status of Part
	prev *Status
	// defectFrames is number of consecutive frames where part had a defect
	defectFrames int
	// okFrames is number of consecutive frames where part was ok
	okFrames int
}

// Result is computation result returned to main goroutine
type Result struct {
	// Defect is used to signal the part defect was found.
	Defect bool
	// Rect is detected part rectangle area
	Rect image.Rectangle
	// TotalParts contains total number of detected parts
	TotalParts int
	// TotalDefects contains total number of defected parts
	TotalDefects int
}

// String implements fmt.Stringer interface for Result
func (r *Result) String() string {
	return fmt.Sprintf("Total parts: %d, Total defects: %v", r.TotalParts, r.TotalDefects)
}

// ToMQTTMessage turns result into MQTT message which can be published to MQTT broker
func (r *Result) ToMQTTMessage() string {
	return fmt.Sprintf("{\"Defect\":%v}", r.Defect)
}

// messageRunner reads data published to pubChan with rate frequency and sends them to remote analytics server
// doneChan is used to receive a signal from the main goroutine to notify the routine to stop and return
func messageRunner(doneChan <-chan struct{}, pubChan <-chan *Result, c *MQTTClient, topic string, rate int) error {
	ticker := time.NewTicker(time.Duration(rate) * time.Second)

	for {
		select {
		case <-ticker.C:
			result := <-pubChan
			_, err := c.Publish(topic, result.ToMQTTMessage())
			// TODO: decide whether to return with error and stop program;
			// For now we just signal there was an error and carry on
			if err != nil {
				fmt.Printf("Error publishing message to %s: %v", topic, err)
			}
		case <-pubChan:
			// we discard messages in between ticker times
		case <-doneChan:
			fmt.Printf("Stopping messageRunner: received stop sginal\n")
			return nil
		}
	}
}

// detectStatus detects part status from the blob and returns it
func detectStatus(blob *image.Rectangle) *Status {
	area := blob.Size().X * blob.Size().Y
	// we assume no part is detected; therefore there is no defect
	status := &Status{
		Defect: false,
		Seen:   false,
	}

	if area != 0 {
		status.Seen = true
		// defected part
		if area > max || area < min {
			status.Defect = true
			return status
		}
		// no defect
		return status
	}

	// no part detected
	return status
}

// detectBlob detects assembly line part in img image and returns it
func detectBlob(img *gocv.Mat) image.Rectangle {
	size := image.Point{3, 3}

	// convert to gray and blur
	gocv.CvtColor(*img, img, gocv.ColorBGRToGray)
	gocv.GaussianBlur(*img, img, size, 0, 0, gocv.BorderDefault)

	// Morphology: OPEN -> CLOSE -> OPEN
	// MORPH_OPEN removes the noise and closes the "holes" in the background
	// MORPH_CLOSE remove the noise and closes the "holes" in the foreground
	gocv.MorphologyEx(*img, img, gocv.MorphOpen, gocv.GetStructuringElement(gocv.MorphEllipse, size))
	gocv.MorphologyEx(*img, img, gocv.MorphClose, gocv.GetStructuringElement(gocv.MorphEllipse, size))
	gocv.MorphologyEx(*img, img, gocv.MorphOpen, gocv.GetStructuringElement(gocv.MorphEllipse, size))

	// threshold the image to emphasize assembly part
	gocv.Threshold(*img, img, 200, 255, gocv.ThresholdBinary)
	// find the contours of assembly part
	contours := gocv.FindContours(*img, gocv.RetrievalExternal, gocv.ChainApproxNone)

	// part will be the biggest contour area
	var maxRect image.Rectangle
	maxArea := 0

	for i := range contours {
		rect := gocv.BoundingRect(contours[i])
		area := rect.Size().X * rect.Size().Y
		// is large enough, and completely within the camera with no overlapping edges
		if area > maxArea && rect.In(image.Rect(0, 0, img.Cols(), img.Rows())) && rect.Size().X > 30 {
			maxArea = area
			maxRect = rect
		}
	}

	return maxRect
}

// frameRunner reads image frames from framesChan and performs face and sentiment detections on them
// doneChan is used to receive a signal from the main goroutine to notify frameRunner to stop and return
func frameRunner(framesChan <-chan *frame, doneChan <-chan struct{},
	resultsChan chan<- *Result, pubChan chan<- *Result) error {

	// frame is image frame
	frame := new(frame)
	// Result stores detection results
	result := new(Result)
	// Part is assembly object part
	part := new(Part)
	now, prev := new(Status), new(Status)
	part.now, part.prev = now, prev

	for {
		select {
		case <-doneChan:
			fmt.Printf("Stopping frameRunner: received stop sginal\n")
			// close results channel
			close(resultsChan)
			// close publish channel
			if pubChan != nil {
				close(pubChan)
			}
			return nil
		case frame = <-framesChan:
			if frame == nil {
				continue
			}
			// let's make a copy of the original
			img := gocv.NewMat()
			frame.img.CopyTo(&img)

			// datect blob on assembly line
			result.Rect = detectBlob(&img)

			// detect status of the blob
			part.now = detectStatus(&result.Rect)

			if part.now.Seen {
				// if part was detected add it to results
				// increment part counters
				if part.now.Defect {
					part.defectFrames++
				} else {
					part.okFrames++
				}

				if part.prev.Seen {
					// if the previously seen part has had no defect detected
					// in 10 previous consecutive frames reset its defetFrames counter
					if !part.now.Defect && part.okFrames > 10 {
						part.defectFrames = 0
					}
					// if previously seen part has had a defect detected
					// in 10 consecutive frames mark the part as defected
					if part.now.Defect && part.defectFrames > 10 {
						// if it didn't have a defect already
						if !part.prev.Defect {
							// set defect and increment total defect count
							result.Defect = true
							result.TotalDefects++
						}
						// part as a defect; reset okFrames count
						part.okFrames = 0
					}
				} else {
					// We havent seen the part before:
					// increment total count of all detected parts
					result.TotalParts++
				}
			} else {
				// no part detected -- empty belt: reset counts
				part.okFrames = 0
				part.defectFrames = 0
			}

			// send data down the channels
			resultsChan <- result
			if pubChan != nil {
				pubChan <- result
			}

			// set prev status to current
			part.prev = part.now
			// close image matrices
			img.Close()
		}
	}
}

// NewCapture creates new video capture from input or camera backend if input is empty and returns it.
// If input is not empty, NewCapture adjusts delay parameter so video playback matches FPS in the video file.
// It fails with error if it either can't open the input video file or the video device
func NewCapture(input string, deviceID int, delay *float64) (*gocv.VideoCapture, error) {
	if input != "" {
		// open video file
		vc, err := gocv.VideoCaptureFile(input)
		if err != nil {
			return nil, err
		}

		fps := vc.Get(gocv.VideoCaptureFPS)
		*delay = 1000 / fps

		return vc, nil
	}

	// open camera device
	vc, err := gocv.VideoCaptureDevice(deviceID)
	if err != nil {
		return nil, err
	}

	return vc, nil
}

// NewMQTTPublisher creates new MQTT client which collects analytics data and publishes them to remote MQTT server.
// It attempts to make a connection to the remote server and if successful it return the client handler
// It returns error if either the connection to the remote server failed or if the client config is invalid.
func NewMQTTPublisher() (*MQTTClient, error) {
	// create MQTT client and connect to MQTT server
	opts, err := MQTTClientOptions()
	if err != nil {
		return nil, err
	}

	// create MQTT client ad connect to remote server
	c, err := MQTTConnect(opts)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// frame ise used to send video frames and program configuration to upstream goroutines
type frame struct {
	// img is image frame
	img *gocv.Mat
}

func main() {
	// parse cli flags
	flag.Parse()
	// create new video capture
	vc, err := NewCapture(input, deviceID, &delay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new video capture: %v\n", err)
		os.Exit(1)
	}
	defer vc.Close()

	// frames channel provides the source of images to process
	framesChan := make(chan *frame, 1)

	// errChan is a channel used to capture program errors
	errChan := make(chan error, 2)

	// doneChan is used to signal goroutines they need to stop
	doneChan := make(chan struct{})

	// resultsChan is used for detection distribution
	resultsChan := make(chan *Result, 1)

	// sigChan is used as a handler to stop all the goroutines
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	// pubChan is used for publishing data analytics stats
	var pubChan chan *Result

	// waitgroup to synchronize all goroutines
	var wg sync.WaitGroup

	if publish {
		p, err := NewMQTTPublisher()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create MQTT publisher: %v\n", err)
			os.Exit(1)
		}
		pubChan = make(chan *Result, 1)
		// start MQTT worker goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			errChan <- messageRunner(doneChan, pubChan, p, topic, rate)
		}()
		defer p.Disconnect(100)
	}

	// start frameRunner goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		errChan <- frameRunner(framesChan, doneChan, resultsChan, pubChan)
	}()

	// open display window
	window := gocv.NewWindow(name)
	window.SetWindowProperty(gocv.WindowPropertyFullscreen, gocv.WindowAutosize)
	defer window.Close()

	// prepare input image matrix
	img := gocv.NewMat()
	defer img.Close()

	// initialize the result pointer
	result := new(Result)

monitor:
	for {
		if ok := vc.Read(&img); !ok {
			fmt.Printf("Cannot read image source %v\n", deviceID)
			break
		}
		if img.Empty() {
			continue
		}

		// resize frame image to smaller size
		gocv.Resize(img, &img, image.Point{960, 540}, 0, 0, gocv.InterpolationLinear)
		screen := img.Clone()
		framesChan <- &frame{img: &img}

		select {
		case sig := <-sigChan:
			fmt.Printf("Shutting down. Got signal: %s\n", sig)
			break monitor
		case err = <-errChan:
			fmt.Printf("Shutting down. Encountered error: %s\n", err)
			break monitor
		case result = <-resultsChan:
			// do nothing here
		default:
			// do nothing; just display latest results
		}

		// display detected measurements
		gocv.PutText(&screen, fmt.Sprintf("Measurement: %d Expected range: [%d - %d] Defect: %v",
			result.Rect.Size().X*result.Rect.Size().Y, min, max, result.Defect), image.Point{0, 15},
			gocv.FontHersheySimplex, 0.5, color.RGBA{0, 255, 0, 0}, 2)

		// defect detection results
		gocv.PutText(&screen, fmt.Sprintf("%s", result), image.Point{0, 40},
			gocv.FontHersheySimplex, 0.5, color.RGBA{0, 255, 0, 0}, 2)

		// if defect then draw red rectangle; otherwise draw green
		if result.Defect {
			gocv.Rectangle(&screen, result.Rect, color.RGBA{255, 0, 0, 0}, 2)
		} else if !result.Rect.Empty() {
			gocv.Rectangle(&screen, result.Rect, color.RGBA{0, 255, 0, 0}, 2)
		}

		// show the image in the window, and wait 1 millisecond
		window.IMShow(screen)

		// press ESC key to exit
		if window.WaitKey(int(delay)) == 27 {
			break monitor
		}
	}

	// signal all goroutines to finish
	close(framesChan)
	close(doneChan)
	for range resultsChan {
		// collect any outstanding results
	}

	// wait for all goroutines to finish
	wg.Wait()
}
