package main

import (
	"covid19/epd"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
	"periph.io/x/periph/host"
)

func main() {
	startTime := time.Now()
	fmt.Println("Start")

	registerFonts()

	// TEST - save to file
	// stats, _ := download()
	// dest := draw(stats, &covidStats{})
	// draw2dimg.SaveToPngFile("test.png", dest)

	// Raspberry PI - display on eink diplay
	host.Init()
	fmt.Println("init")

	e := epd.CreateEpd()
	defer e.Close()

	prev := &covidStats{}

	for n := 0; n >= 0; n++ {
		stats, err := download()
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		fmt.Println(stats)

		e.Init()
		// e.Clear()

		dest := draw(stats, prev)

		draw2dimg.SaveToPngFile("test.png", dest)

		data := getBuffer(dest)
		fmt.Println(data[:1])

		e.DisplayBlack(data)
		e.Sleep()

		if prev.cases == 0 {
			prev = stats
		}

		if n == 0 {
			time.Sleep(20 * time.Second)
		} else {
			time.Sleep(5 * time.Minute)
		}
	}

	endTime := time.Since(startTime)
	fmt.Println("Done in:", endTime.Seconds(), "s")
}

func draw(stats, prev *covidStats) *image.RGBA {
	width := 104
	height := 212
	dest := image.NewRGBA(image.Rect(0, 0, height, width)) // horizontal
	gc := draw2dimg.NewGraphicContext(dest)

	black := color.RGBA{0x00, 0x00, 0x00, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	gc.SetFillColor(white)
	drawRect(gc, 0, 0, float64(height), float64(width))
	gc.Fill()

	gc.SetFillColor(black)
	drawRect(gc, 0, 8, 212, 1)
	gc.Fill()
	drawRect(gc, 0, 95, 212, 1)
	gc.Fill()

	drawStats(gc, stats, prev)

	return dest
}

func drawRect(gc *draw2dimg.GraphicContext, x, y, w, h float64) {
	gc.BeginPath()
	gc.MoveTo(x, y)
	gc.LineTo(x+w, y)
	gc.LineTo(x+w, y+h)
	gc.LineTo(x, y+h)
	gc.Close()
}

func drawStats(gc draw2d.GraphicContext, stats, prev *covidStats) {
	// Set the fill text color to black
	gc.SetFillColor(image.Black)

	gc.SetDPI(72) // 16 m3x6
	// gc.SetDPI(96) // 12 m3x6
	gc.SetFontSize(16)

	gc.SetFontData(draw2d.FontData{
		Name: "m3x6",
	})

	row := 6.0 + 4.0
	gc.FillStringAt(stats.lastUpdated, 1, row-3)

	gc.SetFontSize(16)
	gc.FillStringAt("cases:", 1, 4*row)
	gc.SetFontSize(32)
	gc.FillStringAt(strconv.Itoa(stats.cases), 60, 4*row)
	gc.FillStringAt(strconv.Itoa(stats.czCases), 150, 4*row)

	gc.SetFontSize(16)
	gc.FillStringAt("recovered:", 1, 6*row-2)
	gc.FillStringAt(strconv.Itoa(stats.recovered), 60, 6*row-2)
	gc.FillStringAt(strconv.Itoa(stats.czRecovered), 150, 6*row-2)
	gc.FillStringAt("deaths:", 1, 7*row-2)
	gc.FillStringAt(strconv.Itoa(stats.deaths), 60, 7*row-2)
	gc.FillStringAt(strconv.Itoa(stats.czDeaths), 150, 7*row-2)
	gc.FillStringAt("(+"+strconv.Itoa(stats.czNew)+")", 150, 8*row-2)
	// if prev.cases > 0 {
	// 	gc.FillStringAt("(+"+strconv.Itoa(stats.cases-prev.cases)+")", 60, 8*row-2)
	// }

	gc.FillStringAt("Last refreshed: "+time.Now().Format(time.RFC3339), 1, 103)
}

type covidStats struct {
	lastUpdated string
	cases       int
	deaths      int
	recovered   int
	czCases     int
	czDeaths    int
	czRecovered int
	czNew       int
}

func download() (*covidStats, error) {

	response, err := http.Get("https://www.worldometers.info/coronavirus/")
	if err != nil {
		return nil, err
	}

	contentType := response.Header.Get("Content-Type")

	if contentType[0:9] == "text/html" {
		bodyBytes, _ := ioutil.ReadAll(response.Body)
		text := string(bodyBytes)

		p := strings.NewReader(text)
		doc, _ := goquery.NewDocumentFromReader(p)

		stats := covidStats{
			lastUpdated: doc.Find(".label-counter").Next().Text(),
		}

		totalElements := doc.Find("#maincounter-wrap .maincounter-number span")
		totalElements.Each(func(i int, s *goquery.Selection) {
			totalString := s.Text()
			totalString = strings.TrimSpace(totalString)
			// totalString = strings.ReplaceAll(totalString, ",", "")
			totalString = strings.Replace(totalString, ",", "", -1)
			total, _ := strconv.Atoi(totalString)

			switch i {
			case 0:
				stats.cases = total
			case 1:
				stats.deaths = total
			case 2:
				stats.recovered = total
			}
		})

		tableElements := doc.Find("#main_table_countries tbody tr")
		row := tableElements.First()
		for i := 0; i < tableElements.Length(); i++ {
			tableElements.Get(i)
			cells := row.Find("td")
			country := cells.First().Text()
			country = strings.TrimSpace(country)
			if country == "Czechia" {
				cells.Each(func(i int, s *goquery.Selection) {
					fmt.Printf("%v - %v\n", i, s.Text())
					switch i {
					case 1:
						stats.czCases = toNumber(s.Text())
					case 2:
						stats.czNew = toNumber(strings.Replace(s.Text(), "+", "", -1))
					case 3:
						stats.czDeaths = toNumber(s.Text())
					case 5:
						stats.czRecovered = toNumber(s.Text())
					}
				})
				break
			}
			row = row.Next()
		}

		fmt.Printf("%s\n%v\n%v X %v\n", stats.lastUpdated, stats.cases, stats.deaths, stats.recovered)

		return &stats, nil
	}

	fmt.Println("Failed")
	return nil, errors.New("Invalid data")
}

func toNumber(txt string) int {
	txt = strings.TrimSpace(txt)
	// totalString = strings.ReplaceAll(totalString, ",", "")
	txt = strings.Replace(txt, ",", "", -1)
	num, _ := strconv.Atoi(txt)
	return num
}

func getBuffer(image *image.RGBA) []byte {
	width := 104
	height := 212

	size := (width * height) / 8
	data := make([]byte, size)
	for i := range data {
		data[i] = 255
	}

	imageWidth := image.Rect.Dx()
	imageHeight := image.Rect.Dy()

	if imageWidth == width && imageHeight == height {
		fmt.Println("Vertical")
		for y := 0; y < imageHeight; y++ {
			for x := 0; x < imageWidth; x++ {
				if isBlack(image, x, y) {
					shift := uint32(x % 8)
					data[(x+y*width)/8] &= ^(0x80 >> shift)
				}
			}
		}
	} else if imageWidth == height && imageHeight == width {
		fmt.Println("Horizontal")
		for y := 0; y < imageHeight; y++ {
			for x := 0; x < imageWidth; x++ {
				newX := y
				newY := height - x - 1
				if isBlack(image, x, y) {
					shift := uint32(y % 8)
					data[(newX+newY*width)/8] &= ^(0x80 >> shift)
				}
			}
		}
	} else {
		fmt.Println("Invalid image size")
	}
	return data
}

func getRGBA(image *image.RGBA, x, y int) (int, int, int, int) {
	r, g, b, a := image.At(x, y).RGBA()
	r = r / 257
	g = g / 257
	b = b / 257
	a = a / 257

	return int(r), int(g), int(b), int(a)
}

func isBlack(image *image.RGBA, x, y int) bool {
	r, g, b, a := getRGBA(image, x, y)
	offset := 10
	return r < 255-offset && g < 255-offset && b < 255-offset && a > offset
}

func registerFonts() {
	m3x6Font := parseFont("m3x6")
	draw2d.RegisterFont(draw2d.FontData{
		Name: "m3x6",
	}, m3x6Font)

	fmt.Println("fonts registered")
}

func parseFont(name string) (f *truetype.Font) {
	// wd, _ := os.Getwd()
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	if dir[0:4] == "/tmp" {
		dir = "."
	}
	b, err := ioutil.ReadFile(fmt.Sprintf("%s/font/%s.ttf", dir, name))
	// b, err := ioutil.ReadFile(fmt.Sprintf("/home/pi/apps/go-eink-ip/font/%s.ttf", name))
	if err != nil {
		return nil
	}
	f, err = truetype.Parse(b)
	if err != nil {
		return nil
	}

	return f
}
