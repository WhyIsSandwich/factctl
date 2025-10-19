package resolve

import (
	"context"
	"fmt"
	"io"
)

// Fetcher defines the interface for fetching mod content from a source
type Fetcher interface {
	// Fetch retrieves mod content from the source and writes it to w.
	// It should return the SHA256 hash of the content.
	Fetch(ctx context.Context, src *Source, w io.Writer) (string, error)
}

// ModInfo contains metadata about a mod
type ModInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Hash    string `json:"hash"`    // SHA256 hash of the mod file
	Source  string `json:"source"`  // Original source specification
}

// Resolver handles mod resolution from various sources
type Resolver struct {
	fetchers map[SourceType]Fetcher
}

// NewResolver creates a new Resolver with the default fetchers
func NewResolver() *Resolver {
	return &Resolver{
		fetchers: make(map[SourceType]Fetcher),
	}
}

// RegisterFetcher registers a fetcher for a specific source type
func (r *Resolver) RegisterFetcher(typ SourceType, f Fetcher) {
	r.fetchers[typ] = f
}

// Resolve fetches a mod from its source and returns its info and content
func (r *Resolver) Resolve(ctx context.Context, spec string, w io.Writer) (*ModInfo, error) {
	src, err := ParseSource(spec)
	if err != nil {
		return nil, err
	}

	fetcher, ok := r.fetchers[src.Type]
	if !ok {
		return nil, fmt.Errorf("no fetcher registered for source type %v", src.Type)
	}

	hash, err := fetcher.Fetch(ctx, src, w)
	if err != nil {
		return nil, err
	}

	// TODO: Extract mod info from info.json within the zip
	// For now, return basic info
	return &ModInfo{
		Hash:   hash,
		Source: spec,
	}, nil
}