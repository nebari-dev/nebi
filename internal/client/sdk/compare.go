package sdk

// Owner: U4 (compare). Compare two versions of a project; either may be a
// server-only version (fetched through c.Remote).

import (
	"context"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// Compare diffs the requested packages, channels and platforms of base and
// target (internal/artifact does the diff).
func (c *Client) Compare(ctx context.Context, projectID uuid.UUID, base, target int) (*shared.CompareResult, error) {
	return nil, shared.NotImplemented("sdk.Compare")
}
