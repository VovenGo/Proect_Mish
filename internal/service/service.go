package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vovengo/miha-shamanit/internal/config"
	"github.com/vovengo/miha-shamanit/internal/gen"
)

type App struct {
	cfg       config.Config
	generator gen.Generator
}

type CreateImageInput struct {
	Prompt       string
	SketchDataURL string
	Reference    []byte
	ReferenceName string
}

type CreateImageResult struct {
	ID            string `json:"id"`
	Prompt        string `json:"prompt"`
	Provider      string `json:"provider"`
	CreatedAt     time.Time `json:"createdAt"`
	SketchPath    string `json:"sketchPath,omitempty"`
	ReferencePath string `json:"referencePath,omitempty"`
	OutputPath    string `json:"outputPath"`
	OutputURL     string `json:"outputUrl"`
	FinalPrompt   string `json:"finalPrompt"`
}

func New(cfg config.Config, generator gen.Generator) *App {
	return &App{cfg: cfg, generator: generator}
}

func (a *App) CreateImage(ctx context.Context, in CreateImageInput) (CreateImageResult, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return CreateImageResult{}, errors.New("prompt is required")
	}
	if strings.TrimSpace(in.SketchDataURL) == "" && len(in.Reference) == 0 {
		return CreateImageResult{}, errors.New("sketch or reference is required")
	}

	id := time.Now().UTC().Format("20060102T150405.000000000")
	jobDir := filepath.Join(a.cfg.StorageDir, id)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return CreateImageResult{}, fmt.Errorf("create job dir: %w", err)
	}

	res := CreateImageResult{
		ID:        id,
		Prompt:    strings.TrimSpace(in.Prompt),
		Provider:  a.cfg.GeneratorProvider,
		CreatedAt: time.Now().UTC(),
	}

	var sourceImages []gen.SourceImage
	if strings.TrimSpace(in.SketchDataURL) != "" {
		sketchBytes, err := decodeDataURL(in.SketchDataURL)
		if err != nil {
			return CreateImageResult{}, fmt.Errorf("decode sketch: %w", err)
		}
		path := filepath.Join(jobDir, "sketch.png")
		if err := os.WriteFile(path, sketchBytes, 0o644); err != nil {
			return CreateImageResult{}, fmt.Errorf("write sketch: %w", err)
		}
		res.SketchPath = path
		sourceImages = append(sourceImages, gen.SourceImage{Name: "user-sketch.png", Bytes: sketchBytes, Purpose: "sketch"})
	}

	if len(in.Reference) > 0 {
		name := in.ReferenceName
		if name == "" {
			name = "reference.bin"
		}
		path := filepath.Join(jobDir, filepath.Base(name))
		if err := os.WriteFile(path, in.Reference, 0o644); err != nil {
			return CreateImageResult{}, fmt.Errorf("write reference: %w", err)
		}
		res.ReferencePath = path
		sourceImages = append(sourceImages, gen.SourceImage{Name: filepath.Base(name), Bytes: in.Reference, Purpose: "reference"})
	}

	finalPrompt := a.cfg.SystemPrompt + "\n\nUser prompt: " + res.Prompt
	genOut, err := a.generator.Generate(ctx, gen.Request{
		Prompt:       finalPrompt,
		SourceImages: sourceImages,
		OutputPath:   filepath.Join(jobDir, "result.png"),
	})
	if err != nil {
		return CreateImageResult{}, err
	}

	res.FinalPrompt = finalPrompt
	res.OutputPath = genOut.Path
	res.OutputURL = "/assets/" + id + "/" + filepath.Base(genOut.Path)
	return res, nil
}

func decodeDataURL(raw string) ([]byte, error) {
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid data url")
	}
	return base64.StdEncoding.DecodeString(parts[1])
}
