// internal/infrastructure/textextraction/command_extractor.go
package textextraction

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	domaintext "github.com/gabrielgcmr/sonnda/internal/features/documentprocessing/textextraction"
)

type CommandExtractor struct {
	timeout           time.Duration
	requireUsableText bool
}

type CommandExtractorOptions struct {
	Timeout           time.Duration
	RequireUsableText bool
}

var _ domaintext.Extractor = (*CommandExtractor)(nil)

func NewCommandExtractor() *CommandExtractor {
	return &CommandExtractor{timeout: 20 * time.Second, requireUsableText: true}
}

func NewCommandExtractorWithOptions(options CommandExtractorOptions) *CommandExtractor {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &CommandExtractor{timeout: timeout, requireUsableText: options.RequireUsableText}
}

func (e *CommandExtractor) Extract(ctx context.Context, input domaintext.ExtractInput) (*domaintext.ExtractOutput, error) {
	if strings.TrimSpace(input.LocalPath) == "" {
		return nil, errors.New("local path is required")
	}

	switch normalizeMimeType(input.MimeType) {
	case "application/pdf", "image/pdf":
		return e.extractPDF(ctx, input.LocalPath)
	case "image/jpeg", "image/jpg", "image/png":
		return e.extractImage(ctx, input.LocalPath)
	default:
		return nil, fmt.Errorf("unsupported mime type: %s", input.MimeType)
	}
}

func (e *CommandExtractor) extractPDF(ctx context.Context, localPath string) (*domaintext.ExtractOutput, error) {
	text, err := e.run(ctx, "pdftotext", "-raw", localPath, "-")
	if err != nil {
		return nil, err
	}
	if e.requireUsableText && !domaintext.IsUsableText(text) {
		return nil, domaintext.ErrUnreadablePDF
	}

	return &domaintext.ExtractOutput{
		Text:           text,
		NormalizedText: domaintext.NormalizeForSemanticExtraction(text),
		Method:         "pdf_text_raw",
	}, nil
}

func (e *CommandExtractor) extractImage(ctx context.Context, localPath string) (*domaintext.ExtractOutput, error) {
	timeout := e.timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ocrCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	text, method, err := e.extractImageWithRotations(ocrCtx, localPath)
	if err != nil {
		return nil, err
	}
	if e.requireUsableText && !domaintext.IsUsableText(text) {
		if ocrCtx.Err() != nil {
			return nil, ocrCtx.Err()
		}
		return nil, errors.New("ocr text is not usable")
	}

	return &domaintext.ExtractOutput{
		Text:           text,
		NormalizedText: domaintext.NormalizeForSemanticExtraction(text),
		Method:         method,
	}, nil
}

func (e *CommandExtractor) extractImageWithRotations(ctx context.Context, localPath string) (string, string, error) {
	rotations, cleanup, err := imageRotationCandidates(localPath)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		rotations = []imageCandidate{{path: localPath, rotation: 0, variant: "original"}}
	}

	bestRotation, bestRotationText, bestRotationScore, lastErr := e.selectRotation(ctx, rotations)
	if bestRotationText == "" {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		if lastErr != nil {
			return "", "", lastErr
		}
		return "", "", errors.New("ocr text is empty")
	}

	bestText := bestRotationText
	bestCandidate := bestRotation
	bestPSM := 3
	bestScore := bestRotationScore

	// A primeira passagem encontra a orientacao. As demais variacoes usam apenas
	// essa orientacao para manter o custo do OCR limitado.
	candidates := []imageCandidate{bestRotation}
	preprocessed, preprocessCleanup, preprocessErr := imagePreprocessingCandidates(bestRotation.path)
	if preprocessCleanup != nil {
		defer preprocessCleanup()
	}
	if preprocessErr == nil {
		for index := range preprocessed {
			preprocessed[index].rotation = bestRotation.rotation
		}
		candidates = append(candidates, preprocessed...)
	}

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		psms := []int{6}
		if candidate.variant == "original" {
			// O PSM 3 ja foi usado para selecionar a orientacao.
			psms = []int{4, 6, 11}
		}
		for _, psm := range psms {
			text, err := e.runTesseract(ctx, candidate.path, psm)
			if err != nil {
				lastErr = err
				continue
			}
			score := scoreOCRText(text)
			if score > bestScore {
				bestText = text
				bestCandidate = candidate
				bestPSM = psm
				bestScore = score
			}
		}
	}

	method := ocrMethod(bestCandidate.rotation, bestCandidate.variant, bestPSM)
	if ctx.Err() != nil {
		method += "_partial"
	}
	return bestText, method, nil
}

func (e *CommandExtractor) selectBestRotation(ctx context.Context, candidates []imageCandidate) (imageCandidate, string, int, error) {
	var best imageCandidate
	var bestText string
	var bestScore int
	var lastErr error
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		text, err := e.runTesseract(ctx, candidate.path, 3)
		if err != nil {
			lastErr = err
			continue
		}
		score := scoreOCRText(text)
		if score > bestScore || bestText == "" {
			bestText = text
			best = candidate
			bestScore = score
		}
	}
	return best, bestText, bestScore, lastErr
}

func (e *CommandExtractor) selectRotation(ctx context.Context, candidates []imageCandidate) (imageCandidate, string, int, error) {
	if len(candidates) == 0 {
		return imageCandidate{}, "", 0, errors.New("image candidates are empty")
	}

	rotation, err := e.detectRotation(ctx, candidates[0].path)
	if err == nil {
		for _, candidate := range candidates {
			if candidate.rotation != rotation {
				continue
			}
			text, textErr := e.runTesseract(ctx, candidate.path, 3)
			if textErr == nil && scoreOCRText(text) >= 200 {
				return candidate, text, scoreOCRText(text), nil
			}
			break
		}
	}

	return e.selectBestRotation(ctx, candidates)
}

var rotationPattern = regexp.MustCompile(`(?mi)^Rotate:\s*(0|90|180|270)\s*$`)

func (e *CommandExtractor) detectRotation(ctx context.Context, localPath string) (int, error) {
	output, err := e.run(ctx, "tesseract", localPath, "stdout", "-l", "osd", "--psm", "0")
	if err != nil {
		return 0, err
	}
	matches := rotationPattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return 0, errors.New("tesseract did not return an image rotation")
	}
	return strconv.Atoi(matches[1])
}

func (e *CommandExtractor) runTesseract(ctx context.Context, localPath string, psm int) (string, error) {
	psmValue := strconv.Itoa(psm)
	text, err := e.run(ctx, "tesseract", localPath, "stdout", "-l", "por+eng", "--psm", psmValue)
	if err != nil {
		// Alguns ambientes nao possuem os idiomas instalados.
		text, err = e.run(ctx, "tesseract", localPath, "stdout", "--psm", psmValue)
	}
	return text, err
}

type imageCandidate struct {
	path     string
	rotation int
	variant  string
}

func imageRotationCandidates(localPath string) ([]imageCandidate, func(), error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, nil, err
	}

	tempDir, err := os.MkdirTemp("", "sonnda-ocr-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	normalized := resizeImageToMaxDimension(img, 2200)
	candidates := make([]imageCandidate, 0, 4)
	for _, rotation := range []int{0, 90, 180, 270} {
		targetPath := filepath.Join(tempDir, fmt.Sprintf("rotated-%d.jpg", rotation))
		if err := writeJPEG(targetPath, rotateImage(normalized, rotation)); err != nil {
			cleanup()
			return nil, nil, err
		}
		candidates = append(candidates, imageCandidate{path: targetPath, rotation: rotation, variant: "original"})
	}
	return candidates, cleanup, nil
}

func imagePreprocessingCandidates(localPath string) ([]imageCandidate, func(), error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, nil, err
	}

	tempDir, err := os.MkdirTemp("", "sonnda-ocr-preprocessed-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	enhanced := enhanceForOCR(img)
	variants := []struct {
		name string
		img  image.Image
	}{
		{name: "enhanced", img: enhanced},
		{name: "binary", img: binarizeImage(enhanced)},
	}
	candidates := make([]imageCandidate, 0, len(variants))
	for _, variant := range variants {
		targetPath := filepath.Join(tempDir, variant.name+".jpg")
		if err := writeJPEG(targetPath, variant.img); err != nil {
			cleanup()
			return nil, nil, err
		}
		candidates = append(candidates, imageCandidate{path: targetPath, variant: variant.name})
	}
	return candidates, cleanup, nil
}

func enhanceForOCR(src image.Image) *image.Gray {
	gray := grayscaleImage(src)
	gray = scaleImageForOCR(gray)
	return stretchContrast(gray)
}

func grayscaleImage(src image.Image) *image.Gray {
	bounds := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := color.GrayModel.Convert(src.At(x, y)).(color.Gray)
			dst.SetGray(x-bounds.Min.X, y-bounds.Min.Y, gray)
		}
	}
	return dst
}

func scaleImageForOCR(src *image.Gray) *image.Gray {
	width := src.Bounds().Dx()
	height := src.Bounds().Dy()
	longest := max(width, height)
	if longest >= 2200 {
		return src
	}

	targetLongest := min(longest*2, 3200)
	targetWidth := max(1, width*targetLongest/longest)
	targetHeight := max(1, height*targetLongest/longest)
	dst := image.NewGray(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := y * height / targetHeight
		for x := 0; x < targetWidth; x++ {
			sourceX := x * width / targetWidth
			dst.SetGray(x, y, src.GrayAt(sourceX, sourceY))
		}
	}
	return dst
}

func resizeImageToMaxDimension(src image.Image, maximum int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	longest := max(width, height)
	if longest <= maximum {
		return src
	}

	targetWidth := max(1, width*maximum/longest)
	targetHeight := max(1, height*maximum/longest)
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + y*height/targetHeight
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + x*width/targetWidth
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}
	return dst
}

func stretchContrast(src *image.Gray) *image.Gray {
	var histogram [256]int
	for _, value := range src.Pix {
		histogram[value]++
	}

	low := histogramPercentile(histogram, len(src.Pix)/100)
	high := histogramPercentile(histogram, len(src.Pix)*99/100)
	if high-low < 20 {
		return src
	}

	dst := image.NewGray(src.Bounds())
	for index, value := range src.Pix {
		adjusted := (int(value) - low) * 255 / (high - low)
		dst.Pix[index] = uint8(min(255, max(0, adjusted)))
	}
	return dst
}

func binarizeImage(src *image.Gray) *image.Gray {
	var histogram [256]int
	for _, value := range src.Pix {
		histogram[value]++
	}
	threshold := otsuThreshold(histogram, len(src.Pix))
	dst := image.NewGray(src.Bounds())
	for index, value := range src.Pix {
		if int(value) > threshold {
			dst.Pix[index] = 255
		}
	}
	return dst
}

func histogramPercentile(histogram [256]int, target int) int {
	count := 0
	for value, occurrences := range histogram {
		count += occurrences
		if count >= target {
			return value
		}
	}
	return 255
}

func otsuThreshold(histogram [256]int, total int) int {
	if total == 0 {
		return 128
	}

	var sum int
	for value, count := range histogram {
		sum += value * count
	}

	var sumBackground, backgroundCount int
	bestThreshold, bestVariance := 0, -1.0
	for threshold, count := range histogram {
		backgroundCount += count
		if backgroundCount == 0 {
			continue
		}
		foregroundCount := total - backgroundCount
		if foregroundCount == 0 {
			break
		}
		sumBackground += threshold * count
		backgroundMean := float64(sumBackground) / float64(backgroundCount)
		foregroundMean := float64(sum-sumBackground) / float64(foregroundCount)
		variance := float64(backgroundCount*foregroundCount) * (backgroundMean - foregroundMean) * (backgroundMean - foregroundMean)
		if variance > bestVariance {
			bestVariance = variance
			bestThreshold = threshold
		}
	}
	return bestThreshold
}

func ocrMethod(rotation int, variant string, psm int) string {
	parts := []string{"ocr"}
	if rotation != 0 {
		parts = append(parts, fmt.Sprintf("rotate_%d", rotation))
	}
	if variant != "" && variant != "original" {
		parts = append(parts, variant)
	}
	parts = append(parts, fmt.Sprintf("psm_%d", psm))
	return strings.Join(parts, "_")
}

func writeJPEG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return jpeg.Encode(file, img, &jpeg.Options{Quality: 92})
}

func rotateImage(src image.Image, degrees int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	switch degrees {
	case 90:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(height-1-y, x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return dst
	case 180:
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(width-1-x, height-1-y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return dst
	case 270:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(y, width-1-x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return dst
	default:
		return src
	}
}

var labPanelLinePattern = regexp.MustCompile(`(?im)(?:hemacias|hemoglobina|hematocrito|v\.?g\.?m|h\.?g\.?m|c\.?h\.?g\.?m|leucocitos|plaquetas)(?:[.:]{2,}\s*:?[\s]*|:\s*)\d`)

func scoreOCRText(text string) int {
	normalized := strings.ToLower(text)
	score := len(strings.Fields(normalized))
	for _, signal := range []string{
		"hemograma", "eritrograma", "leucograma", "plaquetas",
		"paciente", "material", "metodo", "laboratorio", "resultado",
		"hemoglobina", "hematocrito", "leucocitos",
	} {
		if strings.Contains(normalized, signal) {
			score += 80
		}
	}
	// Linhas completas de analito e valor sao mais uteis que texto longo com ruido.
	score += len(labPanelLinePattern.FindAllStringIndex(normalized, -1)) * 120
	for _, r := range normalized {
		switch {
		case r >= '0' && r <= '9':
			score += 1
		case r == '�':
			score -= 10
		}
	}
	return score
}

func (e *CommandExtractor) run(ctx context.Context, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}

	timeout := e.timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	output, err := cmd.Output()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func normalizeMimeType(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(normalized, ";"); idx >= 0 {
		normalized = strings.TrimSpace(normalized[:idx])
	}
	return normalized
}
