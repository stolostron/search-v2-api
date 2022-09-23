package resolver

import (
	"context"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	klog "k8s.io/klog/v2"
)

type Message struct {
	userData *rbac.UserData
}

func Messages(ctx context.Context) ([]*model.Message, error) {
	userAccess, userDataErr := rbac.CacheInst.GetUserData(ctx)
	if userDataErr != nil {
		return []*model.Message{}, userDataErr
	}
	message := &Message{
		userData: userAccess,
	}
	return message.messageResults(ctx)
}

func (s *Message) messageResults(ctx context.Context) ([]*model.Message, error) {
	klog.V(2).Info("Resolving Messages()")

	disabledClusters, disabledClustersErr := rbac.CacheInst.GetDisabledClusters(ctx)
	//Cache is invalid
	if disabledClustersErr != nil {
		return []*model.Message{}, disabledClustersErr
	}
	//Cache is valid
	if len(*disabledClusters) <= 0 { //no clusters with addon disabled or user does not have access to view them
		return []*model.Message{}, nil
	} else {
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
