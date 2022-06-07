package resolver

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/stolostron/search-v2-api/graph/model"
	db "github.com/stolostron/search-v2-api/pkg/database"
	klog "k8s.io/klog/v2"
)

type Message struct {
	pool   pgxpoolmock.PgxPool
	query  string
	params []interface{}
}

func Messages(ctx context.Context) ([]*model.Message, error) {
	searchSchemaResult := &Message{
		pool: db.GetConnection(),
	}
	searchSchemaResult.buildSearchAddonDisabledQuery(ctx)
	return searchSchemaResult.messageResults(ctx)
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
	selectDs = ds.Select(goqu.COUNT(goqu.DISTINCT(goqu.L(`"mcInfo".data->>?`, "name"))))

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

func (s *Message) messageResults(ctx context.Context) ([]*model.Message, error) {
	klog.V(2).Info("Resolving Messages()")

	rows := s.pool.QueryRow(ctx, s.query)

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
