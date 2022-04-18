package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/driftprogramming/pgxpoolmock"
	db "github.com/stolostron/search-v2-api/pkg/database"
	klog "k8s.io/klog/v2"
)

type SearchSchemaMessage struct {
	pool   pgxpoolmock.PgxPool
	query  string
	params []interface{}
}

func SearchSchema(ctx context.Context) (map[string]interface{}, error) {
	searchSchemaResult := &SearchSchemaMessage{
		pool: db.GetConnection(),
	}
	searchSchemaResult.searchSchemaQuery(ctx)
	return searchSchemaResult.searchSchemaResults(ctx)
}

func (s *SearchSchemaMessage) searchSchemaQuery(ctx context.Context) {
	var selectDs *goqu.SelectDataset

	// schema query sample: "SELECT distinct jsonb_object_keys(jsonb_strip_nulls(data)) FROM search.resources"

	// This query doesn't show keys with null values but keys with empty string values are not excluded.
	// The query below should exclude empty values, but will take more time to execute (182 ms vs 241 ms)
	// - not tested on a large amount of data.

	//     select distinct key from (select (jsonb_each_text(jsonb_strip_nulls(data))).* from search.resources)A
	//     where value <> '' AND value <> '{}' AND value <> '[]'

	//FROM CLAUSE
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//SELECT CLAUSE
	jsb := goqu.L("jsonb_object_keys(jsonb_strip_nulls(?))", goqu.C("data")) //remove null fields
	selectDs = ds.SelectDistinct(jsb)

	//Get the query
	sql, params, err := selectDs.ToSQL()
	if err != nil {
		klog.Errorf("Error building SearchSchema query: %s", err.Error())
	}
	s.query = sql
	s.params = params
	klog.V(3).Info("SearchSchema Query: ", sql)
}

func (s *SearchSchemaMessage) searchSchemaResults(ctx context.Context) (map[string]interface{}, error) {
	klog.V(2).Info("Resolving searchSchemaResults()")
	srchSchema := map[string]interface{}{}
	schemaTop := []string{"cluster", "kind", "label", "name", "namespace", "status"}
	schemaTopMap := map[string]struct{}{}
	for _, key := range schemaTop {
		schemaTopMap[key] = struct{}{}
	}
	schema := []string{}
	schema = append(schema, schemaTop...)

	rows, err := s.pool.Query(ctx, s.query)
	if err != nil {
		klog.Error("Error fetching search schema results from db ", err)
	}
	defer rows.Close()
	if rows != nil {
		for rows.Next() {
			prop := ""
			_ = rows.Scan(&prop)
			if _, present := schemaTopMap[prop]; !present {
				schema = append(schema, prop)
			}
		}
	}
	srchSchema["allProperties"] = schema
	return srchSchema, nil
}
