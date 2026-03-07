package mongodb

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"go.mongodb.org/mongo-driver/mongo"
)

// requireMongos verifies the client is connected to a mongos router.
// Returns diagnostics with an error if the connection is not mongos.
// BAL-012, CBAL-010
func requireMongos(ctx context.Context, client *mongo.Client) diag.Diagnostics {
	connType, err := DetectConnectionType(ctx, client)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to detect connection type: %w", err))
	}
	if connType != ConnTypeMongos {
		return diag.Errorf("this resource requires a mongos connection; connected to %s", connType)
	}
	return nil
}
