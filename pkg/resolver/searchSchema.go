package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	klog "k8s.io/klog/v2"
)

type SearchSchema struct {
	pool     pgxpoolmock.PgxPool
	query    string
	params   []interface{}
	userData rbac.UserData
	input     *model.SearchInput
	propTypes map[string]string
}

func SearchSchemaResolver(ctx context.Context, srchInput *model.SearchInput) (map[string]interface{}, error) {
	userData, userDataErr := rbac.GetCache().GetUserData(ctx)
	if userDataErr != nil {
		return nil, userDataErr
	}

	// Check that shared cache has property types:
	propTypes, err := rbac.GetCache().GetPropertyTypes(ctx, false)
	if err != nil {
		klog.Warningf("Error creating datatype map with err: [%s] ", err)
	}

	// Proceed if user's rbac data exists
	searchSchemaResult := &SearchSchema{
		pool:      db.GetConnPool(ctx),
		userData:  userData,
		input:     srchInput,
		propTypes: propTypes,
	}
	searchSchemaResult.buildSearchSchemaQuery(ctx)
	return searchSchemaResult.searchSchemaResults(ctx)
}

// Build the query to get all the properties (or keys) from the resources in the database.
// These are used to build the search schema.
func (s *SearchSchema) buildSearchSchemaQuery(ctx context.Context) {
	var selectDs *goqu.SelectDataset

	// schema query sample: SELECT DISTINCT "prop" FROM (SELECT jsonb_object_keys(jsonb_strip_nulls("data")) AS "prop"
	// FROM "search"."resources" LIMIT 100000) AS "schema"

	// This query doesn't show keys with null values but keys with empty string values are not excluded.
	// The query below should exclude empty values, but will take more time to execute (182 ms vs 241 ms)
	// - not tested on a large amount of data.

	//     select distinct key from (select (jsonb_each_text(jsonb_strip_nulls(data))).* from search.resources)A
	//     where value <> '' AND value <> '{}' AND value <> '[]'

	//FROM CLAUSE
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//WHERE CLAUSE
	var whereDs []exp.Expression

	// WHERE CLAUSE
	if s.input != nil && len(s.input.Filters) > 0 {
		whereDs, s.propTypes, _ = WhereClauseFilter(ctx, s.input, s.propTypes)
	}

	//get user info for logging
	_, userInfo := rbac.GetCache().GetUserUID(ctx)

	// if one of them is not nil, userData is not empty
	if s.userData.CsResources != nil || s.userData.NsResources != nil || s.userData.ManagedClusters != nil {
		whereDs = append(whereDs,
			buildRbacWhereClause(ctx, s.userData, userInfo)) // add rbac
	} else {
		klog.Errorf("Error building search schema query: RBAC clause is required!"+
			" None found for search schema query for user %s with uid %s ",
			userInfo.Username, userInfo.UID)

		s.query = ""
		s.params = nil
		return
	}

	//SELECT CLAUSE
	jsb := goqu.L("jsonb_object_keys(jsonb_strip_nulls(?))", goqu.C("data")).As("prop") //remove null fields
	//Adding an arbitrarily high number 100000 as limit here in the inner query
	// Adding a LIMIT helps to speed up the query
	// Adding a high number so as to get almost all the distinct properties from the database
	if whereDs != nil {
		selectDs = ds.SelectDistinct("prop").From(ds.Select(jsb).Where(whereDs...).
			Limit(config.Cfg.QueryLimit * 100).As("schema"))
	} else {
		selectDs = ds.SelectDistinct("prop").From(ds.Select(jsb).
			Limit(config.Cfg.QueryLimit * 100).As("schema"))
	}
	//Get the query
	sql, params, err := selectDs.ToSQL()
	if err != nil {
		klog.Errorf("Error building SearchSchema query: %s", err.Error())
	}
	s.query = sql
	s.params = params
	klog.V(3).Info("SearchSchema Query: ", sql)
}

func (s *SearchSchema) searchSchemaResults(ctx context.Context) (map[string]interface{}, error) {
	klog.V(2).Info("Resolving searchSchemaResults()")
	srchSchema := map[string]interface{}{}
	// These default properties are always present and we want them at the top.
	schema := []string{"cluster", "kind", "label", "name", "namespace", "status"}
	// Use a map to remove duplicates efficiently.
	schemaMap := map[string]struct{}{}
	for _, key := range schema {
		schemaMap[key] = struct{}{}
	}

	rows, err := s.pool.Query(ctx, s.query)
	if err != nil {
		klog.Error("Error fetching search schema results from db ", err)
		return srchSchema, err
	}
	defer rows.Close()
	for rows.Next() {
		prop := ""
		_ = rows.Scan(&prop)
		// Skip properties that start with _ because those are used internally and aren't intended to be exposed.
		if prop[0:1] == "_" {
			continue
		}
		if _, present := schemaMap[prop]; !present {
			schema = append(schema, prop)
		}
	}
	srchSchema["allProperties"] = schema
	return srchSchema, nil
}
