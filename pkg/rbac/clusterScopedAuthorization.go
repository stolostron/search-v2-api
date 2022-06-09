package rbac

import (
	"context"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

type sharedList struct {
	err       error
	resources map[string][]string
	updatedAt time.Time
}

func (cache *Cache) checkUserResources() (sharedList, error) {

	cache.sharedLock.Lock()
	//check if we already have the resources in cache
	cr := cache.shared
	if cr.resources != nil && time.Now().Before(cr.updatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Second)) {
		klog.V(5).Info("Using cluster scoped resources from cache.")
		cache.sharedLock.Unlock()
		return cr, nil
	}
	cache.sharedLock.Unlock()

	if cr.resources != nil && cr.err != nil {
		return cr, cr.err
	}

	//otherwise build query and get cluster-scoped resources for user from database and cache:
	err := cache.getClusterScopedResources()

	return cr, err
}

func (cache *Cache) getClusterScopedResources() error {

	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	// dao.NewDAO(cache.pool)

	//"SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE
	// cluster='local-cluster' AND namespace=NULL"
	query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
		goqu.COALESCE(goqu.L(`"data"->>'kind'`), "").As("kind")).
		Where(goqu.L(`"cluster"::TEXT = 'local-cluster'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()
	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
	}
	rows, queryerr := cache.pool.Query(context.Background(), query)
	if queryerr != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, queryerr.Error())
	}
	if rows != nil {
		defer rows.Close()
		csrmap := make(map[string][]string)

		for rows.Next() {
			var kind string
			var apigroup string
			err := rows.Scan(&apigroup, &kind)
			if err != nil {
				klog.Errorf("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query, apigroup, kind)
			}

			csrmap[apigroup] = append(csrmap[apigroup], kind)

		}

		resourcelist := sharedList{resources: csrmap, err: err, updatedAt: time.Now()}
		cache.shared = resourcelist

		fmt.Println(resourcelist)
	}
	return err
}
