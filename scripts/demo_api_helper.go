package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	openlane "github.com/theopenlane/go-client"
	"github.com/theopenlane/go-client/graphclient"
)

const (
	defaultMaxItems = 250
)

var (
	errTokenRequired = errors.New("token is required")
	errAPIURLMissing = errors.New("api url is required")
)

type ensureResult struct {
	StandardID      string `json:"standard_id"`
	StandardName    string `json:"standard_name"`
	StandardVersion string `json:"standard_version"`
	ControlID       string `json:"control_id"`
	ControlRef      string `json:"control_ref"`
}

type evidenceCandidate struct {
	EvidenceID   string   `json:"evidence_id"`
	EvidenceName string   `json:"evidence_name"`
	CreatedAt    string   `json:"created_at,omitempty"`
	ControlRefs  []string `json:"control_refs,omitempty"`
}

type verifyResult struct {
	Found        bool                `json:"found"`
	EvidenceID   string              `json:"evidence_id,omitempty"`
	EvidenceName string              `json:"evidence_name,omitempty"`
	CreatedAt    string              `json:"created_at,omitempty"`
	ControlRefs  []string            `json:"control_refs,omitempty"`
	Candidates   []evidenceCandidate `json:"candidates,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		exitWithError(fmt.Errorf("usage: %s <ensure|verify> [flags]", os.Args[0]))
	}

	switch os.Args[1] {
	case "ensure":
		runEnsure(os.Args[2:])
	case "verify":
		runVerify(os.Args[2:])
	default:
		exitWithError(fmt.Errorf("unknown mode %q (expected ensure or verify)", os.Args[1]))
	}
}

func runEnsure(args []string) {
	fs := flag.NewFlagSet("ensure", flag.ExitOnError)

	apiURL := fs.String("api-url", "", "Openlane API URL")
	token := fs.String("token", "", "Openlane API token")
	standardName := fs.String("standard-name", "", "Standard name")
	standardVersion := fs.String("standard-version", "", "Standard version")
	standardDescription := fs.String("standard-description", "", "Standard description")
	controlRef := fs.String("control-ref", "", "Control ref code")
	controlDescription := fs.String("control-description", "", "Control description")
	maxItems := fs.Int64("max-items", defaultMaxItems, "Maximum number of list items to request")

	_ = fs.Parse(args)

	client, err := newClient(*apiURL, *token)
	if err != nil {
		exitWithError(err)
	}

	ctx := context.Background()

	standardID, err := getOrCreateStandard(ctx, client, *standardName, *standardVersion, *standardDescription, *maxItems)
	if err != nil {
		exitWithError(err)
	}

	controlID, err := getOrCreateControl(ctx, client, standardID, *controlRef, *controlDescription, *maxItems)
	if err != nil {
		exitWithError(err)
	}

	writeJSON(ensureResult{
		StandardID:      standardID,
		StandardName:    *standardName,
		StandardVersion: *standardVersion,
		ControlID:       controlID,
		ControlRef:      *controlRef,
	})
}

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)

	apiURL := fs.String("api-url", "", "Openlane API URL")
	token := fs.String("token", "", "Openlane API token")
	checkName := fs.String("check-name", "", "Check name prefix")
	controlRef := fs.String("control-ref", "", "Control ref code to match")
	since := fs.String("since", "", "RFC3339 timestamp lower-bound")
	maxItems := fs.Int64("max-items", defaultMaxItems, "Maximum number of list items to request")

	_ = fs.Parse(args)

	if strings.TrimSpace(*since) == "" {
		exitWithError(fmt.Errorf("since is required"))
	}

	sinceTime, err := time.Parse(time.RFC3339, *since)
	if err != nil {
		exitWithError(fmt.Errorf("invalid since value %q: %w", *since, err))
	}

	client, err := newClient(*apiURL, *token)
	if err != nil {
		exitWithError(err)
	}

	result, err := verifyEvidence(context.Background(), client, *checkName, *controlRef, sinceTime, *maxItems)
	if err != nil {
		exitWithError(err)
	}

	writeJSON(result)

	if !result.Found {
		os.Exit(2)
	}
}

func newClient(apiURL, token string) (*openlane.Client, error) {
	apiURL = strings.TrimSpace(apiURL)
	token = strings.TrimSpace(token)

	if apiURL == "" {
		return nil, errAPIURLMissing
	}

	if token == "" {
		return nil, errTokenRequired
	}

	client, err := openlane.New(
		openlane.WithBaseURL(apiURL),
		openlane.WithAPIToken(token),
	)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return client, nil
}

func getOrCreateStandard(ctx context.Context, client *openlane.Client, name, version, description string, maxItems int64) (string, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	description = strings.TrimSpace(description)

	if name == "" {
		return "", fmt.Errorf("standard name is required")
	}

	listResp, err := client.GetAllStandards(ctx, &maxItems, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("list standards: %w", err)
	}

	for _, edge := range listResp.Standards.Edges {
		if edge == nil || edge.Node == nil {
			continue
		}

		if edge.Node.Name != name {
			continue
		}

		if strings.TrimSpace(deref(edge.Node.Version)) != version {
			continue
		}

		return edge.Node.ID, nil
	}

	input := graphclient.CreateStandardInput{
		Name: name,
	}

	if version != "" {
		input.Version = &version
	}

	if description != "" {
		input.Description = &description
	}

	framework := "agent-demo"
	input.Framework = &framework
	governingBody := "Openlane"
	input.GoverningBody = &governingBody

	createResp, err := client.CreateStandard(ctx, input, nil)
	if err != nil {
		return "", fmt.Errorf("create standard: %w", err)
	}

	return createResp.CreateStandard.Standard.ID, nil
}

func getOrCreateControl(ctx context.Context, client *openlane.Client, standardID, controlRef, description string, maxItems int64) (string, error) {
	standardID = strings.TrimSpace(standardID)
	controlRef = strings.TrimSpace(controlRef)
	description = strings.TrimSpace(description)

	if standardID == "" {
		return "", fmt.Errorf("standard id is required")
	}

	if controlRef == "" {
		return "", fmt.Errorf("control ref is required")
	}

	where := &graphclient.ControlWhereInput{
		RefCode:    &controlRef,
		StandardID: &standardID,
	}

	getResp, err := client.GetControls(ctx, &maxItems, nil, nil, nil, where, nil)
	if err != nil {
		return "", fmt.Errorf("list controls: %w", err)
	}

	for _, edge := range getResp.Controls.Edges {
		if edge == nil || edge.Node == nil {
			continue
		}

		return edge.Node.ID, nil
	}

	input := graphclient.CreateControlInput{
		RefCode:    controlRef,
		StandardID: &standardID,
	}

	if description != "" {
		input.Description = &description
	}

	createResp, err := client.CreateControl(ctx, input)
	if err != nil {
		return "", fmt.Errorf("create control: %w", err)
	}

	return createResp.CreateControl.Control.ID, nil
}

func verifyEvidence(ctx context.Context, client *openlane.Client, checkName, controlRef string, since time.Time, maxItems int64) (verifyResult, error) {
	checkName = strings.TrimSpace(checkName)
	controlRef = strings.TrimSpace(controlRef)

	if checkName == "" {
		return verifyResult{}, fmt.Errorf("check name is required")
	}

	if controlRef == "" {
		return verifyResult{}, fmt.Errorf("control ref is required")
	}

	resp, err := client.GetAllEvidences(ctx, &maxItems, nil, nil, nil, nil)
	if err != nil {
		return verifyResult{}, fmt.Errorf("list evidences: %w", err)
	}

	result := verifyResult{}

	var latest *evidenceCandidate
	for _, edge := range resp.Evidences.Edges {
		if edge == nil || edge.Node == nil {
			continue
		}

		node := edge.Node
		if !strings.HasPrefix(node.Name, checkName) {
			continue
		}

		candidate := evidenceCandidate{
			EvidenceID:   node.ID,
			EvidenceName: node.Name,
		}

		if node.CreatedAt != nil {
			candidate.CreatedAt = node.CreatedAt.UTC().Format(time.RFC3339)
		}

		for _, controlEdge := range node.Controls.Edges {
			if controlEdge == nil || controlEdge.Node == nil {
				continue
			}

			candidate.ControlRefs = append(candidate.ControlRefs, controlEdge.Node.RefCode)
		}

		result.Candidates = append(result.Candidates, candidate)

		if node.CreatedAt == nil || node.CreatedAt.Before(since) {
			continue
		}

		if !contains(candidate.ControlRefs, controlRef) {
			continue
		}

		if latest == nil {
			latest = &candidate
			continue
		}

		latestTime, err := time.Parse(time.RFC3339, latest.CreatedAt)
		if err != nil || node.CreatedAt.After(latestTime) {
			latest = &candidate
		}
	}

	if latest == nil {
		result.Found = false
		return result, nil
	}

	result.Found = true
	result.EvidenceID = latest.EvidenceID
	result.EvidenceName = latest.EvidenceName
	result.CreatedAt = latest.CreatedAt
	result.ControlRefs = latest.ControlRefs

	return result, nil
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}

	return false
}

func deref(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		exitWithError(fmt.Errorf("encode json output: %w", err))
	}
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
