package rbac

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/klog/v2"
)

// type clusterScopedResources struct {
// 	resources map[string]string // map of apigroup:kind
// }

//function to make call to database to get all cluster-scoped resources:
func (cache *Cache) getClusterScopedResources(pool *pgxpool.Pool) error {

	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//"SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE cluster='local-cluster' AND namespace=NULL"
	query, _, err := ds.SelectDistinct(goqu.L(`"data"->>'apigroup'`), goqu.L(`"data"->>'kind'`)).
		Where(goqu.L(`"cluster"::TEXT = 'local-cluster'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()

	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
	}

	rows, err := pool.Query(context.Background(), query, nil...)
	if err != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind, apigroup string

		err := rows.Scan(&kind, &apigroup)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
		}
		cache.clusterScopedResources[apigroup] = kind //we need to cache the resources not return

	}
	fmt.Println(cache)

	return err
}
