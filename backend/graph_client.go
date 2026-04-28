package main

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GraphClient wraps Neo4j driver for graph operations
type GraphClient struct {
	driver neo4j.DriverWithContext
}

// NewGraphClient creates a new Neo4j graph client
func NewGraphClient(uri, username, password string) (*GraphClient, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}

	// Test connection
	ctx := context.Background()
	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("verify connectivity: %w", err)
	}

	return &GraphClient{driver: driver}, nil
}

// Ping verifies the driver can reach Neo4j.
func (gc *GraphClient) Ping(ctx context.Context) error {
	return gc.driver.VerifyConnectivity(ctx)
}

// Close closes the Neo4j driver
func (gc *GraphClient) Close(ctx context.Context) error {
	return gc.driver.Close(ctx)
}

// ExecuteQuery executes a Cypher query and returns results
func (gc *GraphClient) ExecuteQuery(ctx context.Context, query string, params map[string]interface{}) ([]map[string]interface{}, error) {
	session := gc.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	var records []map[string]interface{}
	for result.Next(ctx) {
		record := result.Record()
		recordMap := make(map[string]interface{})
		for _, key := range record.Keys {
			value, _ := record.Get(key)
			recordMap[key] = value
		}
		records = append(records, recordMap)
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("result error: %w", err)
	}

	return records, nil
}

// ExecuteWrite executes a write Cypher query
func (gc *GraphClient) ExecuteWrite(ctx context.Context, query string, params map[string]interface{}) error {
	session := gc.driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})

	return err
}
