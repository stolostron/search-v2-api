package schema

import (
	"context"

	klog "k8s.io/klog/v2"

	db "github.com/stolostron/search-v2-api/pkg/database"
)

func SearchSchema(ctx context.Context) (map[string]interface{}, error) {
	query := searchSchemaQuery(ctx)
	klog.Infof("SearchSchema Query: ", query)
	return searchSchemaResults(query)
}

func searchSchemaQuery(ctx context.Context) string {
	var selectClause, query string
	selectClause = "SELECT distinct jsonb_object_keys(data) FROM resources "

	query = selectClause
	klog.Info("SearchSchema Query: ", query)
	return query

}

func searchSchemaResults(query string) (map[string]interface{}, error) {
	// values := []string{"cluster", "kind", "label", "name", "namespace", "status"}
	srchSchema := map[string]interface{}{}
	schemaTop := []string{"cluster", "kind", "label", "name", "namespace", "status"}
	schemaTopMap := map[string]struct{}{}
	for _, key := range schemaTop {
		schemaTopMap[key] = struct{}{}
	}
	schema := []string{}
	schema = append(schema, schemaTop...)

	pool := db.GetConnection()
	//TODO: Handle error
	rows, _ := pool.Query(context.Background(), query)
	defer rows.Close()
	prop := ""
	for rows.Next() {
		_ = rows.Scan(&prop)
		tmpProp := prop
		if _, present := schemaTopMap[tmpProp]; !present {
			schema = append(schema, tmpProp)
		}
	}
	srchSchema["allProperties"] = schema
	return srchSchema, nil
}
