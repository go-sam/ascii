package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sam/args"
	"github.com/go-sam/colours"
	tw "github.com/go-sam/typewriter"
	"github.com/nfnt/resize"
)

var asciiChars = " .:-=+*#"
var randColourMap [8][3]uint8

type ColourMode int

const (
	Monochrome ColourMode = iota
	Posterized
	Colourful
	Random
)

type MirrorMode int

const (
	MirrorNone MirrorMode = iota
	MirrorX
	MirrorY
	MirrorXY
)

// Holds all configuration options for the ASCII art generator.
//
// Contains settings for input source, output dimensions, color rendering,
// display behavior, and typewriter animation speed.
type Config struct {
	imageFile  string
	folderPath string
	imageWidth int
	colourMode ColourMode
	loopMode   bool
	printSpeed int
	mirrorMode MirrorMode
}

//go:embed portrait_kim_kitsuragi.png
var defaultImageData []byte

func main() {
	config := parseArguments()

	if config.folderPath != "" {
		handleMultipleImages(config)
	} else {
		handleSingleImage(config)
	}
}

// Processes a single image file.
//
// Loads the image, resizes it, and either prints it once or continuously
// loops based on the loop mode setting.
func handleSingleImage(config Config) {
	img, err := loadImage(config.imageFile)
	if err != nil {
		return
	}

	resized := resizeImage(img, config.imageWidth)

	if config.loopMode {
		printImageLoop(config, resized)
	}

	printImage(config, resized)
}

// Processes all images in a folder.
//
// Gets the list of image files and either prints them once or continuously
// loops through them based on the loop mode setting.
func handleMultipleImages(config Config) {
	imageFiles := getImagesInFolder(config)

	if config.loopMode {
		for {
			printImages(config, imageFiles)
		}
	} else {
		printImages(config, imageFiles)
	}
}

// Converts an image to ASCII art and displays it with typewriter effect.
//
// Regenerates random colors if in Random mode, then uses the typewriter
// package to print the ASCII art at the specified speed.
func printImage(config Config, img image.Image) {
	if config.colourMode == Random {
		generateRandColourMap()
	}

	if config.mirrorMode != MirrorNone {
		img = mirrorImage(img, config.mirrorMode)
	}

	t := tw.Typewriter{Text: imageToASCII(img, config.colourMode), Speed: 1000 / config.printSpeed}
	t.Type()
}

func printImageLoop(config Config, img image.Image) {
	for {
		printImage(config, img)
		fmt.Println()
		time.Sleep(1 * time.Second)
	}
}

// Processes and displays multiple images in sequence.
//
// Loads each image, resizes it, converts to ASCII art, and prints with
// a 1-second delay between images. Skips any images that fail to load.
func printImages(config Config, imageFiles []string) {
	for _, file := range imageFiles {
		img, err := loadImage(file)

		if err != nil {
			continue
		}

		resized := resizeImage(img, config.imageWidth)
		printImage(config, resized)

		fmt.Println()
		time.Sleep(1 * time.Second)
	}
}

// Kim Kitsuragi
func loadEmbeddedImage() (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(defaultImageData))
	return img, err
}

// Loads an image from a file or returns the embedded default image.
//
// If filename is "default" or empty, loads the embedded Kim Kitsuragi portrait.
// Otherwise opens and decodes the specified image file (supports PNG/JPEG).
func loadImage(filename string) (image.Image, error) {
	if filename == "default" || filename == "" {
		return loadEmbeddedImage()
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)

	return img, err
}

// Resizes an image to the specified width while maintaining aspect ratio.
//
// Height is calculated proportionally and divided by 2 to account for ASCII character
// aspect ratio, using Lanczos3 interpolation for high quality resizing.
func resizeImage(img image.Image, width int) image.Image {
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	height := uint((originalHeight * width) / originalWidth / 2)

	return resize.Resize(uint(width), height, img, resize.Lanczos3)
}

func mirrorImage(img image.Image, mirrorMode MirrorMode) image.Image {
	if mirrorMode == MirrorNone {
		return img
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	mirroredImg := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := range height {
		for x := range width {
			srcX, srcY := x, y

			switch mirrorMode {
			case MirrorX:
				srcX = width - 1 - x

			case MirrorY:
				srcY = height - 1 - y

			case MirrorXY:
				srcX = width - 1 - x
				srcY = height - 1 - y
			}

			mirroredImg.Set(x, y, img.At(srcX, srcY))
		}
	}

	return mirroredImg
}

// Converts RGB values to an ASCII character and color values.
//
// Uses standard luminance formula to map brightness to one of 8 ASCII chars.
// Returns the character and 8-bit RGB values for terminal output.
func processPixel(r, g, b uint32, colourMode ColourMode) (char byte, pR, pG, pB uint8) {
	// Convert to grayscale (0.0 to 1.0) and divide by 2^16
	gray := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65536

	// Get ASCII character based on brightness
	charIndex := int(gray * float64(len(asciiChars)-1))
	if charIndex >= len(asciiChars) {
		charIndex = len(asciiChars) - 1
	}
	char = asciiChars[charIndex]

	// Convert to 8-bit RGB for color processing
	pR, pG, pB = uint8(r>>8), uint8(g>>8), uint8(b>>8)

	if colourMode == Random {
		colors := randColourMap[charIndex]
		pR, pG, pB = colors[0], colors[1], colors[2]
	}

	return char, pR, pG, pB
}

// Converts an image to colored ASCII art string.
//
// Processes each pixel to determine ASCII character and color, then formats
// with ANSI color codes based on the specified color mode.
func imageToASCII(img image.Image, colourMode ColourMode) string {
	bounds := img.Bounds()
	reset := string(colours.Reset)
	var result strings.Builder

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			char, pR, pG, pB := processPixel(r, g, b, colourMode)

			if colourMode == Monochrome {
				result.WriteByte(char)
				continue
			}

			if colourMode == Posterized {
				pR, pG, pB = posterizePixel(r, g, b)
			}

			colourCode := colours.RGB2ANSI(pR, pG, pB)
			result.WriteString(colourCode)
			result.WriteByte(char)
			result.WriteString(reset)
		}
		result.WriteByte('\n')
	}

	return result.String()
}

// Convert colour to 8-bit and Posterize each channel to 2 levels
// (2^3 = 8 combinations) for 8 possible colours
func posterizePixel(r, g, b uint32) (uint8, uint8, uint8) {
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

	levels := 2
	step := 255 / (levels - 1)

	posterR := uint8((int(r8) / (256 / levels)) * step)
	posterG := uint8((int(g8) / (256 / levels)) * step)
	posterB := uint8((int(b8) / (256 / levels)) * step)

	return posterR, posterG, posterB
}

// Scans a directory for image files and returns their full paths.
//
// Only includes .jpg, .jpeg, and .png files.
func getImagesInFolder(config Config) []string {
	files, _ := os.ReadDir(config.folderPath)
	var imageFiles []string

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(file.Name()))

		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			continue
		}

		imageFiles = append(imageFiles, filepath.Join(config.folderPath, file.Name()))
	}

	return imageFiles
}

// Populates the random color map with one random color per ASCII character.
//
// Each of the 8 ASCII characters gets assigned a unique random RGB color for Random color mode.
func generateRandColourMap() {
	for i := range len(asciiChars) {
		r, g, b := colours.RandomRGB()
		randColourMap[i] = [3]uint8{r, g, b}
	}
}

// Parses command-line arguments and returns a Config struct
func parseArguments() Config {
	config := Config{
		imageFile:  "default",
		folderPath: "",
		imageWidth: 80,
		colourMode: Posterized,
		loopMode:   false,
		printSpeed: 1000,
		mirrorMode: MirrorNone,
	}

	if len(os.Args) < 1 {
		return config
	}

	parser := args.New()

	// Help
	if parser.Help() {
		printHelpMessage()
	}

	// Input
	parser.String("i", "image", &config.imageFile)
	parser.String("f", "folder", &config.folderPath)

	// Colour flags
	if parser.HasFlag("m", "monochrome") {
		config.colourMode = Monochrome
	} else if parser.HasFlag("p", "posterized") {
		config.colourMode = Posterized
	} else if parser.HasFlag("c", "colourful") {
		config.colourMode = Colourful
	} else if parser.HasFlag("r", "random") {
		config.colourMode = Random
	}

	// Options
	parser.Integer("w", "width", &config.imageWidth)
	parser.Integer("s", "speed", &config.printSpeed)
	parser.Bool("l", "loop", &config.loopMode)

	// Enum Options
	if value, ok := parser.GetStringValue("mr", "mirror"); ok {
		fmt.Print(value)
		switch value {
		case "x":
			config.mirrorMode = MirrorX
		case "y":
			config.mirrorMode = MirrorY
		case "xy":
			config.mirrorMode = MirrorXY
		}
	}

	// Validation
	if err := parser.ValidateArgs(); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	return config
}

func printHelpMessage() {
	fmt.Println("ASCII Art Generator")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("Converts images (PNG/JPEG) to colored ASCII art using 8 ASCII characters.")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("ascii [OPTIONS]")
	fmt.Println()
	fmt.Println("INPUT:")
	fmt.Println("  -i, --image <file>     Image file to convert (default: Kim Kitsuragi)")
	fmt.Println("  -f, --folder <path>    When used, will print all images in the given folder")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  -w, --width <number>   Width of ASCII output in characters (default: 80)")
	fmt.Println("  -l, --loop             Enable Loop Mode, which prints the image forever (default: false)")
	fmt.Println("  -s, --speed            The speed of printing, in chars per second (default: 1000)")
	fmt.Println("  -mr, --mirror <mode>   Mirror the image: x, y, or xy (default: none)")
	fmt.Println()
	fmt.Println("COLOR MODES:")
	fmt.Println("  -m, --monochrome       Black and white ASCII art")
	fmt.Println("  -p, --posterized       8-color posterized ASCII art (default)")
	fmt.Println("  -c, --colourful        Full-color ASCII art")
	fmt.Println("  -r, --random           Randomised colours!")
	fmt.Println()
	fmt.Println("  -h, --help             Show this help message")
	fmt.Println()
	fmt.Println("ASCII CHARACTERS USED: " + asciiChars)

	os.Exit(0)
}
