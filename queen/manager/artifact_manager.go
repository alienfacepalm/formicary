package manager

import (
	"context"
	"fmt"
	"github.com/twinj/uuid"
	"io"
	"io/ioutil"
	"os"
	"plexobject.com/formicary/internal/artifacts"
	"plexobject.com/formicary/internal/types"
	"plexobject.com/formicary/queen/config"
	"plexobject.com/formicary/queen/repository"
	"time"
)

// ArtifactManager  for managing artifacts
type ArtifactManager struct {
	serverCfg          *config.ServerConfig
	artifactRepository repository.ArtifactRepository
	artifactService    artifacts.Service
}

// NewArtifactManager manages artifacts
func NewArtifactManager(
	serverCfg *config.ServerConfig,
	artifactRepository repository.ArtifactRepository,
	artifactService artifacts.Service) (*ArtifactManager, error) {
	if artifactRepository == nil {
		return nil, fmt.Errorf("artifact-repository is not specified")
	}
	if artifactService == nil {
		return nil, fmt.Errorf("artifact-service is not specified")
	}
	return &ArtifactManager{
		serverCfg:          serverCfg,
		artifactRepository: artifactRepository,
		artifactService:    artifactService,
	}, nil
}

// QueryArtifacts - queries artifact
func (am *ArtifactManager) QueryArtifacts(
	ctx context.Context,
	qc *types.QueryContext,
	params map[string]interface{},
	page int,
	pageSize int,
	order []string) (arts []*types.Artifact, total int64, err error) {
	records, total, err := am.artifactRepository.Query(qc, params, page, pageSize, order)
	if err != nil {
		return nil, 0, err
	}
	for _, art := range records {
		am.UpdateURL(ctx, art)
	}
	return records, total, nil
}

// UploadArtifact - artifacts
func (am *ArtifactManager) UploadArtifact(
	ctx context.Context,
	qc *types.QueryContext,
	body io.ReadCloser,
	params map[string]string) (*types.Artifact, error) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), uuid.NewV4().String())
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file due to %s", err.Error())
	}
	if body == nil {
		return nil, fmt.Errorf("artifact body is nil")
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	dst, err := os.Create(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file due to %s", err.Error())
	}
	_, err = io.Copy(dst, body)
	ioutil.NopCloser(dst)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file due to %s", err.Error())
	}

	artifact := &types.Artifact{
		Name:           uuid.NewV4().String(),
		Metadata:       params,
		Kind:           types.ArtifactKindUser,
		Tags:           map[string]string{},
		UserID:         qc.UserID,
		OrganizationID: qc.OrganizationID,
	}

	if err = am.artifactService.SaveFile(
		ctx,
		artifact.UserID,
		artifact,
		tmpFile.Name()); err != nil {
		return nil, fmt.Errorf("failed to upload file due to %s", err.Error())
	}

	if _, err = am.artifactRepository.Save(artifact); err != nil {
		return nil, fmt.Errorf("failed to save file due to %s", err.Error())
	}
	return artifact, nil
}

// DownloadArtifactBySHA256 - download artifact by sha256
func (am *ArtifactManager) DownloadArtifactBySHA256(
	ctx context.Context,
	qc *types.QueryContext,
	sha256 string) (io.ReadCloser, string, string, error) {
	art, err := am.artifactRepository.GetBySHA256(qc, sha256)
	if err != nil {
		return nil, "", "", err
	}
	reader, err := am.artifactService.Get(
		ctx,
		art.ID)
	return reader, art.Name, art.ContentType, err
}

// GetArtifact - finds artifact by id
func (am *ArtifactManager) GetArtifact(
	ctx context.Context,
	qc *types.QueryContext,
	id string) (*types.Artifact, error) {
	art, err := am.artifactRepository.Get(qc, id)
	if err != nil {
		return nil, err
	}
	am.UpdateURL(ctx, art)
	return art, nil
}

// UpdateArtifact - updates artifact
func (am *ArtifactManager) UpdateArtifact(
	ctx context.Context,
	qc *types.QueryContext,
	artifact *types.Artifact) (saved *types.Artifact, err error) {
	am.UpdateURL(ctx, artifact)
	return am.artifactRepository.Update(qc, artifact)
}

// DeleteArtifact - deletes artifact by id
func (am *ArtifactManager) DeleteArtifact(
	qc *types.QueryContext,
	id string) error {
	return am.artifactRepository.Delete(qc, id)
}

// UpdateURL - using presigned or external api
func (am *ArtifactManager) UpdateURL(
	ctx context.Context,
	art *types.Artifact) {
	if am.serverCfg.ExternalBaseURL == "" {
		if url, err := am.artifactService.PresignedGetURL(
			ctx,
			art.ID,
			art.Name,
			am.serverCfg.URLPresignedExpirationMinutes*time.Minute); err == nil {
			art.URL = url.String()
		}
	} else {
		art.URL = am.serverCfg.ExternalBaseURL + "/api/artifacts/" + art.SHA256 + "/download"
	}
}
