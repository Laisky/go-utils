package cmd

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/nfnt/resize"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"

	_ "image/gif"
	_ "image/jpeg"
)

const (
	faviconFormatICO = "ico"
	faviconFormatPNG = "png"
)

var defaultFaviconSizes = []int{16, 32, 48, 64, 128, 256}

// imageFaviconOptions defines the options for generating favicon files.
type imageFaviconOptions struct {
	Input  string
	Output string
	Sizes  []int
	Format string
	Force  bool
}

var imageFaviconArgs = imageFaviconOptions{}

// init registers the image commands and flags.
func init() {
	rootCmd.AddCommand(imageCMD)
	imageCMD.AddCommand(imageFaviconCmd)

	imageFaviconCmd.Flags().StringVarP(&imageFaviconArgs.Input,
		"input", "i", "", "input image path")
	imageFaviconCmd.Flags().StringVarP(&imageFaviconArgs.Output,
		"output", "o", "", "output filename, should be in the same directory as input")
	imageFaviconCmd.Flags().IntSliceVar(&imageFaviconArgs.Sizes,
		"sizes", defaultFaviconSizes, "favicon sizes, e.g. 16,32,48,64,128,256")
	imageFaviconCmd.Flags().StringVar(&imageFaviconArgs.Format,
		"format", faviconFormatICO, "output format: ico or png")
	imageFaviconCmd.Flags().BoolVar(&imageFaviconArgs.Force,
		"force", false, "overwrite output file if it already exists")
}

var imageCMD = &cobra.Command{
	Use:   "image",
	Short: "image tools",
	Long: gutils.Dedent(`
		Image tools.
	`),
	Args: NoExtraArgs,
}

var imageFaviconCmd = &cobra.Command{
	Use:   "favicon",
	Short: "generate favicon from image",
	Long: gutils.Dedent(`
		Generate favicon from an image by cropping to square, resizing, and compressing.

		Examples:
			$ gutils image favicon -i /path/to/logo.png
			$ gutils image favicon -i /path/to/logo.png --format png --sizes 256
	`),
	Args: NoExtraArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		outputPath, err := generateFaviconFile(imageFaviconArgs)
		if err != nil {
			return errors.Wrap(err, "generate favicon")
		}

		glog.Shared.Info("favicon generated", zap.String("output", outputPath))
		return nil
	},
}

type faviconEntry struct {
	size int
	data []byte
}

// generateFaviconFile creates a favicon file based on the provided arguments.
// It takes the input path, output name, sizes, format, and overwrite flag, then returns the output path or an error.
func generateFaviconFile(args imageFaviconOptions) (string, error) {
	input := strings.TrimSpace(args.Input)
	if input == "" {
		return "", errors.New("input image path is required")
	}

	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format == "" {
		format = faviconFormatICO
	}
	if format != faviconFormatICO && format != faviconFormatPNG {
		return "", errors.Errorf("unsupported format %q", format)
	}

	sizes, err := normalizeFaviconSizes(args.Sizes)
	if err != nil {
		return "", errors.Wrap(err, "normalize favicon sizes")
	}

	inputAbs, err := filepath.Abs(input)
	if err != nil {
		return "", errors.Wrapf(err, "get absolute path for %q", input)
	}

	outputPath, err := buildFaviconOutputPath(inputAbs, args.Output, format)
	if err != nil {
		return "", errors.Wrap(err, "build output path")
	}
	if outputPath == inputAbs {
		return "", errors.New("output path must be different from input path")
	}

	if !args.Force {
		if err = assertFileNotExists(outputPath); err != nil {
			return "", errors.Wrap(err, "check output path")
		}
	}

	src, err := loadImage(inputAbs)
	if err != nil {
		return "", errors.Wrap(err, "load image")
	}
	square := cropCenterSquare(src)

	switch format {
	case faviconFormatPNG:
		outputSize := sizes[len(sizes)-1]
		data, err := encodePNG(resizeSquare(square, outputSize))
		if err != nil {
			return "", errors.Wrapf(err, "encode png size %d", outputSize)
		}
		if err = os.WriteFile(outputPath, data, 0o600); err != nil {
			return "", errors.Wrapf(err, "write output file %q", outputPath)
		}
		return outputPath, nil
	case faviconFormatICO:
		data, err := buildFaviconICO(square, sizes)
		if err != nil {
			return "", errors.Wrap(err, "build ico")
		}
		if err = os.WriteFile(outputPath, data, 0o600); err != nil {
			return "", errors.Wrapf(err, "write output file %q", outputPath)
		}
		return outputPath, nil
	default:
		return "", errors.Errorf("unsupported format %q", format)
	}
}

// buildFaviconOutputPath builds the output path using the input image path, output filename, and format.
// It returns the resolved output path or an error if the path is invalid or not in the input directory.
func buildFaviconOutputPath(inputPath, output, format string) (string, error) {
	inputDir := filepath.Dir(inputPath)
	filename := strings.TrimSpace(output)
	if filename == "" {
		filename = "favicon." + format
	}

	outputPath := filename
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(inputDir, outputPath)
	}
	outputPath = filepath.Clean(outputPath)
	if filepath.Dir(outputPath) != inputDir {
		return "", errors.Errorf("output must be in the same directory as %q", inputDir)
	}

	return outputPath, nil
}

// assertFileNotExists checks whether the given file path is absent.
// It returns nil when the path does not exist, otherwise returns an error.
func assertFileNotExists(fpath string) error {
	if _, err := os.Stat(fpath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "stat output file %q", fpath)
	}

	return errors.Errorf("output file already exists: %q", fpath)
}

// normalizeFaviconSizes validates, deduplicates, and sorts favicon sizes.
// It returns the normalized sizes or an error if input contains invalid values.
func normalizeFaviconSizes(sizes []int) ([]int, error) {
	if len(sizes) == 0 {
		sizes = append([]int(nil), defaultFaviconSizes...)
	}

	uniq := make(map[int]struct{}, len(sizes))
	normalized := make([]int, 0, len(sizes))
	for _, size := range sizes {
		if size <= 0 {
			return nil, errors.Errorf("size must be positive: %d", size)
		}
		if _, ok := uniq[size]; ok {
			continue
		}
		uniq[size] = struct{}{}
		normalized = append(normalized, size)
	}
	sort.Ints(normalized)
	if len(normalized) == 0 {
		return nil, errors.New("no valid sizes found")
	}

	return normalized, nil
}

// loadImage decodes the image from the given file path and returns it or an error.
func loadImage(fpath string) (image.Image, error) {
	fp, err := os.Open(fpath)
	if err != nil {
		return nil, errors.Wrapf(err, "open image %q", fpath)
	}
	defer gutils.SilentClose(fp)

	img, _, err := image.Decode(fp)
	if err != nil {
		return nil, errors.Wrapf(err, "decode image %q", fpath)
	}

	return img, nil
}

// cropCenterSquare crops the provided image into a centered square and returns the cropped image.
func cropCenterSquare(src image.Image) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	size := width
	if height < width {
		size = height
	}

	startX := bounds.Min.X + (width-size)/2
	startY := bounds.Min.Y + (height-size)/2
	rect := image.Rect(startX, startY, startX+size, startY+size)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Src)
	return dst
}

// resizeSquare resizes the provided image to the requested size and returns the resized image.
func resizeSquare(src image.Image, size int) image.Image {
	if size <= 0 {
		return src
	}

	return resize.Resize(uint(size), uint(size), src, resize.Lanczos3)
}

// encodePNG encodes the provided image to PNG with best compression and returns the bytes or an error.
func encodePNG(src image.Image) ([]byte, error) {
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buf, src); err != nil {
		return nil, errors.Wrap(err, "encode png")
	}

	return buf.Bytes(), nil
}

// buildFaviconICO builds an ICO file containing multiple PNG-encoded sizes.
// It takes a source image and size list, then returns the ICO bytes or an error.
func buildFaviconICO(src image.Image, sizes []int) ([]byte, error) {
	entries := make([]faviconEntry, 0, len(sizes))
	for _, size := range sizes {
		if size > 256 {
			return nil, errors.Errorf("ico size must be <= 256, got %d", size)
		}
		encoded, err := encodePNG(resizeSquare(src, size))
		if err != nil {
			return nil, errors.Wrapf(err, "encode png size %d", size)
		}
		entries = append(entries, faviconEntry{
			size: size,
			data: encoded,
		})
	}

	return encodeICO(entries)
}

// countICOEntries converts entries length into uint16 with overflow protection.
// The entries parameter is the list of ICO images and the return value is the ICO directory count field.
func countICOEntries(entries []faviconEntry) (uint16, error) {
	var count uint16
	for range entries {
		if count == ^uint16(0) {
			return 0, errors.Errorf("too many favicon entries: %d", len(entries))
		}
		count++
	}

	return count, nil
}

// faviconSizeToICOByte converts favicon size to ICO width and height byte values.
// The size parameter is in pixels and the returned byte uses 0 to represent 256.
func faviconSizeToICOByte(size int) (uint8, error) {
	if size == 256 {
		return 0, nil
	}
	if size <= 0 || size > 255 {
		return 0, errors.Errorf("invalid favicon size %d", size)
	}

	var encoded uint8
	for i := 0; i < size; i++ {
		encoded++
	}

	return encoded, nil
}

// faviconDataSize converts encoded image payload length into uint32 with overflow protection.
// The data parameter is the PNG payload and the returned value is used by ICO metadata.
func faviconDataSize(data []byte) (uint32, error) {
	var size uint32
	for range data {
		if size == ^uint32(0) {
			return 0, errors.Errorf("ico image data too large: %d", len(data))
		}
		size++
	}

	return size, nil
}

// encodeICO encodes entries into an ICO file binary and returns the ICO bytes or an error.
func encodeICO(entries []faviconEntry) ([]byte, error) {
	if len(entries) == 0 {
		return nil, errors.New("no favicon entries to encode")
	}

	var dir bytes.Buffer
	if err := binary.Write(&dir, binary.LittleEndian, uint16(0)); err != nil {
		return nil, errors.Wrap(err, "write ico reserved")
	}
	if err := binary.Write(&dir, binary.LittleEndian, uint16(1)); err != nil {
		return nil, errors.Wrap(err, "write ico type")
	}

	entryCount, err := countICOEntries(entries)
	if err != nil {
		return nil, err
	}
	if err := binary.Write(&dir, binary.LittleEndian, entryCount); err != nil {
		return nil, errors.Wrap(err, "write ico count")
	}

	offset := uint32(6) + uint32(entryCount)*16
	for _, entry := range entries {
		if entry.size <= 0 {
			return nil, errors.Errorf("invalid favicon size %d", entry.size)
		}

		sizeByte, err := faviconSizeToICOByte(entry.size)
		if err != nil {
			return nil, err
		}
		if err := dir.WriteByte(sizeByte); err != nil {
			return nil, errors.Wrap(err, "write ico width")
		}
		if err := dir.WriteByte(sizeByte); err != nil {
			return nil, errors.Wrap(err, "write ico height")
		}
		if err := dir.WriteByte(0); err != nil {
			return nil, errors.Wrap(err, "write ico color count")
		}
		if err := dir.WriteByte(0); err != nil {
			return nil, errors.Wrap(err, "write ico reserved byte")
		}
		if err := binary.Write(&dir, binary.LittleEndian, uint16(1)); err != nil {
			return nil, errors.Wrap(err, "write ico planes")
		}
		if err := binary.Write(&dir, binary.LittleEndian, uint16(32)); err != nil {
			return nil, errors.Wrap(err, "write ico bit count")
		}

		dataSize, err := faviconDataSize(entry.data)
		if err != nil {
			return nil, err
		}
		if err := binary.Write(&dir, binary.LittleEndian, dataSize); err != nil {
			return nil, errors.Wrap(err, "write ico data size")
		}
		if err := binary.Write(&dir, binary.LittleEndian, offset); err != nil {
			return nil, errors.Wrap(err, "write ico offset")
		}
		if offset > ^uint32(0)-dataSize {
			return nil, errors.Errorf("ico offset overflow: %d + %d", offset, dataSize)
		}
		offset += dataSize
	}

	var out bytes.Buffer
	if _, err := out.Write(dir.Bytes()); err != nil {
		return nil, errors.Wrap(err, "write ico header")
	}
	for _, entry := range entries {
		if _, err := out.Write(entry.data); err != nil {
			return nil, errors.Wrap(err, "write ico image data")
		}
	}

	return out.Bytes(), nil
}
