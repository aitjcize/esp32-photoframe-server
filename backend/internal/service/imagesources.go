package service

// imagesource.Source adapters around the synthetic image services. One
// plugin per source name — the registry stays flat. DB-backed sources live
// in their own *_source.go files alongside this one.

import (
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// ────────────────────────────────────────────────────────────────────────────
// AI Generation
// ────────────────────────────────────────────────────────────────────────────

type aiGenerationSource struct{ svc *AIGenerationService }

// NewAIGenerationSource wraps AIGenerationService as a registry plugin.
func NewAIGenerationSource(svc *AIGenerationService) imagesource.Source {
	return &aiGenerationSource{svc: svc}
}

func (s *aiGenerationSource) Name() string { return model.SourceAIGeneration }

func (s *aiGenerationSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	if req.Device == nil {
		return nil, &sourceError{msg: "ai_generation requires a configured device"}
	}
	img, err := s.svc.Generate(req.Device)
	if err != nil {
		return nil, err
	}
	// Photo-like output — goes through the dither/overlay pipeline.
	return &imagesource.Response{Image: img}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Fractal (Mandelbrot deep zoom)
// ────────────────────────────────────────────────────────────────────────────

type fractalSource struct{ svc *GenerativeService }

// NewFractalSource wraps GenerativeService's fractal generator as a plugin.
func NewFractalSource(svc *GenerativeService) imagesource.Source {
	return &fractalSource{svc: svc}
}

func (s *fractalSource) Name() string { return model.SourceFractal }

func (s *fractalSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	if req.Device == nil {
		return nil, &sourceError{msg: "fractal source requires a configured device"}
	}
	img, err := s.svc.Generate(req.Device.ID, model.SourceFractal, req.Width, req.Height)
	if err != nil {
		return nil, err
	}
	return &imagesource.Response{Image: img, SkipPostProcessing: true}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// DLA (diffusion-limited aggregation)
// ────────────────────────────────────────────────────────────────────────────

type dlaSource struct{ svc *GenerativeService }

// NewDLASource wraps GenerativeService's DLA generator as a plugin.
func NewDLASource(svc *GenerativeService) imagesource.Source {
	return &dlaSource{svc: svc}
}

func (s *dlaSource) Name() string { return model.SourceDLA }

func (s *dlaSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	if req.Device == nil {
		return nil, &sourceError{msg: "dla source requires a configured device"}
	}
	img, err := s.svc.Generate(req.Device.ID, model.SourceDLA, req.Width, req.Height)
	if err != nil {
		return nil, err
	}
	// The DLA palette is a handful of saturated colors that match the Spectra-6
	// inks natively, so color panels can ship it un-dithered. A grayscale (GC16)
	// panel cannot — those colors must flow through the dither-to-gray pipeline,
	// so don't skip post-processing for grayscale devices.
	skip := !(req.Device != nil && req.Device.IsGrayscale())
	return &imagesource.Response{Image: img, SkipPostProcessing: skip}, nil
}

// ────────────────────────────────────────────────────────────────────────────

type sourceError struct{ msg string }

func (e *sourceError) Error() string { return "imagesource: " + e.msg }
