package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	klog "k8s.io/klog/v2"
)

func Messages(ctx context.Context) ([]*model.Message, error) {
	searchSchemaResult := &SearchSchemaMessage{
		pool: db.GetConnection(),
	}
	searchSchemaResult.messageQuery(ctx)
	return searchSchemaResult.messageResults()
}

func (s *SearchSchemaMessage) messageQuery(ctx context.Context) {
	var selectDs1, selectDs2 *goqu.SelectDataset

	// Get all clusters - exclude local-cluster and managed clusters where search ManagedClusterAddon is not present
	// messsage query sample: SELECT COUNT(DISTINCT("cluster")) from search.resources where cluster not in ( '' , 'local-cluster')
	// and cluster not in (
	// SELECT distinct data->> 'namespace' as cluster FROM "search"."resources"
	// where  "data"->>'kind'= 'ManagedClusterAddOn' and "data"->>'name'='search-collector')

	//FROM CLAUSE
	schemaTable := goqu.S("search").Table("resources")
	ds := goqu.From(schemaTable)

	//SELECT CLAUSE
	//Select all clusters
	cluster := goqu.C("cluster") //remove null fields
	selectDs1 = ds.Select(goqu.COUNT(goqu.DISTINCT(cluster)))
	// Select all remote namespaces as clusters
	remoteNS := goqu.L(`"data"->>?`, "namespace").As("cluster")
	selectDs2 = ds.SelectDistinct(remoteNS)

	var whereDs, whereDsSearchEnabledClusters []exp.Expression
	//Exclude local-cluster and empty cluster fields
	clusterIds := []string{"", "local-cluster"}
	whereDs = append(whereDs, goqu.L(`"cluster"`).NotIn(clusterIds))

	//Find clusters where search is enabled
	whereDsSearchEnabledClusters = append(whereDsSearchEnabledClusters,
		goqu.L(`"data"->>?`, "kind").Eq("ManagedClusterAddOn"))
	whereDsSearchEnabledClusters = append(whereDsSearchEnabledClusters,
		goqu.L(`"data"->>?`, "name").Eq("search-collector"))

	//Exclude clusters where search is enabled
	whereDs = append(whereDs, goqu.L(`"cluster"`).NotIn(selectDs2.Where(whereDsSearchEnabledClusters...)))

	//Get the query
	sql, params, err := selectDs1.Where(whereDs...).ToSQL()
	if err != nil {
		klog.Errorf("Error building Messages query: %s", err.Error())
	}
	s.query = sql
	s.params = params
	klog.Infof("Messages Query: %s\n", sql)
}

func (s *SearchSchemaMessage) messageResults() ([]*model.Message, error) {
	klog.V(2).Info("Resolving Messages()")

	rows := s.pool.QueryRow(context.Background(), s.query)

	if rows != nil {
		var count int
		err := rows.Scan(&count)
		if err != nil {
			klog.Errorf("Error %s resolving cluster count for query:%s", err.Error(), s.query)
		}
		if count > 0 {
			messages := make([]*model.Message, 0)
			kind := "information"
			desc := "Search is disabled on some of your managed clusters."
			message := model.Message{ID: "S20",
				Kind:        &kind,
				Description: &desc}
			messages = append(messages, &message)
			return messages, nil
		}
	}
	return nil, nil
}
