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

// If user has access o even one cluster, show the search disabled message
func (s *Message) checkUserAccessToDisabledClusters(ctx context.Context, disabledClusters *map[string]struct{}) bool {

	for _, cluster := range getKeys(*disabledClusters) { //rbac.CacheInst.GetDisabledClusters()) {
		_, userHasAccessToMC := s.userData.ManagedClusters[cluster]
		klog.Info("checking user access to cluster: ", cluster, ": ", userHasAccessToMC)

		if userHasAccessToMC {
			return true
		}
	}
	return false
}
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
				klog.Errorf("Error %s resolving cluster count for query: %s", err.Error(), s.query)
			}
			disabledClusters[srchAddonDisabledCluster] = struct{}{}

			//Since cache is not valid, update shared cache with disabled clusters result
			rbac.CacheInst.SetDisabledClusters(disabledClusters, nil)
		}
	}
	return &disabledClusters, err
}
func (s *Message) messageResults(ctx context.Context) ([]*model.Message, error) {
	klog.V(2).Info("Resolving Messages()")
	var returnDisabledMessage bool

	if rbac.CacheInst.SharedCacheDisabledClustersValid() {
		klog.Info("Cache valid")
		returnDisabledMessage = s.checkUserAccessToDisabledClusters(ctx, rbac.CacheInst.GetDisabledClusters())
	} else {
		klog.Info("Cache not valid - runnning query")

		if disabledClusters, err := s.findSrchAddonDisabledClusters(ctx); err != nil {
			klog.Error("Error retrieving disabled clusters: ", err)
			return []*model.Message{}, err
		} else {
			returnDisabledMessage = s.checkUserAccessToDisabledClusters(ctx, disabledClusters)
		}
	}
	klog.Info("returnDisabledMessage: ", returnDisabledMessage)
	// Show disabled clusters message only if user has access to those managed clusters
	if returnDisabledMessage {
		messages := make([]*model.Message, 0)
		kind := "information"
		desc := "Search is disabled on some of your managed clusters."
		message := model.Message{ID: "S20",
			Kind:        &kind,
			Description: &desc}
		messages = append(messages, &message)
		return messages, nil
	} else {
		klog.Info("user doesn't have access to disabled clusters", returnDisabledMessage)
	}
	return nil, nil
}
