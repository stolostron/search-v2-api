package rbac

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/jackc/pgx/v4/pgxpool"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"k8s.io/klog/v2"
)

type clusterScopedResourcesList struct {
	err       error
	resources map[string]string // map of kind:apigroup
}

func (cache *Cache) checkUserResources(token string) error {

	// look at cache for specific user uid
	uid := cache.tokenReviews[token].tokenReview.UID
	//check if we already have the resources in cache
	cr, resourcesExist := cache.clusterScoped[string(uid)]
	if resourcesExist && cr != nil {
		klog.V(5).Info("Using cluster scoped resources from cache.")
		return nil
	}
	if resourcesExist && cr.err != nil {
		return cr.err
	}
	//otherwise build query and get cluster-scoped resources for user from database and cache:
	cache.getClusterScopedResources(db.GetConnection(), string(uid))

	return nil
}

func (cache *Cache) getClusterScopedResources(pool *pgxpool.Pool, uid string) error {

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
	csrmap := make(map[string]string)
	for rows.Next() {
		var kind, apigroup string
		err := rows.Scan(&kind, &apigroup)
		if err != nil {
			klog.Errorf("Error %s retrieving rows for query:%s", err.Error(), query)
		}

		csrmap[kind] = apigroup
	}
	resourcelist := &clusterScopedResourcesList{resources: csrmap, err: err}
	cache.clusterScoped[uid] = resourcelist

	return err
}
