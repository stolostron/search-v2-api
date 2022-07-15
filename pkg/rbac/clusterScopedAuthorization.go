package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

type clusterScopedResources struct {
	err       error
	resources map[string][]string
	updatedAt time.Time
	lock      sync.Mutex
}

func (cache *Cache) ClusterScopedResources(ctx context.Context) (map[string][]string, error) {
	clusterScoped, err := cache.shared.getClusterScopedResources(cache, ctx)
	return clusterScoped, err
}

func (shared *clusterScopedResources) getClusterScopedResources(cache *Cache, ctx context.Context) (map[string][]string,
	error) {

	//lock to prevent checking more than one at a time
	shared.lock.Lock()
	defer shared.lock.Unlock()
	if shared.resources != nil &&
		time.Now().Before(shared.updatedAt.Add(time.Duration(config.Cfg.SharedCacheTTL)*time.Millisecond)) {
		klog.V(8).Info("Using cluster scoped resources from cache.")
		return shared.resources, shared.err
	}
	klog.V(6).Info("Querying database for cluster-scoped resources.")
	shared.err = nil // Clear previous errors.

	// if data not in cache or expired
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//"SELECT DISTINCT(data->>apigroup, data->>kind) FROM search.resources WHERE
	// cluster='local-cluster' AND namespace=NULL"
	query, _, err := ds.SelectDistinct(goqu.COALESCE(goqu.L(`"data"->>'apigroup'`), "").As("apigroup"),
		goqu.COALESCE(goqu.L(`"data"->>'kind'`), "").As("kind")).
		Where(goqu.L(`"cluster"::TEXT = 'local-cluster'`), goqu.L(`"data"->>'namespace'`).IsNull()).ToSQL()
	if err != nil {
		klog.Errorf("Error creating query [%s]. Error: [%+v]", query, err)
		shared.err = err
	}

	rows, queryerr := cache.pool.Query(ctx, query)
	if queryerr != nil {
		klog.Errorf("Error resolving query [%s]. Error: [%+v]", query, queryerr.Error())
		shared.err = queryerr
		shared.resources = map[string][]string{}
		return shared.resources, shared.err
	}
	csrmap := make(map[string][]string)
	if rows != nil {
		defer rows.Close()

		for rows.Next() {
			var kind string
			var apigroup string
			err := rows.Scan(&apigroup, &kind)
			if err != nil {
				klog.Errorf("Error %s retrieving rows for query:%s for apigroup %s and kind %s", err.Error(), query,
					apigroup, kind)
				shared.err = err
				continue
			}

			csrmap[apigroup] = append(csrmap[apigroup], kind)

		}
	}

	// Then update the cache.
	shared.resources = csrmap
	shared.updatedAt = time.Now()

	return shared.resources, shared.err
}
