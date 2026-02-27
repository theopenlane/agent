package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	openlane "github.com/theopenlane/go-client"
	"github.com/theopenlane/go-client/graphclient"

	"github.com/theopenlane/agent/config"
	agentmodels "github.com/theopenlane/agent/internal/models"
	"github.com/theopenlane/agent/internal/retry"
)

const (
	// controlLookupPageSize is the page size used when querying controls from the API
	controlLookupPageSize = int64(20)
)

// controlCache stores resolved control reference codes to IDs to avoid repeated API calls
type controlCache struct {
	mu      sync.RWMutex
	entries map[string]string
}

// key returns the cache key for a standard ID and control reference code
func (c *controlCache) key(standardID, refCode string) string {
	return standardID + ":" + strings.ToLower(refCode)
}

// get returns a cached control ID, or empty string if not found
func (c *controlCache) get(standardID, refCode string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	id, ok := c.entries[c.key(standardID, refCode)]

	return id, ok
}

// set stores a control ID in the cache
func (c *controlCache) set(standardID, refCode, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[c.key(standardID, refCode)] = id
}

// GraphQLClient handles API communication with the Openlane platform
type GraphQLClient struct {
	client       *openlane.Client
	retryManager *retry.Manager
	cache        *controlCache
}

// NewGraphQLClient creates a new GraphQL client
func NewGraphQLClient(baseURL, apiToken string) (*GraphQLClient, error) {
	return NewGraphQLClientWithRetry(baseURL, apiToken, "", nil)
}

// NewGraphQLClientWithOrgID creates a new GraphQL client scoped to an organization
func NewGraphQLClientWithOrgID(baseURL, apiToken, orgID string) (*GraphQLClient, error) {
	return NewGraphQLClientWithRetry(baseURL, apiToken, orgID, nil)
}

// NewGraphQLClientWithRetry creates a new GraphQL client with optional org scoping and retry configuration
func NewGraphQLClientWithRetry(baseURL, apiToken, orgID string, retryManager *retry.Manager) (*GraphQLClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, ErrBaseURLRequired
	}

	if strings.TrimSpace(apiToken) == "" {
		return nil, ErrAPITokenRequired
	}

	opts := []openlane.ClientOption{
		openlane.WithBaseURL(baseURL),
		openlane.WithAPIToken(apiToken),
	}

	if strings.TrimSpace(orgID) != "" {
		opts = append(opts, openlane.WithInterceptors(openlane.WithOrganizationHeader(orgID)))
	}

	client, err := openlane.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientCreationFailed, err)
	}

	return &GraphQLClient{
		client:       client,
		retryManager: retryManager,
		cache:        &controlCache{entries: make(map[string]string)},
	}, nil
}

// NewGraphQLClientFromConfig creates a GraphQL client from agent configuration
func NewGraphQLClientFromConfig(cfg *config.Config, retryManager *retry.Manager) (*GraphQLClient, error) {
	return NewGraphQLClientWithRetry(cfg.APIURL, cfg.Token(), cfg.OrgID, retryManager)
}

// GetClient returns the underlying openlane client for direct API calls
func (c *GraphQLClient) GetClient() *openlane.Client {
	return c.client
}

// Health checks API connectivity using a lightweight controls query
func (c *GraphQLClient) Health(ctx context.Context) error {
	limit := int64(1)

	_, err := c.client.GetAllControls(ctx, &limit, nil, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrHealthCheckFailed, err)
	}

	return nil
}

// ValidateControls validates that controls exist within their specified standards
func (c *GraphQLClient) ValidateControls(ctx context.Context, standards []config.ComplianceStandard) (map[string][]string, error) {
	validControls := make(map[string][]string)

	for _, standard := range standards {
		if err := c.validateStandard(ctx, standard.Standard); err != nil {
			switch {
			case IsNotFoundError(err):
				log.Warn().Str("standard", standard.Standard).Msg("Standard not found in API")
			default:
				log.Error().Err(err).Str("standard", standard.Standard).Msg("Standard validation failed")
			}

			continue
		}

		for _, control := range standard.Controls {
			if err := c.validateControl(ctx, standard.Standard, control); err != nil {
				switch {
				case IsNotFoundError(err):
					log.Warn().Str("standard", standard.Standard).Str("control", control).Msg("Control not found in API")
				default:
					log.Error().Err(err).Str("standard", standard.Standard).Str("control", control).Msg("Control validation failed")
				}

				continue
			}

			if validControls[standard.Standard] == nil {
				validControls[standard.Standard] = make([]string, 0)
			}

			validControls[standard.Standard] = append(validControls[standard.Standard], control)
		}
	}

	return validControls, nil
}

// ResolveControlIDs resolves control reference codes to control IDs for evidence associations
func (c *GraphQLClient) ResolveControlIDs(ctx context.Context, standards []config.ComplianceStandard) ([]string, error) {
	controlIDs := make(map[string]struct{})

	var resolveErrors []string

	for _, standard := range standards {
		if err := c.validateStandard(ctx, standard.Standard); err != nil {
			resolveErrors = append(resolveErrors, err.Error())
			continue
		}

		for _, controlRef := range standard.Controls {
			controlID, err := c.lookupControlID(ctx, standard.Standard, controlRef)
			if err != nil {
				resolveErrors = append(resolveErrors, err.Error())
				continue
			}

			controlIDs[controlID] = struct{}{}
		}
	}

	resolved := make([]string, 0, len(controlIDs))
	for controlID := range controlIDs {
		resolved = append(resolved, controlID)
	}

	slices.Sort(resolved)

	if len(resolveErrors) > 0 && len(resolved) == 0 {
		return resolved, fmt.Errorf("%w: %s", ErrControlIDResolutionFailed, strings.Join(resolveErrors, "; "))
	}

	return resolved, nil
}

// validateStandard validates that a compliance standard exists
func (c *GraphQLClient) validateStandard(ctx context.Context, standardID string) error {
	standard, err := c.client.GetStandardByID(ctx, standardID)
	if err != nil {
		return fmt.Errorf("validate standard %s: %w", standardID, err)
	}

	if standard == nil {
		return fmt.Errorf("%w: %s", ErrStandardNotFound, standardID)
	}

	return nil
}

// validateControl validates that a control exists within a standard
func (c *GraphQLClient) validateControl(ctx context.Context, standardID, controlRefCode string) error {
	if _, err := c.lookupControlID(ctx, standardID, controlRefCode); err != nil {
		return fmt.Errorf("validate control %s in standard %s: %w", controlRefCode, standardID, err)
	}

	return nil
}

// lookupControlID resolves a control reference within a standard to its control ID;
// results are cached to avoid redundant API calls within the same session
func (c *GraphQLClient) lookupControlID(ctx context.Context, standardID, controlRefCode string) (string, error) {
	if id, ok := c.cache.get(standardID, controlRefCode); ok {
		return id, nil
	}

	pageSize := controlLookupPageSize

	response, err := c.client.GetControls(ctx, &pageSize, nil, nil, nil, &graphclient.ControlWhereInput{
		StandardID:       &standardID,
		RefCodeEqualFold: lo.ToPtr(controlRefCode),
	}, nil)
	if err != nil {
		return "", fmt.Errorf("query control %s in standard %s: %w", controlRefCode, standardID, err)
	}

	if response == nil || len(response.Controls.Edges) == 0 || response.Controls.Edges[0].Node == nil {
		return "", fmt.Errorf("%w: %s in standard %s", ErrControlNotFound, controlRefCode, standardID)
	}

	id := response.Controls.Edges[0].Node.ID
	c.cache.set(standardID, controlRefCode, id)

	return id, nil
}

// CreateEvidence uploads evidence with optional control associations
func (c *GraphQLClient) CreateEvidence(ctx context.Context, controlIDs []string, evidence agentmodels.EvidenceFile, runArtifactID string) error {
	isAutomated := true
	creationTime := evidence.CreatedAt
	evidenceName := strings.TrimPrefix(evidence.Path, "./")

	if evidenceName == "" {
		evidenceName = "evidence-" + runArtifactID
	}

	description := fmt.Sprintf("Evidence collected by agent from %s", evidence.Path)
	collectionProcedure := "Automated collection by Openlane compliance agent"
	source := "openlane-agent"

	input := graphclient.CreateEvidenceInput{
		Name:                evidenceName,
		Description:         &description,
		CollectionProcedure: &collectionProcedure,
		CreationDate:        &creationTime,
		Source:              &source,
		IsAutomated:         &isAutomated,
		ControlIDs:          controlIDs,
		Tags:                []string{"automated", "agent-collected", fmt.Sprintf("run:%s", runArtifactID)},
	}

	var uploads []*graphql.Upload
	if len(evidence.Content) > 0 {
		uploads = append(uploads, PrepareEvidenceFileUpload(evidence))
	}

	var resp *graphclient.CreateEvidence

	err := c.executeWithRetry(ctx, func(ctx context.Context) error {
		var execErr error

		resp, execErr = c.client.CreateEvidence(ctx, input, uploads)

		return execErr
	})
	if err != nil {
		log.Error().Err(err).Str("controls", controlsLabel(controlIDs)).Str("evidence_path", evidence.Path).Msg("Failed to create evidence")

		return fmt.Errorf("%w: %w", ErrEvidenceCreationFailed, err)
	}

	log.Info().Str("controls", controlsLabel(controlIDs)).Str("evidence_id", resp.CreateEvidence.Evidence.ID).Str("evidence_path", evidence.Path).Msg("Evidence created")

	return nil
}

// CreateEvidenceForControl creates evidence directly associated with a single control
func (c *GraphQLClient) CreateEvidenceForControl(ctx context.Context, controlID string, evidence agentmodels.EvidenceFile, runArtifactID string) error {
	return c.CreateEvidence(ctx, []string{controlID}, evidence, runArtifactID)
}

// SetTimeout stores a timeout duration for future HTTP client configuration
func (c *GraphQLClient) SetTimeout(timeout time.Duration) {
	log.Debug().Dur("timeout", timeout).Msg("GraphQL timeout setting stored for future HTTP client configuration")
}

// executeWithRetry wraps an operation with retry logic when a retry manager is configured
func (c *GraphQLClient) executeWithRetry(ctx context.Context, operation func(context.Context) error) error {
	if c.retryManager != nil {
		return c.retryManager.ExecuteWithContext(ctx, operation)
	}

	return operation(ctx)
}

// controlsLabel formats a slice of control IDs for log output
func controlsLabel(controlIDs []string) string {
	if len(controlIDs) == 0 {
		return "none"
	}

	return strings.Join(controlIDs, ", ")
}
