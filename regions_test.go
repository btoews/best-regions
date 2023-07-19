package regions

import (
	"context"
	"testing"

	"github.com/alecthomas/assert/v2"
)

var (
	deployedRegions = []string{"den", "ord", "iad"}
)

func init() {
	EnvFlyApp = "best-regions"
	EnvFlyRegion = "den"

	dns = &staticResolver{
		TXTs: map[string]any{
			"regions.best-regions.internal": deployedRegions,
		},
	}
}

func TestDeployedRegions(t *testing.T) {
	regions, err := DeployedRegions(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, deployedRegions, regions)
}
