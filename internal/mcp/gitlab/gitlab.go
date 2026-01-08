// Package gitlab implements an MCP server for GitLab integration.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/claude-orchestrator/internal/mcp"
)

// Config holds GitLab configuration.
type Config struct {
	// BaseURL is the GitLab instance URL (e.g., "https://gitlab.com")
	BaseURL string `json:"base_url" yaml:"base_url"`
	// Token is the personal access token or project token
	Token string `json:"token" yaml:"token"`
	// DefaultProject is the default project path (e.g., "group/project")
	DefaultProject string `json:"default_project" yaml:"default_project"`
}

// Client is a GitLab API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new GitLab client.
func NewClient(cfg Config) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Issue represents a GitLab issue.
type Issue struct {
	ID          int       `json:"id"`
	IID         int       `json:"iid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	State       string    `json:"state"`
	Labels      []string  `json:"labels"`
	Assignees   []User    `json:"assignees"`
	Author      User      `json:"author"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	WebURL      string    `json:"web_url"`
	Milestone   *Milestone `json:"milestone"`
}

// MergeRequest represents a GitLab merge request.
type MergeRequest struct {
	ID             int       `json:"id"`
	IID            int       `json:"iid"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	State          string    `json:"state"`
	SourceBranch   string    `json:"source_branch"`
	TargetBranch   string    `json:"target_branch"`
	Author         User      `json:"author"`
	Assignees      []User    `json:"assignees"`
	Reviewers      []User    `json:"reviewers"`
	Labels         []string  `json:"labels"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	WebURL         string    `json:"web_url"`
	MergeStatus    string    `json:"merge_status"`
	Draft          bool      `json:"draft"`
	HasConflicts   bool      `json:"has_conflicts"`
	ChangesCount   string    `json:"changes_count"`
}

// Pipeline represents a GitLab CI/CD pipeline.
type Pipeline struct {
	ID        int       `json:"id"`
	IID       int       `json:"iid"`
	Status    string    `json:"status"`
	Ref       string    `json:"ref"`
	SHA       string    `json:"sha"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WebURL    string    `json:"web_url"`
	User      User      `json:"user"`
}

// Job represents a GitLab CI/CD job.
type Job struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Stage      string    `json:"stage"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Duration   float64   `json:"duration"`
	WebURL     string    `json:"web_url"`
}

// User represents a GitLab user.
type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// Milestone represents a GitLab milestone.
type Milestone struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Note represents a comment/note.
type Note struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	Author    User      `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	System    bool      `json:"system"`
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	reqURL := fmt.Sprintf("%s/api/v4%s", c.baseURL, path)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitLab API error (status %d): %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// ListIssues lists issues in a project.
func (c *Client) ListIssues(ctx context.Context, project string, state string, labels []string) ([]Issue, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/issues?per_page=50", encodedProject)

	if state != "" {
		path += "&state=" + url.QueryEscape(state)
	}
	if len(labels) > 0 {
		path += "&labels=" + url.QueryEscape(strings.Join(labels, ","))
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var issues []Issue
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	return issues, nil
}

// GetIssue gets a specific issue.
func (c *Client) GetIssue(ctx context.Context, project string, issueIID int) (*Issue, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/issues/%d", encodedProject, issueIID)

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse issue: %w", err)
	}

	return &issue, nil
}

// CreateIssue creates a new issue.
func (c *Client) CreateIssue(ctx context.Context, project, title, description string, labels []string) (*Issue, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/issues", encodedProject)

	body := map[string]interface{}{
		"title":       title,
		"description": description,
	}
	if len(labels) > 0 {
		body["labels"] = strings.Join(labels, ",")
	}

	bodyJSON, _ := json.Marshal(body)
	data, err := c.doRequest(ctx, "POST", path, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse issue: %w", err)
	}

	return &issue, nil
}

// ListMergeRequests lists merge requests in a project.
func (c *Client) ListMergeRequests(ctx context.Context, project string, state string) ([]MergeRequest, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/merge_requests?per_page=50", encodedProject)

	if state != "" {
		path += "&state=" + url.QueryEscape(state)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var mrs []MergeRequest
	if err := json.Unmarshal(data, &mrs); err != nil {
		return nil, fmt.Errorf("failed to parse merge requests: %w", err)
	}

	return mrs, nil
}

// GetMergeRequest gets a specific merge request.
func (c *Client) GetMergeRequest(ctx context.Context, project string, mrIID int) (*MergeRequest, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d", encodedProject, mrIID)

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("failed to parse merge request: %w", err)
	}

	return &mr, nil
}

// CreateMergeRequest creates a new merge request.
func (c *Client) CreateMergeRequest(ctx context.Context, project, sourceBranch, targetBranch, title, description string) (*MergeRequest, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/merge_requests", encodedProject)

	body := map[string]interface{}{
		"source_branch": sourceBranch,
		"target_branch": targetBranch,
		"title":         title,
		"description":   description,
	}

	bodyJSON, _ := json.Marshal(body)
	data, err := c.doRequest(ctx, "POST", path, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}

	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("failed to parse merge request: %w", err)
	}

	return &mr, nil
}

// ListPipelines lists pipelines in a project.
func (c *Client) ListPipelines(ctx context.Context, project string, ref string, status string) ([]Pipeline, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipelines?per_page=20", encodedProject)

	if ref != "" {
		path += "&ref=" + url.QueryEscape(ref)
	}
	if status != "" {
		path += "&status=" + url.QueryEscape(status)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var pipelines []Pipeline
	if err := json.Unmarshal(data, &pipelines); err != nil {
		return nil, fmt.Errorf("failed to parse pipelines: %w", err)
	}

	return pipelines, nil
}

// GetPipeline gets a specific pipeline.
func (c *Client) GetPipeline(ctx context.Context, project string, pipelineID int) (*Pipeline, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipelines/%d", encodedProject, pipelineID)

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var pipeline Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	return &pipeline, nil
}

// ListPipelineJobs lists jobs in a pipeline.
func (c *Client) ListPipelineJobs(ctx context.Context, project string, pipelineID int) ([]Job, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipelines/%d/jobs", encodedProject, pipelineID)

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("failed to parse jobs: %w", err)
	}

	return jobs, nil
}

// TriggerPipeline triggers a new pipeline.
func (c *Client) TriggerPipeline(ctx context.Context, project, ref string, variables map[string]string) (*Pipeline, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipeline", encodedProject)

	body := map[string]interface{}{
		"ref": ref,
	}
	if len(variables) > 0 {
		vars := make([]map[string]string, 0, len(variables))
		for k, v := range variables {
			vars = append(vars, map[string]string{"key": k, "value": v})
		}
		body["variables"] = vars
	}

	bodyJSON, _ := json.Marshal(body)
	data, err := c.doRequest(ctx, "POST", path, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}

	var pipeline Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	return &pipeline, nil
}

// RetryPipeline retries a failed pipeline.
func (c *Client) RetryPipeline(ctx context.Context, project string, pipelineID int) (*Pipeline, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipelines/%d/retry", encodedProject, pipelineID)

	data, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var pipeline Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	return &pipeline, nil
}

// CancelPipeline cancels a running pipeline.
func (c *Client) CancelPipeline(ctx context.Context, project string, pipelineID int) (*Pipeline, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/pipelines/%d/cancel", encodedProject, pipelineID)

	data, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}

	var pipeline Pipeline
	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline: %w", err)
	}

	return &pipeline, nil
}

// GetMRNotes gets notes/comments on a merge request.
func (c *Client) GetMRNotes(ctx context.Context, project string, mrIID int) ([]Note, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes?per_page=100", encodedProject, mrIID)

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var notes []Note
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, fmt.Errorf("failed to parse notes: %w", err)
	}

	return notes, nil
}

// AddMRNote adds a comment to a merge request.
func (c *Client) AddMRNote(ctx context.Context, project string, mrIID int, body string) (*Note, error) {
	encodedProject := url.PathEscape(project)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes", encodedProject, mrIID)

	reqBody := map[string]string{"body": body}
	bodyJSON, _ := json.Marshal(reqBody)

	data, err := c.doRequest(ctx, "POST", path, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}

	var note Note
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("failed to parse note: %w", err)
	}

	return &note, nil
}

// NewMCPServer creates an MCP server with GitLab tools.
func NewMCPServer(cfg Config) *mcp.Server {
	server := mcp.NewServer("gitlab", "1.0.0")
	client := NewClient(cfg)
	defaultProject := cfg.DefaultProject

	// Helper to get project from params or use default
	getProject := func(params map[string]interface{}) string {
		if p, ok := params["project"].(string); ok && p != "" {
			return p
		}
		return defaultProject
	}

	// List Issues Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_list_issues",
		Description: "List issues in a GitLab project",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path (e.g., 'group/project'). Uses default if not specified."},
				"state":   {Type: "string", Description: "Filter by state", Enum: []string{"opened", "closed", "all"}},
				"labels":  {Type: "string", Description: "Comma-separated labels to filter by"},
			},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		state, _ := params["state"].(string)
		var labels []string
		if l, ok := params["labels"].(string); ok && l != "" {
			labels = strings.Split(l, ",")
		}

		issues, err := client.ListIssues(ctx, project, state, labels)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(issues)
	})

	// Get Issue Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_get_issue",
		Description: "Get details of a specific GitLab issue",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"iid":     {Type: "number", Description: "Issue IID (internal ID)"},
			},
			Required: []string{"iid"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		iid, _ := params["iid"].(float64)
		issue, err := client.GetIssue(ctx, project, int(iid))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(issue)
	})

	// Create Issue Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_create_issue",
		Description: "Create a new issue in a GitLab project",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":     {Type: "string", Description: "Project path"},
				"title":       {Type: "string", Description: "Issue title"},
				"description": {Type: "string", Description: "Issue description (markdown supported)"},
				"labels":      {Type: "string", Description: "Comma-separated labels"},
			},
			Required: []string{"title"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		title, _ := params["title"].(string)
		description, _ := params["description"].(string)
		var labels []string
		if l, ok := params["labels"].(string); ok && l != "" {
			labels = strings.Split(l, ",")
		}

		issue, err := client.CreateIssue(ctx, project, title, description, labels)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(issue)
	})

	// List Merge Requests Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_list_merge_requests",
		Description: "List merge requests in a GitLab project",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"state":   {Type: "string", Description: "Filter by state", Enum: []string{"opened", "closed", "merged", "all"}},
			},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		state, _ := params["state"].(string)
		mrs, err := client.ListMergeRequests(ctx, project, state)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(mrs)
	})

	// Get Merge Request Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_get_merge_request",
		Description: "Get details of a specific merge request",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"iid":     {Type: "number", Description: "Merge request IID"},
			},
			Required: []string{"iid"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		iid, _ := params["iid"].(float64)
		mr, err := client.GetMergeRequest(ctx, project, int(iid))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(mr)
	})

	// Create Merge Request Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_create_merge_request",
		Description: "Create a new merge request",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":       {Type: "string", Description: "Project path"},
				"source_branch": {Type: "string", Description: "Source branch name"},
				"target_branch": {Type: "string", Description: "Target branch name"},
				"title":         {Type: "string", Description: "MR title"},
				"description":   {Type: "string", Description: "MR description"},
			},
			Required: []string{"source_branch", "target_branch", "title"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		sourceBranch, _ := params["source_branch"].(string)
		targetBranch, _ := params["target_branch"].(string)
		title, _ := params["title"].(string)
		description, _ := params["description"].(string)

		mr, err := client.CreateMergeRequest(ctx, project, sourceBranch, targetBranch, title, description)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(mr)
	})

	// List Pipelines Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_list_pipelines",
		Description: "List CI/CD pipelines in a project",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"ref":     {Type: "string", Description: "Filter by branch/tag name"},
				"status":  {Type: "string", Description: "Filter by status", Enum: []string{"running", "pending", "success", "failed", "canceled", "skipped", "manual"}},
			},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		ref, _ := params["ref"].(string)
		status, _ := params["status"].(string)

		pipelines, err := client.ListPipelines(ctx, project, ref, status)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(pipelines)
	})

	// Get Pipeline Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_get_pipeline",
		Description: "Get details of a specific pipeline",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":     {Type: "string", Description: "Project path"},
				"pipeline_id": {Type: "number", Description: "Pipeline ID"},
			},
			Required: []string{"pipeline_id"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		pipelineID, _ := params["pipeline_id"].(float64)
		pipeline, err := client.GetPipeline(ctx, project, int(pipelineID))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(pipeline)
	})

	// List Pipeline Jobs Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_list_pipeline_jobs",
		Description: "List jobs in a pipeline",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":     {Type: "string", Description: "Project path"},
				"pipeline_id": {Type: "number", Description: "Pipeline ID"},
			},
			Required: []string{"pipeline_id"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		pipelineID, _ := params["pipeline_id"].(float64)
		jobs, err := client.ListPipelineJobs(ctx, project, int(pipelineID))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(jobs)
	})

	// Trigger Pipeline Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_trigger_pipeline",
		Description: "Trigger a new pipeline",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":   {Type: "string", Description: "Project path"},
				"ref":       {Type: "string", Description: "Branch or tag to run pipeline on"},
				"variables": {Type: "string", Description: "Pipeline variables as key=value pairs, comma-separated"},
			},
			Required: []string{"ref"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		ref, _ := params["ref"].(string)
		variables := make(map[string]string)
		if v, ok := params["variables"].(string); ok && v != "" {
			for _, pair := range strings.Split(v, ",") {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 {
					variables[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}

		pipeline, err := client.TriggerPipeline(ctx, project, ref, variables)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(pipeline)
	})

	// Retry Pipeline Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_retry_pipeline",
		Description: "Retry a failed pipeline",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":     {Type: "string", Description: "Project path"},
				"pipeline_id": {Type: "number", Description: "Pipeline ID to retry"},
			},
			Required: []string{"pipeline_id"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		pipelineID, _ := params["pipeline_id"].(float64)
		pipeline, err := client.RetryPipeline(ctx, project, int(pipelineID))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(pipeline)
	})

	// Cancel Pipeline Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_cancel_pipeline",
		Description: "Cancel a running pipeline",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project":     {Type: "string", Description: "Project path"},
				"pipeline_id": {Type: "number", Description: "Pipeline ID to cancel"},
			},
			Required: []string{"pipeline_id"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		pipelineID, _ := params["pipeline_id"].(float64)
		pipeline, err := client.CancelPipeline(ctx, project, int(pipelineID))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(pipeline)
	})

	// Add MR Comment Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_add_mr_comment",
		Description: "Add a comment to a merge request",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"iid":     {Type: "number", Description: "Merge request IID"},
				"body":    {Type: "string", Description: "Comment body (markdown supported)"},
			},
			Required: []string{"iid", "body"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		iid, _ := params["iid"].(float64)
		body, _ := params["body"].(string)

		note, err := client.AddMRNote(ctx, project, int(iid), body)
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(note)
	})

	// Get MR Comments Tool
	server.RegisterTool(mcp.Tool{
		Name:        "gitlab_get_mr_comments",
		Description: "Get comments on a merge request",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"project": {Type: "string", Description: "Project path"},
				"iid":     {Type: "number", Description: "Merge request IID"},
			},
			Required: []string{"iid"},
		},
	}, func(ctx context.Context, params map[string]interface{}) (*mcp.ToolResult, error) {
		project := getProject(params)
		if project == "" {
			return mcp.ErrorResult(fmt.Errorf("project is required")), nil
		}

		iid, _ := params["iid"].(float64)
		notes, err := client.GetMRNotes(ctx, project, int(iid))
		if err != nil {
			return mcp.ErrorResult(err), nil
		}

		return mcp.JSONResult(notes)
	})

	return server
}

// Helper to parse int from interface
func parseInt(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}
