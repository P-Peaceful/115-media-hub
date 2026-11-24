package strm

import (
	"context"

	domain "github.com/wenzhengcheng/115-media-hub/backend/internal/domain/strm"
	"github.com/wenzhengcheng/115-media-hub/backend/internal/service/provider115"
)

var (
	ErrNotFound     = domain.ErrNotFound
	ErrConflict     = domain.ErrConflict
	ErrInvalidQuery = domain.ErrInvalidQuery
)

type Artifact = domain.Artifact
type WriteIntent = domain.WriteIntent
type ListFilter = domain.ListFilter
type ArtifactPage = domain.ArtifactPage

type Detail struct {
	Artifact
	FileState string
	Content   *string
}

type Repository interface {
	StrmArtifactByTarget(context.Context, string) (Artifact, error)
	StrmIntentByTarget(context.Context, string) (WriteIntent, error)
	SaveStrmIntent(context.Context, WriteIntent) error
	CompleteStrmIntent(context.Context, WriteIntent) error
	DiscardStrmIntent(context.Context, string) error
	StrmIntentsByLibrary(context.Context, string) ([]WriteIntent, error)
	ListStrmArtifacts(context.Context, ListFilter) (ArtifactPage, error)
	StrmArtifact(context.Context, string) (Artifact, error)
}

type Provider interface {
	Files(context.Context, provider115.FileQuery, string) (provider115.FilePage, error)
	ResolvePlayback(ctx context.Context, accountID, sourceFileID, playbackCode, userAgent, requestID string) (string, error)
}
