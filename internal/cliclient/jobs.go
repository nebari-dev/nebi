package cliclient

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListJobs returns all jobs for the authenticated user.
func (c *Client) ListJobs(ctx context.Context) ([]Job, error) {
	var jobs []Job
	_, err := c.Get(ctx, "/jobs", &jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// GetJob returns a single job by ID.
func (c *Client) GetJob(ctx context.Context, id string) (*Job, error) {
	var job Job
	_, err := c.Get(ctx, fmt.Sprintf("/jobs/%s", id), &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// StreamJobLogs consumes the server's SSE log stream for a job and writes
// each log line to w until the job finishes. It blocks for the lifetime
// of the job; cancel ctx to stop early.
func (c *Client) StreamJobLogs(ctx context.Context, jobID string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/jobs/"+jobID+"/logs/stream", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	// A dedicated client without a timeout: the stream stays open for as
	// long as the job runs.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			// Blank line terminates an SSE frame.
			event = ""
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			if event == "done" {
				return nil
			}
		case strings.HasPrefix(line, "data: "):
			data := strings.TrimPrefix(line, "data: ")
			if event == "error" {
				return fmt.Errorf("log stream error: %s", data)
			}
			if event == "" {
				fmt.Fprintln(w, data)
			}
		default:
			// Continuation of a multi-line data payload (the server emits
			// historical logs as one frame with embedded newlines).
			if event == "" {
				fmt.Fprintln(w, line)
			}
		}
	}
	return scanner.Err()
}
