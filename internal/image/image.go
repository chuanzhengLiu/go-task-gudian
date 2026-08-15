package image

import (
	"ancient-texts-backend/internal/config"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/tiff"
)

var validImageTypes = map[string][]byte{
	"image/jpeg": {0xFF, 0xD8, 0xFF},
	"image/png":  {0x89, 0x50, 0x4E, 0x47},
	"image/tiff": {0x49, 0x49, 0x2A, 0x00},
	"image/tiff_le": {0x49, 0x49, 0x2A, 0x00},
	"image/tiff_be": {0x4D, 0x4D, 0x00, 0x2A},
}

const maxFileSize = 500 * 1024 * 1024

func ValidateImageFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	if info.Size() > maxFileSize {
		return "", errors.New("file size exceeds 500MB limit")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	header := make([]byte, 8)
	_, err = file.Read(header)
	if err != nil {
		return "", err
	}

	contentType := ""
	for mimeType, magic := range validImageTypes {
		if len(header) >= len(magic) {
			match := true
			for i := range magic {
				if header[i] != magic[i] {
					match = false
					break
				}
			}
			if match {
				if strings.HasPrefix(mimeType, "image/tiff") {
					contentType = "image/tiff"
				} else {
					contentType = mimeType
				}
				break
			}
		}
	}

	if contentType == "" {
		return "", errors.New("invalid file type, only TIFF, PNG, JPG are allowed")
	}

	return contentType, nil
}

func SaveUploadedFile(srcPath string, projectID uint64) (string, string, error) {
	contentType, err := ValidateImageFile(srcPath)
	if err != nil {
		return "", "", err
	}

	ext := ".jpg"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/tiff":
		ext = ".tif"
	}

	projectDir := fmt.Sprintf("%s/%d", config.AppConfig.UPLOAD_DIR, projectID)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", "", err
	}

	fileName := uuid.New().String() + ext
	destPath := filepath.Join(projectDir, fileName)
	relativePath := fmt.Sprintf("%d/%s", projectID, fileName)

	if err := copyFile(srcPath, destPath); err != nil {
		return "", "", err
	}

	return relativePath, destPath, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func LoadImage(path string) (image.Image, string, error) {
	fullPath := filepath.Join(config.AppConfig.UPLOAD_DIR, path)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	header := make([]byte, 8)
	_, err = file.Read(header)
	if err != nil {
		return nil, "", err
	}
	file.Seek(0, 0)

	var img image.Image
	var format string

	if header[0] == 0xFF && header[1] == 0xD8 {
		img, err = jpeg.Decode(file)
		format = "jpeg"
	} else if header[0] == 0x89 && header[1] == 0x50 {
		img, err = png.Decode(file)
		format = "png"
	} else if (header[0] == 0x49 && header[1] == 0x49) || (header[0] == 0x4D && header[1] == 0x4D) {
		img, err = tiff.Decode(file)
		format = "tiff"
	} else {
		return nil, "", errors.New("unsupported image format")
	}

	if err != nil {
		return nil, "", err
	}

	return img, format, nil
}

func GenerateTiles(imagePath string, projectID uint64) (string, error) {
	img, _, err := LoadImage(imagePath)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	tileSize := 256
	tileOverlap := 0

	maxDim := max(width, height)
	numLevels := 1
	for maxDim > tileSize {
		numLevels++
		maxDim = (maxDim + 1) / 2
	}

	baseName := strings.TrimSuffix(filepath.Base(imagePath), filepath.Ext(imagePath))
	tileDir := fmt.Sprintf("%s/%d/%s", config.AppConfig.TILE_DIR, projectID, baseName)
	if err := os.MkdirAll(tileDir, 0755); err != nil {
		return "", err
	}

	for level := 0; level < numLevels; level++ {
		levelScale := 1.0 / float64(int(1)<<(numLevels-1-level))
		levelWidth := max(1, int(float64(width)*levelScale))
		levelHeight := max(1, int(float64(height)*levelScale))

		levelDir := fmt.Sprintf("%s/%d", tileDir, level)
		os.MkdirAll(levelDir, 0755)

		scaledImg := resizeImage(img, levelWidth, levelHeight)

		tilesX := (levelWidth + tileSize - 1) / tileSize
		tilesY := (levelHeight + tileSize - 1) / tileSize

		for ty := 0; ty < tilesY; ty++ {
			for tx := 0; tx < tilesX; tx++ {
				x0 := tx * tileSize
				y0 := ty * tileSize
				x1 := min(x0+tileSize+tileOverlap, levelWidth)
				y1 := min(y0+tileSize+tileOverlap, levelHeight)

				tileBounds := image.Rect(x0, y0, x1, y1)
				tileImg := scaledImg.(interface {
					SubImage(image.Rectangle) image.Image
				}).SubImage(tileBounds)

				tilePath := fmt.Sprintf("%s/%d_%d.jpg", levelDir, tx, ty)
				saveJPEG(tilePath, tileImg, 85)
			}
		}
	}

	dzPath := fmt.Sprintf("%s/%d/%s.dzi", config.AppConfig.TILE_DIR, projectID, baseName)
	relativeDziPath := fmt.Sprintf("%d/%s", projectID, baseName)
	generateDZI(dzPath, width, height, tileSize, tileOverlap, "jpg")

	return relativeDziPath, nil
}

func saveJPEG(path string, img image.Image, quality int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return jpeg.Encode(file, img, &jpeg.Options{Quality: quality})
}

func generateDZI(path string, width, height, tileSize, overlap int, format string) {
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Image xmlns="http://schemas.microsoft.com/deepzoom/2008"
  Format="%s"
  Overlap="%d"
  TileSize="%d">
  <Size Height="%d" Width="%d"/>
</Image>`, format, overlap, tileSize, height, width)
	os.WriteFile(path, []byte(xml), 0644)
}

func resizeImage(img image.Image, newWidth, newHeight int) image.Image {
	srcBounds := img.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) * float64(srcWidth) / float64(newWidth))
			srcY := int(float64(y) * float64(srcHeight) / float64(newHeight))
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}

	return dst
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func GetTilePath(projectID uint64, name string) string {
	return fmt.Sprintf("%s/%d/%s", config.AppConfig.TILE_DIR, projectID, name)
}

func GetImagePath(relativePath string) string {
	return filepath.Join(config.AppConfig.UPLOAD_DIR, relativePath)
}

func ReadTIFFMetadata(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	header := make([]byte, 8)
	_, err = file.Read(header)
	if err != nil {
		return 0, 0, err
	}

	var littleEndian bool
	if header[0] == 'I' && header[1] == 'I' {
		littleEndian = true
	} else if header[0] == 'M' && header[1] == 'M' {
		littleEndian = false
	} else {
		return 0, 0, errors.New("invalid TIFF header")
	}

	var ifdOffset uint32
	if littleEndian {
		ifdOffset = binary.LittleEndian.Uint32(header[4:8])
	} else {
		ifdOffset = binary.BigEndian.Uint32(header[4:8])
	}

	file.Seek(int64(ifdOffset), 0)

	var numEntries uint16
	entryBuf := make([]byte, 2)
	file.Read(entryBuf)
	if littleEndian {
		numEntries = binary.LittleEndian.Uint16(entryBuf)
	} else {
		numEntries = binary.BigEndian.Uint16(entryBuf)
	}

	var width, height int
	for i := 0; i < int(numEntries); i++ {
		entry := make([]byte, 12)
		file.Read(entry)

		var tag uint16
		var tagType uint16
		var count uint32
		var valueOffset uint32

		if littleEndian {
			tag = binary.LittleEndian.Uint16(entry[0:2])
			tagType = binary.LittleEndian.Uint16(entry[2:4])
			count = binary.LittleEndian.Uint32(entry[4:8])
			valueOffset = binary.LittleEndian.Uint32(entry[8:12])
		} else {
			tag = binary.BigEndian.Uint16(entry[0:2])
			tagType = binary.BigEndian.Uint16(entry[2:4])
			count = binary.BigEndian.Uint32(entry[4:8])
			valueOffset = binary.BigEndian.Uint32(entry[8:12])
		}

		if tag == 256 {
			if tagType == 3 && count == 1 {
				width = int(binary.LittleEndian.Uint16(entry[8:10]))
			} else {
				pos, _ := file.Seek(0, 1)
				file.Seek(int64(valueOffset), 0)
				valBuf := make([]byte, 4)
				file.Read(valBuf)
				if littleEndian {
					width = int(binary.LittleEndian.Uint32(valBuf))
				} else {
					width = int(binary.BigEndian.Uint32(valBuf))
				}
				file.Seek(pos, 0)
			}
		} else if tag == 257 {
			if tagType == 3 && count == 1 {
				height = int(binary.LittleEndian.Uint16(entry[8:10]))
			} else {
				pos, _ := file.Seek(0, 1)
				file.Seek(int64(valueOffset), 0)
				valBuf := make([]byte, 4)
				file.Read(valBuf)
				if littleEndian {
					height = int(binary.LittleEndian.Uint32(valBuf))
				} else {
					height = int(binary.BigEndian.Uint32(valBuf))
				}
				file.Seek(pos, 0)
			}
		}
	}

	return width, height, nil
}
