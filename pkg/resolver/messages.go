package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	klog "k8s.io/klog/v2"
)

type Message struct {
	pool     pgxpoolmock.PgxPool
	query    string
	params   []interface{}
	userData *rbac.UserData
}

func Messages(ctx context.Context) ([]*model.Message, error) {
	userAccess, userDataErr := rbac.CacheInst.GetUserData(ctx)
	if userDataErr != nil {
		return nil, userDataErr
	}
	message := &Message{
		pool:     db.GetConnection(),
		userData: userAccess,
	}
	message.buildSearchAddonDisabledQuery(ctx)
	return message.messageResults(ctx)
}

// Build the query to find any ManagedClusters where the search addon is disabled.
func (s *Message) buildSearchAddonDisabledQuery(ctx context.Context) {
	var selectDs *goqu.SelectDataset

	//FROM CLAUSE
	schemaTable1 := goqu.S("search").Table("resources").As("mcInfo")
	schemaTable2 := goqu.S("search").Table("resources").As("srchAddon")

	// For each ManagedClusterInfo resource in the hub,
	// we should have a matching ManagedClusterAddOn
	// with name=search-collector in the same namespace.
	ds := goqu.From(schemaTable1).
		LeftOuterJoin(schemaTable2,
			goqu.On(goqu.L(`"mcInfo".data->>?`, "name").Eq(goqu.L(`"srchAddon".data->>?`, "namespace")),
				goqu.L(`"srchAddon".data->>?`, "kind").Eq("ManagedClusterAddOn"),
				goqu.L(`"srchAddon".data->>?`, "name").Eq("search-collector")))

	//SELECT CLAUSE
	selectDs = ds.SelectDistinct(goqu.L(`"mcInfo".data->>?`, "name").As("srchAddonDisabledCluster"))

	// WHERE CLAUSE
	var whereDs []exp.Expression

	// select ManagedClusterInfo
	whereDs = append(whereDs, goqu.L(`"mcInfo".data->>?`, "kind").Eq("ManagedClusterInfo"))
	// addon uid will be null if addon is disabled
	whereDs = append(whereDs, goqu.L(`"srchAddon".uid`).IsNull())
	// exclude local-cluster
	whereDs = append(whereDs, goqu.L(`"mcInfo".data->>?`, "name").Neq("local-cluster"))

	//Get the query
	sql, params, err := selectDs.Where(whereDs...).ToSQL()
	if err != nil {
		klog.Errorf("Error building Messages Query for managed clusters with Search addon disabled: %s", err.Error())
	}
	s.query = sql
	s.params = params
	klog.V(3).Infof("Messages Query for managed clusters with Search addon disabled: %s\n", sql)
}

// <<<<<<< HEAD
// // If user has access o even one cluster, show the search disabled message
// func (s *Message) checkUserAccessToDisabledClusters(ctx context.Context, disabledClusters *map[string]struct{}) bool {

// 	for _, cluster := range getKeys(*disabledClusters) { //rbac.CacheInst.GetDisabledClusters()) {
// 		_, userHasAccessToMC := s.userData.ManagedClusters[cluster]
// 		klog.Info("checking user access to cluster: ", cluster, ": ", userHasAccessToMC)

// 		if userHasAccessToMC {
// 			return true
// 		}
// 	}
// 	return false
// }
// =======
// >>>>>>> disabledClustersRbac
func (s *Message) findSrchAddonDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	disabledClusters := make(map[string]struct{})

	rows, err := s.pool.Query(ctx, s.query)
	if err != nil {
		klog.Error("Error fetching SearchAddon disabled cluster results from db ", err)
		rbac.CacheInst.SetDisabledClusters(disabledClusters, err)
		return &disabledClusters, err
	}
	defer rows.Close()
	if rows != nil {
		for rows.Next() {
			var srchAddonDisabledCluster string
			err := rows.Scan(&srchAddonDisabledCluster)
			if err != nil {
				klog.Errorf("Error %s resolving addon disabled cluster name for query: %s", err.Error(), s.query)
				continue // skip and continue in case of scan error
			}
			disabledClusters[srchAddonDisabledCluster] = struct{}{}
		}
		//Since cache was not valid, update shared cache with disabled clusters result
		rbac.CacheInst.SetDisabledClusters(disabledClusters, nil)
	}
	return &disabledClusters, err
}
func (s *Message) userHasAccessToDisabledClusters(disabledClusters *map[string]struct{}) bool {

	for disabledCluster := range *disabledClusters {
		_, userHasAccessToMC := s.userData.ManagedClusters[disabledCluster]
		if userHasAccessToMC { //user has access
			klog.V(7).Info("user has access to search addon disabled cluster: ", disabledCluster)
			return true
		}
	}
	return false
}

func (s *Message) getDisabledClusters(ctx context.Context) (*map[string]struct{}, error) {
	disabledClusters, disabledClustersErr := rbac.CacheInst.GetDisabledClusters()
	if disabledClustersErr != nil { //Cache is invalid - rerun query
		//run query and get disabled clusters
		if disabledClustersFromQuery, err := s.findSrchAddonDisabledClusters(ctx); err != nil {
			klog.Error("Error retrieving Search Addon disabled clusters: ", err)
			rbac.CacheInst.SetDisabledClusters(map[string]struct{}{}, err)
			return &map[string]struct{}{}, err
		} else {
			rbac.CacheInst.SetDisabledClusters(*disabledClustersFromQuery, nil)
			return disabledClustersFromQuery, err
		}
	} else {
		klog.Info("Disabled clusters cache valid. Returning results")
	}
	//cache is valid
	return disabledClusters, disabledClustersErr
}
func (s *Message) messageResults(ctx context.Context) ([]*model.Message, error) {
	klog.V(2).Info("Resolving Messages()")
	disabledClusters, disabledClustersErr := s.getDisabledClusters(ctx)
	//Cache is invalid
	if disabledClustersErr != nil {
		return []*model.Message{}, disabledClustersErr
	}
	//Cache is valid
	if len(*disabledClusters) <= 0 { //no clusters with addon disabled
		return []*model.Message{}, nil
	} else { //check if user has access to disabled clusters
		if s.userHasAccessToDisabledClusters(disabledClusters) {
			klog.V(5).Info("user has access to Search Addon disabled clusters ")
			// Show disabled clusters message only if user has access to those managed clusters

			messages := make([]*model.Message, 0)
			kind := "information"
			desc := "Search is disabled on some of your managed clusters."
			message := model.Message{ID: "S20",
				Kind:        &kind,
				Description: &desc}
			messages = append(messages, &message)
			return messages, nil
		} else {
			klog.V(5).Info("user doesn't have access to Search Addon disabled clusters.")
			return []*model.Message{}, nil
		}
	}
}
