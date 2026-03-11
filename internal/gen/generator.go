package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"strings"

	"github.com/vovengo/miha-shamanit/internal/config"
)

type SourceImage struct {
	Name    string
	Bytes   []byte
	Purpose string
}

type Request struct {
	Prompt       string
	SourceImages []SourceImage
	OutputPath   string
}

type Result struct {
	Path string
}

type Generator interface {
	Generate(ctx context.Context, req Request) (Result, error)
}

func NewFromConfig(cfg config.Config) Generator {
	if cfg.GeneratorProvider == "openai" {
		return &OpenAICompatibleGenerator{cfg: cfg, client: http.DefaultClient}
	}
	return &MockGenerator{}
}

type MockGenerator struct{}

func (g *MockGenerator) Generate(_ context.Context, req Request) (Result, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 18, G: 19, B: 32, A: 255}}, image.Point{}, draw.Src)

	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			if (x+y)%37 == 0 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 153, A: 255})
			}
			if (x*3+y*5)%101 == 0 {
				img.Set(x, y, color.RGBA{R: 0, G: 255, B: 204, A: 255})
			}
		}
	}

	if len(req.SourceImages) > 0 {
		if src, _, err := image.Decode(bytes.NewReader(req.SourceImages[0].Bytes)); err == nil {
			target := image.Rect(128, 128, min(128+src.Bounds().Dx(), 896), min(128+src.Bounds().Dy(), 896))
			draw.Draw(img, target, src, src.Bounds().Min, draw.Over)
		}
	}

	f, err := os.Create(req.OutputPath)
	if err != nil {
		return Result{}, fmt.Errorf("create mock output: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return Result{}, fmt.Errorf("encode mock output: %w", err)
	}
	return Result{Path: req.OutputPath}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type OpenAICompatibleGenerator struct {
	cfg    config.Config
	client *http.Client
}

func (g *OpenAICompatibleGenerator) Generate(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(g.cfg.OpenAIAPIKey) == "" {
		return Result{}, fmt.Errorf("GEN_PROVIDER=openai but OPENAI_API_KEY is empty")
	}

	type inputImage struct {
		Type     string `json:"type"`
		ImageURL string `json:"image_url"`
	}
	type inputText struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type body struct {
		Model string `json:"model"`
		Size  string `json:"size,omitempty"`
		Input []any  `json:"input"`
	}

	input := []any{inputText{Type: "input_text", Text: req.Prompt}}
	for _, src := range req.SourceImages {
		mime := http.DetectContentType(src.Bytes)
		input = append(input, inputImage{Type: "input_image", ImageURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(src.Bytes)})
	}

	payload, err := json.Marshal(body{Model: g.cfg.OpenAIModel, Size: g.cfg.OpenAIImageSize, Input: input})
	if err != nil {
		return Result{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.cfg.OpenAIBaseURL, "/")+"/images", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+g.cfg.OpenAIAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("call image provider: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("image provider returned status %s", resp.Status)
	}

	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("decode response: %w", err)
	}
	if len(out.Data) == 0 {
		return Result{}, fmt.Errorf("image provider returned no data")
	}
	if out.Data[0].B64JSON == "" {
		return Result{}, fmt.Errorf("provider response missing b64_json; URL-only mode not implemented yet")
	}
	imgBytes, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
	if err != nil {
		return Result{}, fmt.Errorf("decode provider image: %w", err)
	}
	if err := os.WriteFile(req.OutputPath, imgBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write output: %w", err)
	}
	return Result{Path: req.OutputPath}, nil
}
