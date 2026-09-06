package api

import (
	"context"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
)

func (r *configResultResolver) Plugins(ctx context.Context, obj *ConfigResult, include []string) (map[string]map[string]interface{}, error) {
	principal, err := authz.PrincipalFromContext(ctx)
	if err != nil || principal.Role != authz.RoleAdmin {
		return map[string]map[string]interface{}{}, nil
	}
	if len(include) == 0 {
		ret := config.GetInstance().GetAllPluginConfiguration()
		return ret, nil
	}

	ret := make(map[string]map[string]interface{})

	for _, plugin := range include {
		c := config.GetInstance().GetPluginConfiguration(plugin)
		if len(c) > 0 {
			ret[plugin] = c
		}
	}

	return ret, nil
}
