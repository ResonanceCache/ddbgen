package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// createAppTable creates the demo table with the same shape ddbgen infra
// emits (examples/ecommerce/infra/): pk/sk plus GSI1, all string-typed.
// DynamoDB Local activates tables synchronously, but we poll briefly so
// the demo also works against real endpoints.
func createAppTable(ctx context.Context, ddb *dynamodb.Client, table string) error {
	attr := func(name string) types.AttributeDefinition {
		return types.AttributeDefinition{AttributeName: aws.String(name), AttributeType: types.ScalarAttributeTypeS}
	}
	_, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			attr("pk"), attr("sk"), attr("gsi1pk"), attr("gsi1sk"),
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("GSI1"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("gsi1pk"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("gsi1sk"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	if err != nil {
		return fmt.Errorf("creating table %s: %w", table, err)
	}
	for i := 0; i < 50; i++ {
		out, err := ddb.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
		if err != nil {
			return err
		}
		if out.Table.TableStatus == types.TableStatusActive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("table %s did not become active", table)
}
